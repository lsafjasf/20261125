# mediaidx

长媒体追加场景下的 Go 完整性索引。实现不使用数据库：媒体字节写在连续数据文件中，索引写在一个追加式日志文件中。

核心约束：

- 每天新增片段只哈希新叶节点，不重读、不重算历史媒体。
- 每个叶节点对应一个视频帧，或一个音频采样段。
- 尾部截断、局部替换、索引空洞都会报告到具体的帧号或采样段。
- 事务提交前崩溃不影响已完成前沿；恢复时丢弃未提交日志和媒体尾字节。
- 只依赖标准库，无 CGO、无外部服务。

## 文件模型

`Open("media.idx")` 使用两个文件：

- `media.idx`：只追加的索引日志，包含文件头、叶节点描述和提交记录。
- `media.idx.data`：连续的媒体字节，偏移量由叶节点记录。

每个叶节点保存：

```text
序号 | frame/sample | 单元起点 | 单元数量 | 数据偏移 | 数据长度 | SHA-256
```

索引根是 Merkle Mountain Range（MMR）峰值。叶节点哈希为
`SHA-256(0x00 || SHA-256(data))`，内部节点为
`SHA-256(0x01 || left || right)`。

提交记录保存累计叶数、数据边界、MMR 峰值、根和上一条提交的哈希。重新打开时直接从提交记录恢复 MMR 前沿，不重新哈希历史叶节点；追加时只把新叶节点压入前沿，耗时与新增叶数成正比，根计算为 `O(log n)`。

## 提交与恢复

一次 `Begin`/`Commit` 是原子批次：

1. `AppendFrame` / `AppendSamples` 写入媒体数据，并在内存中暂存叶节点。
2. `Commit` 先持久化数据文件。
3. 再追加叶节点记录和一条带 CRC32C 的提交记录，并持久化索引。
4. 只有完整有效的提交记录会推进可见前沿。

恢复时从日志起点逐条校验 CRC 和提交链。遇到不完整记录、坏提交或提交后的多余记录，会截断到最后一条有效提交末尾；媒体文件也会截回该提交记录的数据边界。因此：

- 已落盘但未提交的媒体字节不会算完成。
- 已写叶节点记录但缺少提交记录的批次不会算完成。
- 恢复后无需人工清理，可继续从旧前沿追加。

如果媒体文件短于已提交边界，索引不会丢弃历史，但会进入 damaged 状态；`Begin` 返回 `ErrDamaged`，`Verify` 会给出第一个缺失的帧或采样段。

## Go API

```go
index, err := mediaidx.Open("media.idx")
if err != nil {
    log.Fatal(err)
}
defer index.Close()

tx, err := index.Begin()
if err != nil {
    log.Fatal(err)
}
// 每个文件/分块是一个帧叶节点，帧号自动续接。
if _, err := tx.AppendFrame(frame0); err != nil {
    log.Fatal(err)
}
if _, err := tx.AppendFrame(frame1); err != nil {
    log.Fatal(err)
}
// 一段音频包含 1024 个采样，采样起点自动续接。
if _, err := tx.AppendSamples(1024, audioSegment); err != nil {
    log.Fatal(err)
}
info, err := tx.Commit()
if err != nil {
    log.Fatal(err)
}
fmt.Printf("commit=%d leaves=%d root=%s\n", info.Number, info.LeafCount, info.Root)
```

显式坐标使用 `AppendFrameAt` / `AppendSamplesAt` 或
`Append(mediaidx.Locator{...}, data)`。坐标必须随媒体顺序前进，不接受重叠或回退。

校验与定位：

```go
report, err := index.Verify()
if err != nil {
    log.Fatal(err)
}
if !report.Complete {
    for _, issue := range report.Issues {
        fmt.Printf("%s at %s, bytes [%d,%d)\n",
            issue.Code, issue.Locator,
            issue.DataOffset, issue.DataOffset+issue.DataSize)
    }
}

// 已知被替换或截断的字节范围，映射回帧/采样段。
leaf, ok := index.LocateOffset(byteOffset)
affected, err := index.LocateRange(byteOffset, byteCount)

// 只核验被映射到的一个叶节点，不扫描其他媒体。
issue, err := index.VerifyLeaf(leaf.Seq)
```

`Verify` 的 issue 类型：

- `tail_truncated`：媒体尾部不足，定位到第一个不完整的叶节点。
- `corrupt_leaf`：同长度局部替换，定位到对应帧或采样段。
- `unit_gap`：坐标跳号，例如缺帧，定位到缺失起止范围。
- `uncommitted_suffix`：数据超出最后提交边界，不能算完成。

`Stats` 只读取元数据，不扫描媒体；返回提交号、叶数、数据边界、根、空洞/damaged/未提交状态。

## CLI

```bash
# 两个视频帧分块，原子追加为一个提交
go run ./cmd/mediaidx append \
  -index media.idx -kind frame \
  day-042/frame-000001.bin day-042/frame-000002.bin

# 每个文件是一段 1024 采样的音频
go run ./cmd/mediaidx append \
  -index media.idx -kind sample -count 1024 \
  day-042/audio-*.bin

go run ./cmd/mediaidx inspect -index media.idx
go run ./cmd/mediaidx verify -index media.idx -json
```

`verify` 在索引不完整时退出码为 1，便于纳入每日流水线。`append -start` 可显式指定首单元；不指定时从已提交前沿自动续写。

## 验证

```bash
go test ./...
go test -race ./...
```

测试覆盖：重开后续写、MMR 根恢复、仅新叶哈希、尾部截断、同长度局部替换、采样段定位、坐标空洞、提交前中断、撕裂叶记录、撕裂提交记录和显式回滚。

## 边界

- 单写者模型；不要多个进程同时写同一对文件。
- 库负责 `INDEX.data`，会把超出最后提交边界的数据视为未提交并截断。
- 调用方负责把容器解码成帧或采样段字节；索引不解析 MP4/WAV 等格式。
- `Verify` 会流式读取全部已提交媒体；日常追加本身不触发该扫描。局部检查可先用 `LocateRange` 找到叶节点，再按叶摘要做定向核验。
