# hlgmonitor — HLG gamma / 显示峰值 / OOTF 可调管线

工作区没有提供实际 HLG 媒体文件，因此程序生成一段确定性的 HLG 测试素材：
0→1 灰阶斜坡、`E′=0.75…1.05` 的八组彩色高光和超白信号。所有峰值档位处理的是
完全相同的 HLG 码值，只改变显示端的 `Lw`、系统 gamma、OOTF `α` 与 OOTF 形式。

仅使用 Go 标准库；SVG 图表、CSV 和 JSON 均由 `go run .` 直接生成。

```bash
go test ./...
go run .
```

默认写入 `artifacts/hlg-pipeline/`：

- `grayscale_curves.svg`：绝对 nit 灰阶曲线、归一化形状对照和错误标定虚线。
- `color_highlights.svg`：八组彩色高光 × 五个码值 × 三档峰值。
- `peak_comparison.svg`：`1000 / 2000 / 4000 nit` 三栏对照，含相对峰值预览和共同绝对标尺。
- `curve_data.csv`、`highlights.csv`：未舍入的中间值与绝对 nit。
- `report.json`：配置、公式和关键检查点。

目录中同时放置了由当前 SVG 导出的同名 `.png` 预览，便于不支持 SVG 的查看器直接打开；
可复跑的主输出仍是 SVG/CSV/JSON。

## 可调参数

```bash
# 三档扩展峰值，例如更极端的 1000 / 4000 / 10000 nit
go run . -peaks 1000,4000,10000 -out artifacts/hlg-10k

# 强制所有档位使用同一个系统 gamma（复现“固定 1.2”配置错误）
go run . -gamma 1.2

# 切换 OOTF；scene-power 用于观察缺少 Ys^(gamma-1) 通道耦合时的彩色偏差
go run . -ootf scene-power
go run . -ootf linear-scene

# 调整黑位和共同 SDR 预览白基准
go run . -black-nit 0.01 -reference-nits 1000
```

参数包括 `-peaks`、`-gamma-law auto|standard|extended`、`-gamma`、
`-ootf bt2100|scene-power|linear-scene`、`-black-nit`、`-reference-nits`、
`-wrong-alpha`、`-wrong-gamma` 和 `-out`。

## 管线和公式

对每个 HLG 码值 `E′`（Rec.2100/BT.2100，允许超白）执行：

```text
E′c = max(0, (1-β)E′ + β)
Es  = InverseHLGOETF(E′c)
Ys  = 0.2627Rs + 0.6780Gs + 0.0593Bs
RD/GD/BD = α · Ys^(γ-1) · Rs/Gs/Bs       # BT.2100 HLG OOTF
Y_D = 0.2627RD + 0.6780GD + 0.0593BD    # 绝对亮度，单位 nit
```

默认 `α=Lw`，因此灰阶 `E′=1` 标定到该显示器的标称峰值；`β` 由黑位反推：

```text
β = sqrt(3 (LB/Lw)^(1/γ))
```

系统 gamma 支持：

```text
standard: γ = 1.2 + 0.42 log10(Lw/1000)
extended: γ = 1.2 · 1.111^log2(Lw/1000)
auto:     400–2000 nit 用 standard，超出生产监听范围用 extended
```

## 默认数值对照

同一灰阶码值的 OOTF 原始 `Y` 亮度如下（未做峰值裁切）：

| HLG 码值 | 1000 nit / γ1.200 | 2000 nit / γ1.326 | 4000 nit / γ1.481 | 错误：4000 面板但 α=1000、γ1.20 |
| ---: | ---: | ---: | ---: | ---: |
| `E′=0.50` | 50.7 nit | 74.1 nit | 100.8 nit | 50.7 nit |
| `E′=0.75` | 203.2 nit | 343.5 nit | 559.4 nit | 203.2 nit |
| `E′=1.00` | 1000 nit | 2000 nit | 4000 nit | **1000 nit（应为 4000）** |
| `E′=1.05` | 1389 nit，超峰 | 2876 nit，超峰 | 6000 nit，超峰 | 1389 nit |

三档正确管线并不是“同一个绝对曲线复制三份”：`α=Lw` 抬高绝对上限，同时随峰值
增大的 `γ` 压回中间调，使不同峰值显示器的主观对比度保持接近。超白 `E′>1` 必然
超过标称峰值；图中红框表示本工具按通道硬裁切，真实显示设备还可能使用色调映射。

## 为什么曲线平滑，绝对亮度仍然错误

“平滑”只验证 inverse OETF 和 OOTF 是否连续、导数是否稳定、有没有量化色带；它
不能验证整体尺度因子 `α`。错误示例把物理峰值为 `4000 nit` 的显示器仍按
`α=1000 nit` 和固定 `γ=1.2` 计算：

- 归一化曲线 `L/α` 与 1000 nit 参考档完全重合，视觉上同样平滑；
- `E′=0.75` 都是 `203.2 nit`，但 4000 nit 正确管线应为 `559.4 nit`；
- `E′=1` 的端点只有 `1000 nit`，相对物理峰值差了 4 倍；
- 如果绘图时再把每条曲线除以各自最大值，所有端点都会变成 1，错误会被彻底掩盖。

因此验收必须同时检查：绝对 nit 坐标轴、`E′=0.5/0.75/1` 检查点、`α=Lw` 或明确的
标定值，以及 `highlights.csv` / `report.json`。`peak_comparison.svg` 的下排灰阶
使用共同的 `1000 nit` SDR 标尺，扩展峰值档超过参考白的部分会直接裁切，从而暴露
“形状正常但绝对光更亮”的差异。

## 图表

![灰阶曲线](artifacts/hlg-pipeline/grayscale_curves.svg)

![彩色高光](artifacts/hlg-pipeline/color_highlights.svg)

![三档峰值对照](artifacts/hlg-pipeline/peak_comparison.svg)

色板是 Rec.2020 线性 nit 经 BT.2020→BT.709 矩阵和 sRGB OETF 后的近似预览，不是
色彩管理输出；超出 sRGB 色域或显示峰值的颜色会被裁切。精确亮度以 CSV/JSON 为准。

真实素材接入时，先把 10-bit 有限范围码值归一化到 `[0,1]`，再对每个像素的
`R′G′B′` 调用 `Config.Decode(RGB{...})`；Go 标准库可解码 PNG，但不能直接解码
HEVC/HLG 视频，视频帧可由外部解封装/解码为 PNG 或原始 RGB 后送入同一管线。

公式参考 ITU-R BT.2100-3 的 HLG Reference OETF/OOTF 与 system gamma 注记：
<https://www.itu.int/dms_pubrec/itu-r/rec/bt/R-REC-BT.2100-3-202502-I!!PDF-E.pdf>。
