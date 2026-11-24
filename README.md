# batchproc — 可断点恢复的 Go 单二进制批处理器

针对“批处理器重启后从头渲染、留下截断文件”的问题，用**阶段检查点 + 临时文件原子发布 +
SHA-256 完成标记**修复，并保证：单任务失败被隔离、成功产物未经用户显式确认绝不覆盖。
零第三方依赖，`go build` 产出单个静态二进制。

## 30 秒看对照

```bash
go build -o batchproc .
./batchproc demo --report DEMO-REPORT.md
```

`demo` 会把同一个二进制作为子进程真实启动、用 `--crash job:stage` 注入硬崩溃（退出码 137），
再“重启”，并对旧逻辑（`__naive`：`os.Create` 直接截断写正式文件、无状态、无标记）与修复版
做同条件对照，共 25 条断言。典型输出：

```text
================ 断点恢复 vs 从头重跑：对照 ================
┌──────────────────────────┬──────────────────────────────────────┬──────────────────────────────────────────────┐
│ 对照维度                 │ 旧逻辑（无状态/直接截断写）          │ 修复后（检查点/临时文件/标记）               │
├──────────────────────────┼──────────────────────────────────────┼──────────────────────────────────────────────┤
│ 崩溃后正式产物           │ clip-a.mp4 被截断 (2.06 MiB)         │ 正式产物不存在，字节只进 .tmp-*              │
│ 完成标记 / 校验          │ 无 .ok、无 SHA-256                   │ 未完成不写标记，完成后写 .ok                 │
│ 重启后实际执行阶段       │ 4/4 阶段全部重跑，0 跳过             │ 仅 encode/publish 重跑；prepare/render 检查  │
│                          │                                      │ 点跳过                                       │
│ 崩溃残留与清扫           │ 正式文件本身是残片，只能再次截断重写 │ 留 1 个 .tmp-*，重启清扫，残留 0             │
│ 重复执行（第三次启动）   │ 无状态，仍会 2048 KiB 全量重渲染     │ 0 个阶段执行，识别有效 .ok 后 uptodate 跳过  │
│ 单任务失败影响           │ 一个任务报错，整批中断               │ 错误被捕获并隔离，其余任务照常完成，退出码 2 │
│ 成功产物覆盖策略         │ os.Create 立即截断，旧产物无法挽回   │ 默认跳过；--force 须输入 yes 或传 --yes 确认 │
└──────────────────────────┴──────────────────────────────────────┴──────────────────────────────────────────────┘
通过 25/25，全部对照断言通过 ✅
```

完整样例见 `DEMO-REPORT.md`（每次运行重新生成）。

## 问题 → 修复手段

| 原始问题 | 修复机制 | 关键代码 |
| --- | --- | --- |
| 重启后从头渲染 | 每个阶段成功后原子落检查点；重启校验产物大小+SHA-256，命中则跳过，损坏则从该阶段级联重跑 | `internal/runner/state.go`、`internal/runner/job.go` |
| 留下截断文件 | 所有写入先进同目录唯一 `.tmp-*`，`fsync` 后 `rename` 原子替换；重启清扫残片；可疑正式文件先隔离 | `internal/store/atomic.go`、`internal/store/sweep.go`、`internal/runner/publish.go` |
| 无法判断产物是否完成 | `<artifact>.ok` 完成标记，含大小与 SHA-256；标记/文件/摘要三者一致才算成功 | `internal/model/model.go`、`internal/runner/inspect.go` |
| 单任务失败拖垮整批 | 每任务 `recover` + 错误捕获，状态记为 `failed`，继续后续任务；退出码 2 表示部分失败 | `internal/runner/runner.go` |
| 未确认就覆盖成功产物 | 有效 `.ok` 的任务默认幂等跳过；`--force` 时须交互输入完整 `yes` 或显式 `--yes`，否则退出码 3 且不动磁盘 | `internal/confirm/confirm.go` |

## 快速使用

```bash
# 1) 构建单二进制（可选静态、去调试信息）
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o batchproc .

# 2) 跑示例批次（其中 clip-bad-input 会在 encode 确定性失败，退出码 2，其余成功）
./batchproc run --jobs jobs.example.json --out ./out
echo $?   # 2

# 3) 校验所有产物与完成标记
./batchproc verify --jobs jobs.example.json --out ./out

# 4) 再次运行：成功任务全部跳过，坏任务可在修复 jobs 后重试
./batchproc run --jobs jobs.example.json --out ./out

# 5) 强制重跑成功任务：非交互环境必须显式 --yes
./batchproc run --jobs jobs.example.json --out ./out --force            # 无 TTY 时退出码 3
./batchproc run --jobs jobs.example.json --out ./out --force --yes      # 脚本化显式确认
```

### 亲手制造一次崩溃并恢复

