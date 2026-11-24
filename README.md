# LUT 串联顺序差异：A→B 与 B→A

两个非线性 3D LUT 串联时通常不满足交换律：`B(A(x)) ≠ A(B(x))`。本项目用**纯 Go 标准库**实现两个 LUT、两种串联顺序、差异图、密集区域扫描、中文解释报告，以及针对中性色 / 高饱和色 / 接近边界值样本的测试，并显式验证“把两个结果平均”不能掩盖顺序分歧。

## 运行

```bash
go run ./cmd/lutdiff     # 生成 artifacts/ 下的全部图像、CSV 与 REPORT.md
go test ./...            # 运行全部验证测试
```

无需任何第三方依赖，不需要网络拉取模块。

## 两个 LUT

- LUT A：逐通道伽马曲线（gamma `0.68`），非线性。
- LUT B：Rec.709 亮度加权的饱和度矩阵（增益 `1.45`）+ `[0,1]` 裁剪。
- 均烘焙为 `33³` 3D LUT，使用三线性插值；报告区域统计基于 `41³` RGB 立方体扫描。

不可交换的根因：矩阵/饱和度运算 `M` 与非线性曲线 `g` 满足 `M(g(x)) ≠ g(Mx)`；边界裁剪 `C` 进一步引入 `C(M(C(g(x))))` 与 `C(g(C(Mx)))` 的顺序差异。

## 代码结构

- `lut.go`：`LUT3D`、烘焙与三线性插值。
- `ops.go`：LUT A/B 设计函数、两种顺序管线、固定样本与区域分类。
- `color.go`：RGB 距离、8-bit 码值差、sRGB→Lab 与 CIE76 ΔE、热色标。
- `chart.go` / `image.go` / `legend.go`：程序化测试图卡、管线渲染、差异图与图例。
- `analysis.go`：固定样本分析与 RGB 立方体区域统计。
- `report.go` / `report_sections.go`：`artifacts/REPORT.md` 与 `samples.csv` 生成。
- `lut_test.go`：恒等 LUT、节点精度、三类样本、平均反掩盖、色域与分类测试。
- `cmd/lutdiff/main.go`：生成入口。

## 主要产物（`artifacts/`）

- `REPORT.md`：完整中文解释报告（含样本表、区域统计、Top 差异点、平均法分析）。
- `source_chart.png`：色相×饱和度平面 + 灰/R/G/B/近黑/近白六条渐变带。
- `order_A_then_B.png`、`order_B_then_A.png`：两种顺序的真实结果。
- `diff_map.png` / `diff.png`：同刻度差异热图（带数值图例）。
- `diff_amplified_x5.png`：放大 5 倍的灰度差异图。
- `average_AB_BA.png`：两个顺序的逐像素平均（仅作反证，不是合法管线结果）。
- `average_error_vs_AB.png`：把平均冒充 A→B 时的误差热图（与差异图同刻度，强度减半）。
- `samples.csv`：固定样本的浮点数值结果。
- `summary.json`：各区域可交换性判定与平均法误差的机器可读结论。

## 结论摘要

- 内部中性灰轴 `R=G=B` 上两种顺序严格一致；近黑首格可能有 ≤2.5 码值的 LUT 裁剪/插值次级误差。
- 高饱和区域显著不可交换：最坏约 47 个 8-bit 码值、ΔE*ab 约 11.3。
- 接近边界（通道贴近 0/1，尤其次通道近零）同样高度顺序敏感。
- 平均 `(AB+BA)/2` 相对指定参考顺序 A→B 的误差恰为顺序差的一半，是第三个结果；大量输入上仍超过 ±1 码值验收线，不能用来掩盖分歧。
