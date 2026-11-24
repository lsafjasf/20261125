# 可恢复、内容寻址的转码执行器

这是一个只依赖 Go 标准库的转码执行核心。进程被杀、磁盘写满或阶段失败后，
同一个输入可以从最后一个已提交的阶段继续；最终输出只有在完整发布并校验后
才会产生完成标记。没有外部数据库或队列，现有队列 worker 只需调用
`Engine.Run` 或 `Engine.RunWithRetry`。

## 快速运行

```bash
go run ./cmd/transcode run \
  -root ./transcode-data \
  -input ./sample.bin \
  -param profile=copy \
  -retries 2
```

示例流水线先写确定性元数据，再做 passthrough。`primaryPath` 指向最终文件。
仓库内的 `transcode.PassthroughPipeline` 用于演示；生产环境可把
`transcode` 阶段的 `Run` 换成 `exec.CommandContext` 调用的 ffmpeg 等编码器。
此时必须提升该阶段的 `Version`，因为版本会进入内容地址。

```bash
go test ./...
go test -race ./...
```

## 两种内容地址

任务键固定输入和配置，输出对象再按清单内容寻址：

```text
jobKey     = SHA256(canonical JSON(
               input file SHA256,
               sorted parameters,
               ordered stage name/version/primary list))
objectHash = SHA256(canonical manifest.json bytes)
```

因此输入文件名、绝对路径和提交时间都不影响结果。`jobKey` 只用于定位工作和
完成索引；最终目录是：

```text
root/objects/sha256/<objectHash[:2]>/<objectHash>/
  manifest.json
  stages/<stage-name>/<artifact...>
```

`manifest.json` 记录每个产物的相对路径、大小和 SHA256。重复执行会先复验清单
哈希和每个产物；验证成功才直接复用。完成索引只是 `jobKey -> objectHash` 的
指针，不能被单独信任。

## 提交与恢复

```text
root/
  objects/sha256/...      已发布、不可变的完成对象
  jobs/.../<jobKey>.json  完成索引（原子写入）
  work/.../<jobKey>/
    input/source          输入内容快照
    stages/0000-name/     已提交阶段产物
    checkpoints/...json   阶段检查点（原子写入）
    tmp/                  未提交阶段和对象构建目录
  locks/                  flock 进程锁
```

每个阶段遵循以下顺序：

1. 在 `work/.../tmp/stage-*` 中执行，失败或取消时整个临时目录删除。
2. 扫描产物，拒绝符号链接和特殊文件，计算每个文件的大小与 SHA256。
3. `fsync` 文件树，把临时目录原子改名为 `stages/NNNN-name`。
4. 最后原子写入并 `fsync` 阶段检查点。

恢复时按顺序校验检查点。字段、阶段版本、产物集合、大小或任一哈希不一致，
都会从该阶段开始重跑，并连带清除所有下游阶段。检查点不是“开始过”的标记，
而是“该阶段完整且可验证”的标记。

对象发布同样使用私有临时目录：复制并复验阶段产物、写清单、`fsync`，再原子
改名到最终对象路径，复验整个对象，最后才写完成索引。对象校验除了拒绝缺失
或哈希不符的文件，也拒绝清单之外的残留文件。因此半成品不会出现在完成路径，
也不会被当作完成结果。

## 崩溃恢复矩阵

| 崩溃/失败位置 | 下次执行行为 |
| --- | --- |
| 阶段执行中 | 删除残留临时目录，重跑该阶段 |
| 阶段改名后、检查点前 | 检查点不存在，重跑该阶段 |
| 检查点写入中 | 旧检查点已移除；临时文件被忽略，重跑该阶段 |
| 对象构建中 | 删除对象临时目录；已提交阶段仍可复用 |
| 对象改名后、完成索引前 | 校验并复用已有完整对象，再补写索引 |
| 完成索引写入中 | 索引未原子出现则重算；完整对象可复用 |
| 磁盘满 | 错误返回，不提交当前阶段或对象；释放空间后可重试 |

同一 `jobKey` 有跨进程文件锁，避免两个 worker 同时提交同一份工作。锁由内核
在进程退出时释放，所以进程被杀不会留下永久锁。

## 取消、重试与确定性

`Run` 接受 `context.Context`；哈希、阶段执行、复制、发布和锁等待都会响应取消。
取消会清理本次临时目录，不会写检查点或完成索引。

`RunWithRetry` 提供有界指数退避，并可用 `RetryPolicy.Retryable` 定制错误分类。
阶段失败后重试会从失败阶段恢复，已提交阶段不重跑。输入在建立快照期间发生
变化会返回 `ErrInputChanged`，可安全重试。

确定性是阶段实现契约：同一输入、参数和阶段版本必须产生相同字节。如果完成
索引存在但对象损坏或丢失，重建出的对象必须仍等于原 `objectHash`；不同则返回
`ErrNondeterministic`，不会静默改写同一任务的结果。修改转码逻辑或参数含义时，
必须提升对应阶段的 `Version`。

## 接入真实转码器

```go
Stage{
    Name:    "h264",
    Version: "7", // 编码参数/代码变化时递增
    Primary: "output.mp4",
    Run: func(ctx context.Context, in transcode.StageInput) error {
        cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
            "-i", in.InputPath,
            filepath.Join(in.OutputDir, "output.mp4"))
        output, err := cmd.CombinedOutput()
        if err != nil {
            return fmt.Errorf("ffmpeg: %w: %s", err, output)
        }
        return nil
    },
}
```

编码器必须只写 `in.OutputDir`，并在返回 `nil` 前完成文件写入。需要读取上游
产物时使用 `in.Previous[stageName]`。真实编码器产生非确定性元数据时，应在阶段
内规范化为固定值，或把该元数据排除在最终产物之外。
