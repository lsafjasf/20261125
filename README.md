# colorcal — 色卡曝光 / 白平衡 / 通道响应联合拟合

仓库初始内容只有说明文件，因此这里交付一个纯 Go 标准库的可运行校准器和
可复测验收。它不把所有色块按通道均值硬拉齐，而是用稳健非线性最小二乘同时
估计曝光、白平衡和分通道响应曲线，并明确标出高光裁切、暗部低 SNR、局部
反光/离群色块以及参数耦合。

## 模型

输入参考色为线性 sRGB / D65，相机测量值归一化到 `[0,1]`：

```text
mu_ic = (gain_c * reference_ic) ^ gamma_c
gain_c = exposure * white_balance_c
geomean(white_balance_rgb) = 1
```

- `exposure`：全局曝光尺度。
- `white_balance`：在几何平均规范下分解出的 RGB 白平衡。
- `channel_response_gamma`：R/G/B 三条独立幂响应曲线。
- `effective_gain`：`exposure * white_balance_c`，这是数据真正能直接辨识的量。

曝光和白平衡的整体尺度在数学上不可分离；这里固定
`geomean(WB)=1`，并在报告中保留规范说明和完整协方差/相关矩阵。

## 可靠性与耦合

每个色块/通道会输出：

- `highlight_clipped`：测量值达到裁切阈值或输入显式标记裁切，不进入拟合。
- `shadow_low_snr`：预测信号低于 `ShadowSNR * sigma`，暗部校正不确定。
- `local_reflection_or_outlier`：稳健残差被 Tukey/Huber 权值压到接近零。
- `reliable` / `downweighted` / `unreliable`：色块级结论。

`diagnostics` 还包含 Jacobian 条件数、特征值、约化卡方、10×10 参数协方差/
相关矩阵和高相关参数对。窄色调范围会产生接近 `±1` 的
`log_gain_c` / `log_gamma_c` 耦合；工具会把它标为 `critical`，而不是给出
虚假的独立精确参数。

## 快速验收

```sh
bash acceptance.sh
```

脚本会运行 `go vet ./...`、`go test ./...`，构建 CLI，生成观测 JSON 和一份
单独保存的已知生成参数；`fit` 进程只读取观测文件，最后由 Python 对照报告与
生成参数。当前固定种子的典型结果：

```text
exposure=1.304  truth=1.300
WB=[1.282 0.994 0.786]  truth=[1.280 1.000 0.781]
gamma=[0.901 1.029 1.127] truth=[0.900 1.030 1.120]
skin DeltaE2000: mean-align=2.99 -> joint=0.60
```

多随机种子测试还会检查参数误差、肤色改善和高光/反光块检出。

## CLI

```sh
# 生成 24 色合成色卡；truth 只供外部验收，fit 不读取
go run ./cmd/colorcal sample \
  -out testdata/sample-chart.json \
  -truth /tmp/colorcal-truth.json

# 只凭观测拟合
go run ./cmd/colorcal fit \
  -input testdata/sample-chart.json \
  -report /tmp/report.json \
  -corrected /tmp/corrected.json

# 一键演示，输出 observations/report/truth/corrected
go run ./cmd/colorcal demo -out /tmp/colorcal-demo

# 拟合完成后，再用独立子命令对照单独保存的已知生成参数
go run ./cmd/colorcal verify \
  -truth /tmp/colorcal-truth.json \
  -report /tmp/report.json
```

## 输入格式

```json
{
  "schema_version": "colorcal/v1",
  "value_space": "normalized_linear_reference_and_camera_output",
  "saturation_level": 1.0,
  "patches": [
    {
      "id": "light-skin",
      "name": "Light Skin",
      "group": "skin",
      "reference_linear_rgb": [0.539, 0.305, 0.223],
      "measured_rgb": [0.72, 0.43, 0.31],
      "sigma_rgb": [0.008, 0.007, 0.006],
      "clip_rgb": [false, false, false],
      "mask_rgb": [false, false, false]
    }
  ]
}
```

`sigma_rgb` 可省略；省略时使用稳健 MAD 估计。`mask_rgb` 用于人工遮罩。
大于 1 的原始 RGB 会按 `bit_depth` 或 `saturation_level` 自动归一化；
提供 `black_level` 时会在拟合前扣除已知黑位。

## 代码结构

- `fit.go`：观测筛选、log-log 初值、IRLS 稳健拟合入口。
- `optimize.go`：纯 Go Levenberg–Marquardt 和数值 Jacobian。
- `diagnostics.go`：协方差传播、相关矩阵、条件数和耦合排序。
- `report.go`：色块状态、均值基线、肤色与整体色差汇总。
- `color.go`：线性 RGB→XYZ→Lab 和 CIEDE2000。
- `synth.go`：确定性 24 色卡、高光/阴影/局部反光场景和验收真值。
- `acceptance_test.go`、`robustness_test.go`、`coupling_test.go`：已知参数对照。

## 边界

这里的通道响应采用单调幂曲线，适合未严重裁切的 RAW/线性化测量。已经硬裁切
的通道不能从该色块无损恢复；局部反光被当作离群污染而不是额外自由参数，
避免用每块一个 flare 参数把校准模型过拟合掉。若输入只有很窄的色调范围，
报告中的高条件数和 `critical` 耦合比参数点值更重要。
