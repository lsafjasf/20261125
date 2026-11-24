# sdrquant

高位深浮点素材转 8-bit SDR 的可审计量化器。只用 Go 标准库，不依赖外部图像库，也不靠模糊掩盖暗部色带。

## 处理顺序

```text
编码(transfer) -> 预量化抖动 -> 误差反馈 -> 8-bit 舍入
```

对每个通道、每个输出 LSB：

```text
y  = transfer(x) * 255
a  = y + feedback + dither
q  = clamp(round(a), 0, 255)
低bit残差 = y - q
扩散误差  = y + feedback - q
```

抖动只影响当前判决；扩散误差在量化后进入相邻像素的 `feedback`。两者分开记录，避免把抖动噪声误算成量化残差。默认配置是 TPDF 抖动 + Floyd-Steinberg（蛇形扫描）+ 固定种子，可复现。

## 构建与运行

```bash
go build -o sdrquant ./cmd/sdrquant

# 无抖动、无整形的基线，观察原始量化台阶
./sdrquant -synthetic -dither none -shaping none \
  -output baseline.png -diag diagnostics-baseline

# 修复暗部色带并生成诊断
./sdrquant -synthetic -dither tpdf -shaping floyd-steinberg \
  -output sdr.png -diag diagnostics
```

合成图在 75% 处带一个 0.004 的真实源台阶，用于验证红色/黄色分类不会混淆；`-synthetic-step 0` 可关闭它。

真实素材：

```bash
./sdrquant -input source.pfm -output sdr.png -diag diagnostics
./sdrquant -input source.png -transfer display -output sdr.png -diag diagnostics
```

输入支持 PFM（`Pf`/`PF`，自动处理小端和上下翻转）、PNG、JPEG、GIF。PFM 与 16-bit PNG 保留高位深；普通 8-bit 图片按 display-referred 处理。若浮点 PFM 是 scene-linear，加 `-transfer srgb`。

## 可切换项

- `-dither none|rectangular|tpdf`：矩形抖动幅度为 ±0.5 LSB，TPDF 为两个均匀分布之和（±1 LSB）。
- `-dither-strength 1.0`：抖动幅度，单位输出 LSB。
- `-shaping none|floyd-steinberg|atkinson|sierra-lite|jjn|stucki`：量化后的空间误差反馈核。
- `-seed 1 -serpentine=true`：固定噪声和扫描方向，便于 A/B 与回归测试。
- `-transfer display|srgb`：`display` 表示输入已是显示域；`srgb` 先将线性浮点映射到 sRGB。
- `-spectrum-size 512`：诊断 FFT 尺寸，自动取 2 的幂并限制在 64..2048。

## 输出

主输出 `-output` 是未经过任何模糊的 8-bit SDR PNG。`-diag` 目录包含：

- `residual.png`：输入减输出的低 bit 残差，单位 LSB；暖色为正、冷色为负，灰色为零。
- `diffusion.png`：当前像素收到的误差反馈量；`-shaping none` 时为中性灰。它回答“误差被推到了哪里”，不等同于残差图。
- `spectrum.png`：低 bit 残差的二维空间频谱，DC 在中心，显示动态范围 60 dB。色带型误差集中在低频；白化/整形噪声会移到高频。
- `classification.png`：像素来源分类图。
- `metrics.json`：完整数值指标。
- `spectrum.csv`：128 个径向频段的平均功率，便于绘图或比较。

分类图配色：

- 红色：源素材本就有边缘，依据局部源梯度判定。
- 黄色：源素材局部平滑，但基线量化器产生了台阶；这是“原素材不存在的台阶”。
- 蓝色：实际输出相对基线量化值发生了变化，属于抖动/噪声整形引入的变化；局部源平滑时统计为抖动噪声。
- 绿色：源素材自身的平滑纹理。
- 深灰：源素材平滑且无不存在的台阶。
- 品红：源值越界或非有限值。

`metrics.json` 中两组指标不可混用：

- `distinction.baseline_synthetic_edges` 用无抖动、无整形的反事实基线统计量化台阶。
- `distinction.dither_altered_pixels` 统计实际输出被抖动/整形改变的像素。
- `classification.*` 是互斥分类；红、黄、蓝分别回答“源边缘”“量化台阶”“抖动/整形变化”。
- `spectrum.high_frequency_ratio` 与 `spectrum.centroid_cycles_per_pixel` 用于证明噪声被移向高频，而不是被模糊掉。

调整参数后应同时比较分类图、残差 RMS 和高频能量；单个色带指标下降不等于图像被正确修复。

## 测试

```bash
go test ./...
go test -race ./...
go vet ./...
```

测试覆盖 PFM/16-bit PNG 往返、固定种子、越界与 NaN、线性到 sRGB 编码、恒定灰阶的误差扩散、真实源边缘和量化台阶分类、FFT、全部诊断文件输出。
