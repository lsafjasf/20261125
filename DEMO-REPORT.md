# 断点恢复 / 重复执行 对照报告

- 生成时间: 2026-11-25T01:20:52+08:00
- 二进制: `/home/hj/userfeedback/uid4-0032/batchproc`
- 工作区: `/tmp/bp-final`

## 对照表

| 对照维度 | 旧逻辑（无状态 / 直接截断写） | 修复后（检查点 / 临时文件 / 完成标记） |
| --- | --- | --- |
| 崩溃后正式产物 | clip-a.mp4 被截断 (2.06 MiB) | 正式产物不存在，字节只进 .tmp-* |
| 完成标记 / 校验 | 无 .ok、无 SHA-256 | 未完成不写标记，完成后写 .ok |
| 重启后实际执行阶段 | 4/4 阶段全部重跑，0 跳过 | 仅 encode/publish 重跑；prepare/render 检查点跳过 |
| 崩溃当次已做工作 | 2 个阶段完成但无状态可恢复 | prepare/render 已落检查点 |
| 崩溃残留与清扫 | 正式文件本身是残片，只能再次截断重写 | 留 1 个 .tmp-*，重启清扫，残留 0 |
| 重复执行（第三次启动） | 无状态，仍会 2048 KiB 全量重渲染 | 0 个阶段执行，识别有效 .ok 后 uptodate 跳过 |
| 校验命令 verify | 无法判断文件是否完整 | 大小 + SHA-256 + 标记三者一致才 OK |
| 单任务失败影响 | 一个任务报错，整批中断 | 错误被捕获并隔离，其余任务照常完成，退出码 2 |
| 成功产物覆盖策略 | os.Create 立即截断，旧产物无法挽回 | 默认跳过；--force 须输入 yes 或传 --yes，临时文件原子替换 |

## 断言结果

- [PASS] **旧逻辑崩溃退出码为 137** — code=137
- [PASS] **旧逻辑崩溃后留下非空截断文件** — clip-a.mp4=2162880 字节
- [PASS] **旧逻辑没有完成标记** — marker exists=false
- [PASS] **旧逻辑重启后 4 个阶段全部重跑** — exit=0, a2 stages=[prepare render encode publish]
- [PASS] **截断文件小于完整产物** — truncated=2162880, full=3211456
- [PASS] **修复版崩溃退出码为 137** — code=137
- [PASS] **崩溃后不存在截断的正式产物** — final exists=false
- [PASS] **崩溃后留有临时残片** — .tmp files=1
- [PASS] **断点恢复退出码为 0** — code=0
- [PASS] **恢复时只执行 encode/publish** — done=[encode publish]
- [PASS] **prepare/render 命中检查点跳过** — skip=[prepare render]
- [PASS] **重启后清扫了临时残片** — remaining .tmp=0
- [PASS] **verify 校验通过** — verify=0 markerValid=true
- [PASS] **重复执行零阶段渲染且跳过任务** — b3 done=[] jobSkip=1
- [PASS] **部分失败时批处理退出码为 2** — code=2
- [PASS] **前后两个正常任务都成功** — jobDone=2, goodA=true, goodC=true
- [PASS] **坏任务被记录为失败** — jobFailed=1
- [PASS] **坏任务没有产物** — bad final exists=false
- [PASS] **首次渲染成功** — code=0
- [PASS] **非交互且无 --yes 时拒绝覆盖(退出码3)** — code=3
- [PASS] **空确认后 SHA-256 不变** — before=79ad1c65ed0d after=79ad1c65ed0d
- [PASS] **输入 no 时拒绝覆盖(退出码3)** — code=3
- [PASS] **拒绝后 SHA-256 仍不变** — same=true
- [PASS] **输入完整 yes 后才覆盖重跑** — code=0 stages=[prepare render encode publish]
- [PASS] **覆盖后新产物标记有效** — markerValid=true

通过 25/25。

## 手动复现

```bash
go build -o batchproc .
./batchproc demo            # 端到端对照（自动构造崩溃/重启）
./batchproc verify --jobs jobs.example.json --out ./out
```
