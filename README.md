# mixloud — 纯 Go 混音短时响度核对

仓库初始化时没有待分析的混音 WAV，因此这里交付一个可直接分析 WAV 的纯标准库 Go 工具，并用内置合成夹具验证“对白 / 音乐 / 效果声、静音、广告突入、双声道反相、局部爆音”都能被单独报告。

## 快速开始

```bash
go test ./...
go build -o bin/mixloud ./cmd/mixloud

# 运行覆盖全部异常的 16 秒合成样例
./bin/mixloud demo --out-dir reports/demo --write-wav

# 分析真实混音；参数放在文件名前后均可
./bin/mixloud analyze final_mix.wav --out-dir reports/final --write-wav

# 有分轨时，对白占比和“对白优先”闪避更准确
./bin/mixloud analyze final_mix.wav \
  --dialogue stems/dialogue.wav \
  --music stems/music.wav \
  --sfx stems/sfx.wav \
  --out-dir reports/final --write-wav
```

输出目录包含：

- `report.md`：三方案对比、交付判断、特殊区间和最响窗口。
- `report.json`：完整机器可读结果。
- `windows.csv`：所有方案/分轨的 400 ms 窗口指标。
- `bypass.wav`、`peak_limit.wav`、`dialogue_priority.wav`：加 `--write-wav` 后写出的 32-bit float WAV。

## 指标口径

- 窗口：默认 `400 ms` 窗、`100 ms` 步进（75% 重叠，符合 BS.1770 400 ms 门限块习惯）；可用 `--hop-ms 400` 改成严格不重叠。
- 整合响度：K 加权、绝对门限 `-70 LUFS`、相对门限 `-10 LU`，以能量平均后转回 LUFS。
- 响度范围：`LRA(400ms) = P95 - P10`，使用绝对 `-70` 和相对整合响度 `-20 LU` 门限；另给 `LRA(EBU 3s)` 作为标准 LRA 参考。
- 局部响度：报告每个 400 ms 窗的 LUFS/RMS/峰值，并给出最大窗响度、P95、中位值和峰值最高窗口，不用全曲平均值掩盖爆音。
- 对白占比：优先用 `--dialogue` 分轨做 VAD；无分轨时使用中置度、20 ms 音节调制、人声频段和过零率启发式。报告对白窗口并集时长占总时长、占有声段比例两个值。
- 静音段：10 ms 帧 RMS 低于 `--silence-db`（默认 `-70 dBFS`），合并后默认只报告不少于 `250 ms` 的区间。
- 广告突入：当前 400 ms 窗响度相对前 2 秒中位数基线突增至少 `--ad-jump-lu 8 LU`，且持续不少于 `600 ms`；峰均比低于约 8 dB 时标记为强压缩广告，高峰均比事件转入爆音/效果声复核。
- 双声道反相：400 ms 窗相关系数 `<= -0.5` 且单声道兼容增益 `<= -6 dB`，合并为区间并给出最低相关系数。

## 三种方案

- `bypass` / 不处理：原始采样透传。
- `peak_limit` / 峰值限制：立体声联动、5 ms 前瞻、0.5 ms 起始和 80 ms 释放的峰值限制器，默认天花板 `--peak-limit-db -1`。
- `dialogue_priority` / 对白优先：有分轨时按对白包络把音乐/效果声衰减 `--dialogue-duck-db -6`；只有立体声混音时，在对白窗内衰减 Side 信号、保留 Mid/对白；单声道无分轨时无法做声源分离，会明确保持透传并在报告中说明。

## Demo 验证结果

内置 demo 的典型结果为：不处理最大采样峰值约 `+0.55 dBFS`、存在满刻度样本；峰值限制后最大峰值为 `-1.00 dBFS` 且满刻度样本为 0；三种方案均检出 1 段 1 秒静音、1 段广告突入和 1 段相关系数约 `-1.0` 的反相区间。具体数值以本机 `go test ./...` 和 `reports/demo/report.md` 为准。

## 边界

- WAV 支持 PCM 8/16/24/32-bit、32-bit float 和常见 `WAVE_FORMAT_EXTENSIBLE`；不直接解码 MP3/AAC，请先转成 WAV。
- 采样峰值和削波计数为精确采样域结果；报告提醒编码前另做真峰值（dBTP）确认。
- 无对白分轨时的对白占比是可解释启发式估计，不能替代人声标注或分轨 VAD。
- 全部功能只使用 Go 标准库，无 cgo、无第三方依赖。

Repository initialized for 20261125.
