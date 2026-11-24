# Log 高光截断诊断与肩部修复

这是一个只使用 Go 标准库的可复现实验。它从编码曲线锚点和肩部参数出发，定位
“窗外高光整片灰白、暗部同时抬起”的转换错误，并用三组不同曝光、不同已知 Log
曲线的样本比较修复前后。

## 结论

截断不在相机 Log 编码端，而在**显示转换入口的 legal-white 归一化**：

```text
旧路径: log code -> code / (940/1023) -> clamp(1) -> sRGB
修复后: log code -> inverse Log -> scene-linear -> shoulder -> sRGB
```

旧路径把 Log 码当作显示线性值。这一步同时造成两个症状：

- 低于肩部的暗部没有经过反 Log，直接套 sRGB，黑位被明显抬高。
- `940/1023` 之后的有效 Log 码在反 Log 之前被钳成 1，肩部信息丢失，窗前高光
  被压成同一种灰白。

三组样本的源码都低于编码上限 `code 1.0`，但均有两个可恢复层次高于
`940/1023`。修复后源端高光层次为 `8/8`，旧路径为 `7/8`，且旧路径最后一个
高光间隔正好为 `0`。

## 曲线反证

工具内置三段分段函数，18% 灰分别锚定到公开参考值：

| 曲线 | 18% 灰码 | `940/1023` 场景 | `code 1.0` 场景 |
|---|---:|---:|---:|
| Sony S-Log3 | 0.41056 | 18.495x | 38.421x |
| ARRI LogC3 EI 800 | 0.39101 | 25.863x | 55.080x |
| ACEScct | 0.41359 | 83.201x | 222.861x |

因此 `code / (940/1023)` 在场景线性中远未到各自编码上限，不能作为显示白。
真正的截断点是该归一化之后的 `clamp(0,1)`，不是 `Curve.Encode`。

## 肩部约束

默认修复肩部参数是：

```text
knee               = 0.70
output ceiling     = 0.90  (渐近上限，不硬钳)
calibration input  = 128 scene-linear
calibration output = 0.88
```

输入 `<= 0.70` 时输出逐值恒等，所以本实验的 18% 中间调误差为精确的 `0`。
拐点以上使用有理单调肩部；它不把高光统一压到某个固定码值，也不触碰 `code
1.0` 的源端饱和区。若源端已经饱和，报告会单独计为不可恢复。

有理肩部：

```text
u     = max(0, scene - knee)
scale = (calibration_input - knee) * (1 - r) / r
r     = (calibration_output - knee) / (ceiling - knee)
out   = knee + (ceiling - knee) * u / (u + scale)
```

该式在拐点处连续、导数始终为正；`scale=14.1444` 时 `128 -> 0.88`，观感上保留
高光顺序，而不是把整片窗外压成同一亮度。

## 样本

| 组 | 曲线 | EV | 源端高光 | 旧路径 | 修复 |
|---|---|---:|---|---|---|
| low-key | S-Log3 | -1 | 0.5..32 | 2 层早钳，窗顶 1.0000 | 8/8 层，窗顶 0.9250 |
| base | LogC3 | 0 | 0.625..40 | 2 层早钳，窗顶 1.0000 | 8/8 层，窗顶 0.9295 |
| high-key | ACEScct | +1 | 2..128 | 2 层早钳，窗顶 1.0000 | 8/8 层，窗顶 0.9453 |

修复前后对照图：`artifacts/before-after.png`。三行依次对应上表，左列为旧路径，
右列为修复路径；左侧小方块是暗部，大方块是 18% 锚点，右侧八条竖带是窗外高光。

## 运行

```bash
go test ./...
go run ./cmd/logfix -out artifacts
```

常用肩部参数可以直接覆盖：

```bash
go run ./cmd/logfix \
  -out artifacts \
  -knee 0.70 -ceiling 0.90 \
  -cal-input 128 -cal-output 0.88
```

输出：

- `artifacts/report.md`：逐曲线、逐曝光的中文审计报告。
- `artifacts/metrics.csv`：修复前后指标，一行一个曝光样本。
- `artifacts/trace.csv`：每个 patch 在编码、解码、旧路径和修复路径中的全部中间值。
- `artifacts/before-after.png`：16-bit sRGB 对照图。

## 验证重点

`go test ./...` 检查：

- 三个 Log 曲线的 18% 灰、黑位锚点和 encode/decode 往返。
- 肩部在 knee 以下恒等、以上严格递增，且不会提前到达上限。
- 三组曝光都能把早期显示钳位诊断为 `before inverse Log`。
- 修复后高光层数等于源层数，末端间隔大于零，窗顶保留 headroom。
- 修复前后 18% 中间调及暗部误差都小于 `1e-12`。
- 源端 `code 1.0` 饱和会被识别，而不是把不可恢复细节“修复”为假层次。

## 参数参考

- Sony S-Log3 曲线与 18% / 95 / 420 锚点：
  <https://festivalluts.com/academy/exposing-sony-slog3>
- ARRI ALEXA LogC3 曲线参数与 EI 800 附录：
  <https://www.arri.com/resource/blob/31918/66f56e6abb6e5b6553929edf9aa7483e/2017-03-alexa-logc-curve-in-vfx-data.pdf>
- ACEScct 分段函数与 `0.0078125` / `10.5402377416545` 参数：
  <https://docs.acescentral.com/encodings/acescct/>
