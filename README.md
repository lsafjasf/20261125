# 多源媒体审查器

同一份媒体的**语言**和**时长**可能被四类来源分别给出不同的值。本项目用 Go
标准库实现一个多源审查器，把每个字段的结论严格分成三类：

- `automatic` 自动结论：可采信来源的证据本身足以支持，无需人工；
- `manual` 人工判定：证据存在冲突或不满足互证要求，交人决定，不输出结论值；
- `undetermined` 无法确定：没有可采信来源，**禁止回退采用文件系统值**。

## 来源与可信度

| 来源 | 常量 | 默认可采信 | 说明 |
| --- | --- | --- | --- |
| 码流 | `bitstream` | 是 | 从基本流解码得到，可信度最高 |
| 容器 | `container` | 是 | 容器头/标签元数据 |
| sidecar | `sidecar` | 是 | 外部伴随元数据（如 `.nfo`/`.json`） |
| 文件系统 | `filesystem` | **否** | 文件名、目录名、`stat` 启发式，只作证据展示 |

文件系统在 `DefaultPolicy` 中被排除在 `AdmissibleSources` 之外：它可以出现在
证据链和冲突路径里，但永远不能支持自动结论，也不会在无法确定时被当作兜底值。

## 判定规则

1. 先归一化原始值：
   - 语言：统一小写、`_` 转 `-`、取主子标签，并把 ISO 639-2/3 与常见英文名
     （如 `eng`、`english`、`zh_CN`）映射到 ISO 639-1（如 `en`、`zh`）；
   - 时长：支持 `HH:MM:SS[.fff]`、Go duration（`1h2m3.5s`）和裸秒数，
     统一换算为秒并输出 `6123.5s` 形式的规范值。
2. 解析失败的观测保留在证据链中，标记 `present=false`，不参与判定。
3. 对同一字段的所有观测两两比较，任何不一致都生成一条**冲突路径**：
   - 含文件系统的冲突会被记录，但不会阻断其它来源一致时的自动结论；
   - 同源内部冲突（如两个 sidecar 互相矛盾）一律转人工。
4. 仅在可采信来源中聚类（时长按 `DurationEpsilon`，默认 10ms 容差）：
   - 无任何可采信值 → `undetermined`，`value` 强制为空；
   - 存在多个互不一致的聚类 → `manual`；
   - 唯一聚类、多来源一致 → `automatic`，置信度 `high`；
   - 唯一聚类、仅单一来源 → 默认 `automatic`、置信度 `low`；
     若策略开启 `RequireCorroboration`，则改走 `manual`。

## 字段来源与冲突路径

每个字段的结论（`auditor.FieldVerdict`）都完整提交：

- `provenance`：该字段全部观测，逐条给出 `source`、`rawValue`（原始值）、
  `canonical`（规范值）、`present`（是否解析成功）、`admitted`（是否可采信）、
  `detail`（采集位置，如轨道号、原子名、文件路径）；
- `conflicts`：全部冲突路径，给出冲突双方来源与规范值，并标记
  `involvesFilesystem` 和 `intraSource`；
- `value` / `valueSources`：仅自动结论非空，列出所有支持该值的来源；
- `reason`：人工/无法确定时的机器可读原因码。

`Report` 还提供 `Automatic()`、`Manual()`、`Undetermined()` 三个访问器，方便
下游把自动结论与人工队列分开处理。

## 代码结构

- `auditor/types.go`：来源、字段、观测、冲突、结论与报告类型；
- `auditor/normalize.go`：语言标签与时长归一化（纯标准库）；
- `auditor/policy.go`：可采信来源、时长容差、互证开关；
- `auditor/review.go`、`auditor/review_helpers.go`：审查引擎、冲突检测、聚类；
- `auditor/provider.go`：`Provider` 采集器接口与失败隔离；
- `auditor/*_test.go`：单元测试（覆盖率 91%）；
- `cmd/auditdemo`：可运行演示，输出 JSON 或紧凑中文文本。

## 运行

```bash
go test ./...
go run ./cmd/auditdemo            # 三个媒体项的 JSON 报告
go run ./cmd/auditdemo -text      # 中文紧凑文本
go run ./cmd/auditdemo -media movie-C
```

演示中 `movie-A` 自动高置信通过，`movie-B` 因码流/容器冲突转人工，
`movie-C` 只有文件系统证据而判为无法确定且不输出任何兜底值。

## 接入真实采集器

为每个来源实现 `auditor.Provider`：

```go
type Provider interface {
    Source() auditor.Source
    Collect(mediaID string) ([]auditor.Observation, error)
}
```

然后调用 `auditor.ReviewWithProviders(mediaID, providers, auditor.DefaultPolicy())`。
单个采集器报错会记录到 `Report.SourceErrors` 而不会中断整体审查；所有判定
逻辑集中在本包内，保证四个来源始终按同一套规则被衡量。