```bash
echo '{"jobs":[{"id":"clip-x","size_kb":1024}]}' > /tmp/jobs.json
./batchproc run --jobs /tmp/jobs.json --out /tmp/out --crash clip-x:render
echo $?                                   # 137（注入的硬崩溃）
find /tmp/out -name '.tmp-*'              # 只看到临时残片，没有 clip-x.mp4
./batchproc run --jobs /tmp/jobs.json --out /tmp/out
# ↳ prepare 命中检查点跳过；render/encode/publish 续跑成功，并清扫残片
./batchproc run --jobs /tmp/jobs.json --out /tmp/out
# ✓ 已完成且 SHA-256 校验通过，跳过（重复执行幂等）
```

## 磁盘布局

```text
out/
├── clip-x.mp4                 # 正式产物：要么不存在，要么完整且与 .ok 匹配
├── clip-x.mp4.ok              # 完成标记：size + sha256 + attempts + completed_at
└── .batchproc/
    ├── state/clip-x.json      # 阶段状态：每个已提交阶段的 artifact/size/sha256
    ├── work/clip-x.render.bin # 已提交的阶段中间产物（检查点实体）
    ├── tmp/                   # （预留的尝试目录）
    ├── quarantine/            # 被判定为截断/损坏/旧逻辑遗留的文件隔离处
    └── events.jsonl           # stage_start/done/skip、job_done/failed 审计事件
```

写入顺序保证每个崩溃窗口都可恢复：

1. 阶段产物写到同目录 `.tmp-*` → `fsync` → `rename` 原子提交；
2. 更新 `state/<job>.json`（同样原子写），该阶段才算检查点；
3. `publish` 用临时文件原子替换正式产物；
4. 原子写 `<artifact>.ok`（SHA-256 标记）；
5. 状态置为 `completed`。

| 崩溃时机 | 重启行为 |
| --- | --- |
| 阶段写入中 | 正式产物不受影响；残留在 `.tmp-*`，启动清扫；该阶段重跑，更早检查点跳过 |
| 阶段 rename 后、state 更新前 | 该阶段视为未完成并重做（原子 rename 幂等覆盖） |
| publish rename 后、写 marker 前 | 发现正式文件摘要与 encode 检查点一致，直接认领并补 marker |
| marker 已写、state 未完成 | marker 校验通过，直接幂等跳过并补齐状态 |
| 遇到无 marker 且摘要不符的正式文件 | 判定为截断/旧逻辑文件，移入 `quarantine/` 后重新发布，不直接删除 |

检查点实体若被篡改或删除，启动校验会发现 size/SHA-256 不符，并从该阶段起**级联失效**后续阶段。

## 退出码

| 码 | 含义 |
| --- | --- |
| `0` | 全部成功，或全部为已完成的幂等跳过 |
| `1` | 用法/IO 错误，或 `verify` 发现不完整产物 |
| `2` | 存在任务级失败，但其他任务已完成（部分成功） |
| `3` | `--force` 需要覆盖成功产物，却未获得显式确认 |
| `137` | `--crash` 注入的模拟硬崩溃 |

## 命令与参数

- `batchproc run`：`--jobs`、`--out`、`--force`、`--yes`、`--crash job:stage`、`--run-id`、`--chunk-delay`
- `batchproc verify`：校验 marker/大小/SHA-256，并报告残留 `.tmp-*`
- `batchproc demo`：端到端对照；`--dir` 指定工作区、`--keep` 保留、`--report` 输出 Markdown
- `batchproc __naive`：旧逻辑模拟器（demo 内部调用）

任务文件示例见 `jobs.example.json`。任务字段：`id`、`size_kb`、`fail_stage`
（可选的确定性失败注入，取值 `prepare|render|encode|publish`）。

## 测试

```bash
go vet ./...
go test -count=1 ./...
go test -race ./internal/runner/
```

覆盖：崩溃后续跑且只跑未完成阶段、重复执行零渲染、检查点篡改级联、单任务失败隔离、
覆盖确认的三种拒绝路径与 `yes`/`--yes` 放行、截断文件隔离、marker 丢失修复；
根包集成测试会真实 `go build` 单二进制并跑通 `demo` 的全部断言。

## 工程说明

```text
main.go / cli_*.go             # 子命令与退出码
internal/model                 # jobs / state / marker / event 的 JSON schema
internal/store                 # 目录布局、原子写、SHA-256、事件日志、临时文件清扫
internal/engine                # 分块渲染阶段、崩溃钩子、旧逻辑 __naive
internal/confirm               # 覆盖成功产物前的显式确认
internal/runner                # 检查点恢复、发布矩阵、失败隔离、verify
internal/demo                  # 子进程级崩溃/重启对照与报告
```

- 纯标准库，无第三方依赖；`CGO_ENABLED=0` 可产出约 3–5 MB 静态单二进制，可直接交叉编译。
- 当前按任务**顺序执行**，语义最清晰；同一 `--out` 目录设计为单写者，请勿并发运行两个批次。
