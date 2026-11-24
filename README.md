# surrounddiag

纯 Go 标准库实现的 5.1 导出取证工具，用于排查“对白发虚、环绕消失”这类
相位、声道映射、中心声道和低频分布问题。工具输出问题声道、时间段、可能根因
和证据；逐声道试听文件与样本级延迟都由本工具完成。

当前工作区没有提供真实 5.1 工程或导出文件，所以仓库内没有任何伪造的真实诊断
时间段。`testdata/demo/` 是带 `truth.json` 的合成对照集，只用于验证检测器。

## 快速开始

```sh
go build -o bin/surrounddiag ./cmd/surrounddiag

# 有参考母版时，能做确定的声道交换、极性和样点延迟判断
./bin/surrounddiag analyze \
  --file export-5.1.wav \
  --reference master-5.1.wav \
  --json report.json \
  --markdown report.md

# 没有参考母版时，只输出可证实的候选，不猜测绝对声道极性
./bin/surrounddiag analyze --file export-5.1.wav --markdown report.md

# 逐声道导出单声道 24-bit PCM 试听；可选播放器按顺序接收每个 WAV
./bin/surrounddiag audition \
  --file export-5.1.wav \
  --outdir previews \
  --start 12.0 --end 18.0 \
  --play ffplay

# 单独查看逐声道样本级延迟
./bin/surrounddiag delay \
  --file export-5.1.wav \
  --reference master-5.1.wav
```

WAV 未写 channel mask，或导出器使用了非标准顺序时，必须显式给出顺序：

```sh
./bin/surrounddiag analyze --file input.wav --order FL,FR,FC,LFE,BL,BR
```

## 检测模型

### 反相与宽声场

“低相关”或“负相关”本身不是反相。反相候选必须同时满足：

- 原始样点波形 `|r| >= 0.85`；
- `r <= -0.75`；
- 绝对值包络相关 `>= 0.70`；
- 在至少一个时间窗口成立，而不是只凭整段均值。

因此正常 L/R 去相关宽声场即使 `r` 接近 0 或偶尔为负，也不会被标成反相。
真正的单路极性翻转会得到接近 `-1` 的波形相关和接近 `+1` 的包络相关。

### 声道交换

提供参考母版时，工具对每个“导出声道 × 参考声道”组合做全采样率互相关，
用唯一分配确定映射。只有强匹配才输出：

- `swapped`：两路互为最佳匹配；
- `misrouted`：导出声道匹配到另一路参考，但未形成互逆配对；
- `inverted`：同一路但极性相反；
- `unmatched`：内容无法可靠对应。

无参考时只能根据 LFE 低频占比、FC 语音频段等输出“疑似映射错误”，不能确定
任意两路非重复素材的真实顺序。

### 中心声道

结合三项证据：

- FC 相对 FL/FR 的电平；
- FC 的 300–3400 Hz 语音频段占比；
- FC 与 FL/FR 的镜像/同相波形关系。

FC 反相通常会表现为下混时对白抵消、对白变虚或后退；FC 缺失则更像中心声像
被折叠进 L/R。两者不会混为同一个结论。

### 低频分布

逐声道计算 20–120 Hz 能量、语音频段能量和高频能量。有参考时，20–120 Hz
能量相对参考下降或增加会给出时间段：

- LFE 低频下降：高通、衰减或未路由；
- 其他声道低频增加：低频错误串入或 LFE/FC 交换；
- FC 低频占主导而语音频段弱：中心与 LFE 可能互换。

## 样本级延迟

互相关在原始采样率、全带宽 FFT 上完成，不做会漏掉奇偶样点的粗抽取。输出的
`lag_samples` 是整数样点，`lag_ms` 由原始采样率换算。

- 声道对表中的正延迟：B 比 A 晚；
- 参考映射表中的延迟：参考相对导出；负值表示导出比参考晚，正值表示导出比
  参考早。

参考比较要求导出与母版采样率、声道数一致；工具拒绝自动重采样，以免把重采样
延迟混进样本级结论。

## 支持范围

- RIFF/WAVE，整数 PCM 8/16/24/32-bit；
- IEEE float PCM 32/64-bit；
- `WAVE_FORMAT_PCM` 和 `WAVE_FORMAT_EXTENSIBLE`；
- 标准 speaker mask、`cmap` 声明和 CLI 顺序覆盖；
- 逐声道试听输出为单声道 24-bit PCM，不做降噪、增益或下混。

## 合成对照集

```sh
./bin/surrounddiag fixture --outdir testdata/demo
./bin/surrounddiag analyze \
  --file testdata/demo/export-5.1.wav \
  --reference testdata/demo/master-5.1.wav \
  --markdown artifacts/reference-report.md \
  --json artifacts/reference-report.json
```

注入真值：

| 问题 | 声道 | 时间段 |
|---|---|---|
| 极性翻转 | FC | 2.000–5.000 s |
| 内容互换 | BL / BR | 全段 |
| 环绕衰减约 23 dB | BL / BR | 5.000–8.000 s |
| LFE 低频衰减 | LFE | 6.000–8.000 s |
| 样点延迟 | FL | 3 samples |

同一份素材的 FL/FR 是独立去相关宽声场，检测器必须不把它判成反相。

## 测试

```sh
go test ./...
go vet ./...
```

测试覆盖 extensible 5.1、24-bit 往返、宽声场反例、镜像反相、奇/偶/负样点
延迟、参考映射、FC 区间反相、BL/BR 交换、环绕衰减、LFE 低频丢失和逐声道
试听导出。
