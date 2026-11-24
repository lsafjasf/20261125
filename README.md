# wavchan

纯 Go 标准库实现的未压缩 PCM/WAV 声道顺序检查与修复工具。工具不会猜测真实声道，只输出检测证据，由使用者选择映射。

## 支持范围

- RIFF/WAVE 容器和整数 PCM：8、16、24、32 bit。
- WAVE_FORMAT_PCM，以及 PCM 子类型的 WAVEFORMATEXTENSIBLE。
- 交错存储的多声道 `data` payload。
- 自定义 `cmap` chunk 用于记录有序声道声明；标准 `fmt` 声道掩码仍只用于读取证据。

## 检测证据

`inspect` 是只读操作，输出：

- 整文件和 PCM payload 的 SHA-256。
- RIFF 大小、chunk ID、偏移和大小。
- `fmt` 声道数、采样率、位深、block align 和声道掩码。
- 当前声明顺序（优先 `cmap`，其次 `fmt` 掩码，最后 `CHn` 回退名）。
- 每声道首样本、最小值、最大值、非零数、RMS 和独立 SHA-256。

这些证据可以帮助区分声道，但工具不会自动把统计特征解释成 FL、FR 或 LFE。

## 互斥修复模式

一次修复只能选择一种模式：

- `samples`：只重排未压缩 PCM 样本，保留 `fmt` 和 `cmap` 字节语义不变。
- `declaration`：只插入或替换 `cmap` 有序声明，PCM payload 的 SHA-256 必须保持不变。

程序会回读修复结果并强制校验上述不变量；任何尝试同时修改样本和声明的路径都会返回错误。

## 命令

```sh
go build -o wavchan .

./wavchan inspect --file input.wav

./wavchan repair \
  --file input.wav \
  --mode samples \
  --mapping 2,1 \
  --dry-run

./wavchan repair \
  --file input.wav \
  --mode declaration \
  --names FR,FL \
  --yes

./wavchan rollback \
  --file input.wav \
  --backup input.wav.bak-...
```

不带 `--yes` 时，工具会展示预览并要求输入 `yes` 才写入。非 TTY 环境必须显式提供 `--mode`，样本模式还需要 `--mapping`，声明模式还需要 `--names`。

交互式运行时，工具会在检测证据后列出候选排列；6 声道及以下给全排列，更高声道给保持原顺序和相邻交换项，也可以手动输入任意合法排列。

`--mapping` 使用 1-based 源位置。立体声的 `--mapping 2,1` 表示输出第 1 声道取原第 2 路样本，输出第 2 声道取原第 1 路样本。

## cmap 格式

`cmap` 是 little-endian chunk：

- 4 字节：声道数量 `N`。
- `N * 16` 字节：每个声道一个最多 16 字节的零填充名称。
- 若 payload 为奇数长度，追加一个零填充字节以保持 WAV word 对齐。

工具会同步更新 RIFF 顶层大小和后续 chunk 偏移。

## 备份、回读和回滚

实际写入前会在同目录创建带 UTC 时间戳的 `.bak-...` 备份。写入采用临时文件和原子重命名；写入后重新打开目标文件计算 SHA-256，并与预期输出比较。

回滚可以显式传入备份路径；不传时自动选择字典序最新的 `.bak-*` 文件。回滚内容也会先通过 WAV 解析校验，再原子覆盖目标文件。

## 测试

```sh
go test ./...
go vet ./...
```
