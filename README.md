# rangeaudit

Go 原生的 8-bit luma 范围与黑电平审计工具。它不信任播放器标签，只根据样本分布给出：

- 合法范围候选：`limited (16–235)` 或 `full (0–255)`；
- 修正后的标签，以及标签是正确、错误还是未验证；
- 黑电平读数：合法黑锚点、稳健黑端、抬黑 code 数、严重度；
- 抬黑 / 风格化暗部 / 标签错误的可区分部分与不可区分部分；
- 证据不足时，显式保留 `limited + 标签错误` 与 `full + 抬黑/风格化暗部` 两种解释。

实现只使用 Go 标准库，不依赖 cgo，也不调用外部视频工具。

## 快速开始

```bash
go test ./...
go run ./cmd/rangeaudit -declared full -md report.md -json report.json clip.y4m
```

一次分析多个 PNG、Y4M 或原始 YUV 文件：

```bash
go run ./cmd/rangeaudit -declared full -width 1920 -height 1080 -pixfmt yuv420p \
  -md report.md -json report.json shot-*.yuv
```

`-md -` 把 Markdown 写到标准输出，这是默认行为。`-json` 默认不生成。

## 输入

支持：

- `PNG`、`JPEG`、`GIF`：按 BT.709 系数转成 8-bit luma；
- `Y4M`：8-bit `420`、`422`、`444`、`mono`，分辨率从文件头读取；
- `YUV/RAW`：8-bit `gray`、`yuv420p`、`yuv422p`、`yuv444p`，必须提供 `-width`、`-height` 和 `-pixfmt`。

输入应当是保留原始 code value 的导出，不要在审计前做 range conversion、auto levels、去噪或重新调色。对 `mp4/mov` 等容器，先用保持 luma code value 的流程解出 Y4M 或 8-bit YUV，再交给本工具。

## 批次清单

不同分辨率、不同原始格式的样片用 JSON 清单：

```json
{
  "defaults": {
    "declared": "full",
    "maxSamples": 2000000
  },
  "clips": [
    {"id": "shot-01", "path": "shot-01.y4m"},
    {
      "id": "shot-02",
      "path": "shot-02.yuv",
      "width": 1920,
      "height": 1080,
      "pixelFormat": "yuv420p"
    }
  ]
}
```

运行：

```bash
go run ./cmd/rangeaudit -manifest batch.json -md report.md -json report.json
```

相对路径以清单文件所在目录为基准。`declared` 可取 `full`、`limited` 或 `unknown`。
命令行显式给出的 `-declared` 会覆盖清单中的声明标签。

## 判定方法

工具先统计 256 个 code 的直方图、分位数、`0/16/235/255` 的端点占比，以及 16–235 之外的比例，再分别给两个范围假设计分。

`limited` 的主要证据：

- 没有或几乎没有样本落在 16–235 之外；
- code 16 和/或 code 235 有明显的硬轨道；
- 稳健支撑完全落在 16–235 内；
- 稳健跨度接近 limited 的 219 code。

`full` 的主要证据：

- 有实质比例的样本低于 16 或高于 235；
- code 0 和/或 code 255 有显著堆积；
- 稳健分位同时越过 16 和 235；
- 稳健跨度超过 limited 的 219 code。

分差不足时输出 `ambiguous`，不会把 16–235 直接判成错标。

判决等级：

- `high`：证据差至少 4 分且主导假设至少 8 分；
- `medium`：证据差至少 3 分且主导假设至少 6 分，仍是暂定标签；
- 其他情况为 `ambiguous/low`，修正标签保留两个完整候选。

即使判决为 high，JSON 中仍保留两个候选假设及其分数，便于复核。

## 黑电平

黑电平按每个范围候选分别计算：

- `limited` 的黑锚点是 16；
- `full` 的黑锚点是 0；
- `observedFloor` 是 p0.1 的稳健黑端；
- `pedestalCodes = observedFloor - black anchor`；
- 严重度：`none`（0–1 code）、`soft`（2–3）、`moderate`（4–8）、`strong`（>8）。

因此同样是 16–235 的分布，会产生两种解释：

1. `limited`：16 是合法黑，黑电平 `nominal`，播放器 `full` 标签错误；
2. `full`：16 是抬黑 16 code，或者创作者有意避开纯黑，两种意图仅凭直方图不能完全区分。

`styleReading=low-key-or-stylized-dark-compatible` 只表示暗部分布与风格化低调布光兼容，不宣称已证明创作意图。报告会继续列出“技术性抬黑”和“风格化暗部”两个保留解释。

## 报告

Markdown 报告包含批次结论、逐片总览、原始证据、两个候选假设的分数、黑电平读数和修正标签。JSON 适合接入自动质检，主要字段：

- `assessment.decision`：`limited`、`full` 或 `ambiguous`；
- `assessment.correctedLabel`：修正标签；
- `assessment.labelStatus`：`correct`、`incorrect`、`unverified` 等；
- `assessment.hypotheses`：两个候选及各自分数；
- `stats.percentiles`、`stats.counts`、`stats.fractions`：原始证据。

## 演示

生成一组完全合成的演示样片和报告：

```bash
go run ./cmd/rangedemo -out artifacts/demo
```

输出：

- `artifacts/demo/report.md`
- `artifacts/demo/report.json`
- `artifacts/demo/manifest.json`
- `artifacts/demo/fixtures/*.png`

演示覆盖：16/235 硬轨道错标、纯内部支撑未决、full 抬黑、low-key 风格化暗部。演示图是合成夹具，不是实际客户素材。

## 限制

- 单帧或低对比度镜头可能天然不触及 0/255，这无法仅靠直方图与“故意避开黑”区分。
- 转码、合法范围转换、裁切、去噪和调色会改变端点轨道，应优先审计原始母版。
- `GIF/JPEG` 有损压缩会制造代码值离群点，评分对少量越界有容差，但不能替代原始编码数据。
- 真正的 range 修正应同时检查容器/码流元数据、解码矩阵和显示链路；本工具给出的是基于像素证据的修正建议。
