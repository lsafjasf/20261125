# tearcheck

可复核、可复现的横向撕裂证据链工具。只使用 Go 标准库，不依赖第三方包。

## 解决什么问题

旧诊断把“横向亮度变化”直接当成撕裂：镜头内横向光带、灯光扫过、字幕/UI 横条都会变红。新逻辑先区分两类现象：

- **光效阶跃（photometric step）**：同一条横线上亮度整体抬高或降低，差值在整行上基本一致，横向纹理没有发生位移。归为 `light_effect`，不判撕裂。
- **结构错位（structural seam）**：横线上下两带的横向梯度剖面在非零水平位移处显著更吻合，且错位宽度稳定。才生成 `seam` 证据。

单帧孤立的单缝只标为 `uncertain`；连续/间歇出现，或同一帧出现两条方向相反、位置成对的缝（位移带），才标为 `tear`。审片员可以逐帧确认、清除或重新标为不确定。

## 快速开始

### 输入是解码后的帧序列

Go 标准库不包含 MP4/MOV 解复用和 H.264/H.265 解码器。因此本工具分析**已解码帧 + 原始 PTS 清单**；视频解码本身不属于诊断逻辑。可以用现有转码流程把帧和 PTS 导出，再交给本工具。

目录结构：

```text
frames/
  frames.json
  f0001.png
  f0002.png
  ...
```

`frames.json` 示例：

```json
{
  "schema": "tear-manifest/v1",
  "fps_num": 30000,
  "fps_den": 1001,
  "frames": [
    {"id": "f0001", "file": "f0001.png", "pts_us": 0, "duration_us": 33367},
    {"id": "f0002", "file": "f0002.png", "pts_us": 33367, "duration_us": 33367},
    {"id": "f0003", "file": "f0003.png", "pts_us": 66733, "duration_us": 33367}
  ]
}
```

规则：

- `file` 必须是相对 `--input` 的路径，不能是绝对路径，也不能用 `..` 跳出输入目录。
- `pts_us` 使用整数微秒；也接受 `pts_ms` / `duration_ms`。
- `id` 可选，缺省由相对文件名生成。所有 `id` 必须唯一。
- 时间戳必须不递减；完全相同的 PTS 可用于重复帧。
- 如果所有帧都省略 `pts_*`，则用 `fps_num/fps_den` 和帧顺序推导；如果只有部分帧带 PTS，则报错。
- 没有 `frames.json` 时，会按路径排序扫描 `.png/.jpg/.jpeg`，用 `--fps-num/--fps-den` 生成近似时间戳。正式审片应使用真实 PTS 清单。

### 构建和运行

```bash
go build -o tearcheck ./cmd/tearcheck

./tearcheck analyze --input frames --out report.json
./tearcheck verify  --input frames
```

`verify` 会对同一输入连续分析两次，逐字节比较报告，输出：

```text
identical=true
sha256=...
```

常用参数：

```text
--max-shift N        最大水平位移搜索范围，默认 48 像素
--band-rows N        接缝上下各取多少行，默认 6
--min-match-bp N     位移后剖面匹配分数下限，默认 6800/10000
--min-gain-bp N      相对零位移的最小增益，默认 2200/10000
--min-continuous N   连续事件至少需要多少个非重复帧，默认 3
--fail-on-tear       最终仍有撕裂时退出码为 3
```

`max_shift` 和 `band_rows` 会按输入分辨率自动收缩，报告中的 `settings` 是实际生效值。

退出码：`0` 成功，`1` 运行错误，`2` 参数错误，`3` 最终报告仍有撕裂（仅 `--fail-on-tear`）。

## 审片员逐帧修正

先查看需要关注的帧：

```bash
./tearcheck review list --report report.json
./tearcheck review list --report report.json --state all
```

交互式逐帧审片：

```bash
./tearcheck review walk \
  --report report.json \
  --review reviews.json \
  --reviewer alice \
  --notes
```

快捷键：`c` 确认撕裂，`x` 清除，`u` 不确定，`s` 跳过，`q` 保存退出。审片结果写入 `reviews.json`，下一次分析时应用：

```bash
./tearcheck analyze --input frames --review reviews.json --out report.json
```

也可以非交互地逐帧修改：

```bash
./tearcheck review set \
  --report report.json \
  --review reviews.json \
  --frame f0004 \
  --decision clear \
  --note "镜头内横向光带" \
  --reviewer alice

./tearcheck review unset --review reviews.json --frame f0004
```

审片文件格式：

```json
{
  "schema": "tear-review/v1",
  "entries": [
    {"frame_id": "f0004", "decision": "clear", "note": "镜头内横向光带", "reviewer": "alice"},
    {"frame_id": "f0008", "decision": "confirm_tear", "reviewer": "alice"}
  ]
}
```

可用的 `decision`：`confirm_tear`、`clear`、`uncertain`。审片结果按 `frame_id` 关联；未知 `frame_id` 会直接报错，避免静默写错帧。

## 报告里的证据链

报告为 `tear-evidence/v1`，所有帧按时间顺序输出。

顶层字段：

