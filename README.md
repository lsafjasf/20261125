# cutluma：十段分切色彩转换的切点亮度台阶诊断与修复

同一场景切成 10 段、每段独立做色彩转换后，9 个拼接点出现亮度台阶。本工具：

1. 检查每段**黑点、白点、曝光与转换参数**是否一致（声明参数精确比对 + 输出分位数实测）；
2. 在每个切点做跨切点曲线取证，定位差异是**切点前/源端已有**（正常光变、真实跳变）
   还是**分段转换参数失配造成**；
3. 对转换失配段输出统一配方的**修复参数与 257 点 1D-LUT**，并闭环验证台阶消除；
4. 输出跨切点曲线 CSV、全时间线 CSV、JSON 报告、SVG/PNG 曲线图和中文 Markdown 报告。

**全部算法只用 Go 标准库**（`encoding/csv`、`encoding/json`、`math`、`sort` 等），
无第三方依赖，`go test ./...` 可离线运行。

## 快速开始

```bash
# 运行内置确定性场景（10 段 × 120 帧，注入 6 类切点真因），产物写入 artifacts/
go run ./cmd/cutcheck

# 跑测试（判定准确率、抗误判、修复率、JSON 回环等）
go test ./...

# 分析自己的任务 JSON
go run ./cmd/cutcheck -in my_job.json -out out/
```

内置场景的 9 个切点与诊断结果（`go run` 可复现）：

| 切点 | 帧 | 脚本注入真因 | 诊断 |
|---:|---:|---|---|
| 0 | 120 | EV +0.22 档漂移 | `conversion_mismatch` |
| 1 | 240 | 黑点/白点漂移 | `conversion_mismatch` |
| 2 | 360 | 跨切点平滑自然光变，参数相同 | `natural_light_change` |
| 3 | 480 | EV −0.18 与 power 漂移 | `conversion_mismatch` |
| 4 | 600 | 自然光斜坡 + 转换失配叠加 | `mixed` |
| 5 | 720 | 转换 gamma/power 漂移 | `conversion_mismatch` |
| 6 | 840 | 切点前就开始的宽缓光涌，参数相同 | `natural_light_change` |
| 7 | 960 | 白点与 power 漂移 | `conversion_mismatch` |
| 8 | 1080 | 源端真实硬跳变，参数相同 | `preexisting_step` |

合成自检 `synthetic truth matched: true`，9/9 与注入真因一致。

## 产物（`artifacts/`）

- `summary.md`：中文诊断报告（参数表、切点表、修复验证表）。
- `summary.json`：机器可读结论（分类切点列表、最大/平均修复率、真因匹配）。
- `report.json`：完整 `Report`（每段实测黑点/白点/曝光差/相对 gamma，每切点全部特征）。
- `repair_params.json`：每段修复模式、统一配方、校正 EV、257 点 `lut` 与接缝 trim、验证结果。
- `input_job.json`：本次分析输入（可作为 `-in` 模板，回灌结果一致）。
- `segment_report.csv` / `cut_report.csv`：两张明细表。
- `curves_cross_cut.csv`：**跨切点曲线**，列为
  `cut,rel_frame,abs_frame,segment,luma_raw,luma_repaired,source_luma`，每个切点 ±24 帧。
- `curve_timeline.csv`：全 1200 帧原始/修复/源中位亮度与黑/白分位。
- `curves.svg` / `curves.png`：全时间线 + 3×3 切点放大图（红=原始输出，蓝=修复，绿虚线=源）。

## 判定原理

转换模型（归一化亮度域）：

```
y = clamp( ((x · 2^ExposureEV − Black) / (White − Black)) ^ TransferPower )
```

每段报告两类参数：

- **声明参数** `segments[].params`：转换任务实际使用的 B/W/EV/Power，按容差逐字段比对目标配方；
- **实测描述子**：段内各帧分位数的稳健中位数——黑点 `q0.005`、白点 `q0.995`、
  中位亮度、相对参考段的有效曝光档 `log2(med/medRef)`、log-log Theil–Sen 拟合的相对 gamma。