- `input`：帧数、唯一内容帧数、分辨率、时间戳来源、清单哈希。
- `settings`：本次实际使用的全部检测阈值。
- `review`：审片文件的应用条数和规范化哈希。
- `summary`：机器结论与审片后结论的计数。
- `events`：跨帧聚合的撕裂事件。
- `frames`：逐帧证据。

逐帧字段：

| 字段 | 含义 |
| --- | --- |
| `pts_us` / `pts_ms` / `timecode` | 原始时间戳及可读时间码 |
| `hash` | 灰度帧内容 SHA-256 |
| `duplicate` / `duplicate_of` / `duplicate_run` | 重复帧、首个同内容帧、连续重复序号 |
| `machine.classification` | `tear` / `uncertain` / `light_effect` / `clean` / `duplicate` |
| `machine.continuity` | `continuous` / `intermittent` / `isolated` / `none` / `duplicate` |
| `machine.event_id` | 所属跨帧事件 |
| `machine.width_px` | 该帧最大水平断层宽度 |
| `machine.seams[]` | 行号、带符号位移、宽度、匹配分、零位移分、增益、偏置比、亮度阶跃、结构差、纹理强度 |
| `machine.bands[]` | 两条反向接缝形成的位移带：上下行号、带高、两侧位移、宽度 |
| `machine.light_edges[]` | 被识别为整行光效阶跃、未判撕裂的证据 |
| `review` | 审片员的决定、备注、姓名 |
| `final` | 应用审片后的最终分类：`tear` / `not_tear` / `uncertain` / `duplicate` |

跨帧事件 `events[]`：

- `start_pts_us` / `end_pts_us`、`start_index` / `end_index`、`frame_ids`。
- `continuity`：事件连续性分类。
- `width_px`（中位数）、`min_width_px`、`max_width_px`。
- `evidence[]`：每个非重复证据帧的行号、位移、宽度和匹配指标。
- `final_classification`、`reviewed_frames`。

重复帧会被标记并关联到首个同内容帧，但**不会作为新的连续性证据**加入事件，避免冻结帧把一次撕裂放大成很长的连续事件。

## 分类规则

1. 将帧转灰度，计算每行横向梯度。
2. 对每个候选接缝行，取上下各 `band_rows` 行，形成两条横向梯度列剖面。
3. 在 `[-max_shift, +max_shift]` 内寻找剖面最佳匹配位移。
4. 接缝行本身计算“整行亮度阶跃”和“结构差”：
   - 阶跃大、结构差小、偏置比高：横向光效，记入 `light_edges`，不判撕裂。
   - 结构差明显、偏置比低：才可能成为 `seam`。
5. 只有 `|位移| >= min_shift`、匹配分和相对零位移增益都过阈值时，才生成接缝。
6. 单帧孤立单缝机器标 `uncertain`；同一帧上下两条反向缝形成位移带，或事件跨帧连续/间歇出现，标 `tear`。
7. 事件连续性：
   - `continuous`：至少 `min_continuous` 个非重复帧，且逐个相邻。
   - `intermittent`：至少两个非重复证据帧，但中间有间隔。
   - `isolated`：只有一个非重复证据帧。

因此横向光效会出现在 `light_effect`，普通无接缝帧为 `clean`，两者的 `final` 都是 `not_tear`，不会标红；审片员仍可对任何帧单独确认或清除。

## 确定性保证

对相同二进制、相同 Go 版本和相同输入，重复运行得到逐字节相同的 JSON：

- 不写入运行时间、随机数、临时路径或绝对路径。
- 帧、接缝、光效、事件、审片条目全部稳定排序；并列时使用显式次序。
- 指标使用整数或 basis point，避免浮点格式化差异。
- 事件 ID 按帧序和接缝行序生成。
- 审片文件先规范化再计算哈希，条目顺序和空白不影响结果。
- 输出通过临时文件 + 原子替换写入。

验收：

```bash
go test ./...
./tearcheck verify --input frames
```

`go test` 覆盖：光效不误报、连续撕裂、单帧位移带、孤立单缝、重复帧不放大事件、逐帧审片、清单校验、毫秒时间戳、全灰度范围、报告字节级一致性和 CLI 端到端流程。

## 代码结构

```text
cmd/tearcheck/         CLI：analyze / verify / review list|walk|set|unset
internal/frameio/      帧清单、PNG/JPEG 解码、灰度化、内容哈希
internal/detect/       接缝/光效/位移带检测，纯整数指标
internal/analyze/      重复帧、事件连续性、审片应用、报告汇总
internal/review/       审片文件规范化、哈希、原子写入
```

## 限制

- 本工具不解码 MP4/MOV；输入必须是已解码帧和 PTS 清单。
- 单帧孤立单缝默认是 `uncertain`，需要审片员确认；这是为了避免再次把所有横向异常都自动标红。
- 阈值针对横向/近似横向撕裂。大幅旋转、强压缩块效应、整屏转场仍可能产生 `uncertain` 或需人工清除的候选。
- 审片粒度是帧；事件级结论由该事件内所有帧的最终结论汇总得到。