每个切点在 ±24 帧窗口（边界受段长约束）计算：

- `fitGap`：左右两条 Theil–Sen 稳健直线在切点处的趋势校正间隙；
- `zScore`：最近 5 帧中位电平差相对稳健噪声 SE 的显著性；
- `outlierZ`：**切点处相邻增量相对窗内所有段内相邻增量的稳健离群分数**。
  真台阶约 10–37，平滑光变（含 20 帧 0.14 幅度的陡坡）低于 1，门限 6；
- `trendChange` / `preCutDrift` / `postCutDrift`：窗内斜率变化与切点前 48 帧漂移，
  用于识别连续光变（“差异在切点前已存在”）；
- `predictedStep`：用相邻段声明参数和源锚点预测的跳变量，与实测间隙互相印证；
- `source.*`：若提供转换前分位数曲线，在源域重复同一分析，直接完成归因。

分类：

- `conversion_mismatch`：边界不连续显著 + 参数失配且预测吻合 + 源域连续；
- `mixed`：转换台阶与自然光变共存，只修台阶、保留趋势；
- `natural_light_change`：间隙不显著但斜率/长窗漂移显著，不动电平；
- `preexisting_step`：源域本身存在同号同幅硬跳变（真实切换/灯光突变），不属转换缺陷；
- `consistent` / `ambiguous`：无异常 / 证据不足（例如无参数且无源曲线的硬台阶）。

“正常光变不被误判”由三重机制保证：判定门是**趋势校正后的边界间隙 + 增量离群**，
而不是相邻帧亮度差；平滑斜坡的边界增量与邻近增量同族，过不了离群门；
静态分段 LUT 只统一转换配方，不会压平段内光变。测试 `TestSteepSmoothLightChangeIsNatural`
专门用 20 帧内 0.14 的陡坡做对抗验证。

## 修复参数

首选参数级精确修复：对每段生成

```
LUT_k(v) = TargetParams.Apply( Params_k.Inverse(v) )
```

即“用该段配方求逆回到源域，再按统一配方转换”，导出 257 点 LUT。
无声明参数时退化为段分位数到参考段分位数的单调分段线性 LUT（`mode=quantile_lut`）。
`trims[]` 给出残留间隙的短斜坡补偿（左右各承担一半，默认 6–8 帧）。
闭环 `verification[]` 记录每个切点修复前后间隙与趋势保持率：
内置场景转换类台阶平均消除约 99%，正常光变趋势保持约 90–115%，源端硬跳变保留 >98%。

## 输入 JSON 契约（摘要）

```json
{
  "frameCount": 1200,
  "quantileLevels": [0.005, 0.02, 0.1, 0.25, 0.5, 0.75, 0.9, 0.98, 0.995],
  "cuts": [120, 240],
  "frames": [{"frame": 0, "out": [9 个输出分位], "src": [9 个转换前分位，可省略]}],
  "segments": [{"index": 0, "start": 0, "end": 120,
                "params": {"black": 0.02, "white": 0.96, "exposureEV": 0, "transferPower": 1}}]
}
```

真实管线中，`out` 可由每帧亮度直方图取分位数得到（无需原始帧进入本工具）；
`src` 与 `params` 为可选项，但提供后归因最直接。

## 代码结构

- `params.go`：转换模型、逆模型、重转换 LUT、参数容差；
- `stats.go`：分位数、MAD、Theil–Sen 稳健拟合；
- `job.go`：输入契约与 JSON IO；
- `scenario.go`：确定性十段合成场景（含自然光变与源端跳变干扰）；
- `analyze.go` / `detect.go`：分段测量、一致性与切点归因；
- `repair.go`：修复 LUT、接缝 trim、闭环验证；
- `curves.go` / `tables.go` / `artifacts.go` / `markdown.go` / `svg.go`：CSV/JSON/MD/SVG 输出；
- `cmd/cutcheck/main.go`：命令行入口。

