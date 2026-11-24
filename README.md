# FFV1 跨机器帧哈希复核器

纯 Go 标准库实现。逐帧核对 FFV1/Matroska 的 **slice 顺序、slice CRC、
像素格式、容器时间戳、码流哈希和解码哈希**，给出“编码差异 / 解码差异 /
容器差异”的结论，并输出逐帧对照。

运行时的分析和判断全部在 Go 内完成，不调用 `ffmpeg`、`mkvinfo` 或第三方
Go 包。`ffmpeg` 只在一个可选的交叉验证测试里作为外部 oracle 使用；没有
安装时该测试自动 skip。

## 快速开始

```bash
go test ./...

# 两台机器的 captures + 各自的 framemd5 清单
go run . -a machine-a.mkv -b machine-b.mkv \
  -hash-a machine-a.framemd5 -hash-b machine-b.framemd5

# JSON
go run . -json -a machine-a.mkv -b machine-b.mkv \
  -hash-a machine-a.framemd5 -hash-b machine-b.framemd5
```

也可以直接给两个位置参数：`go run . a.mkv b.mkv`。

## 判断规则

工具把每一帧拆成三层证据：

| 层 | 证据 | 用途 |
| --- | --- | --- |
| 码流 | 整个 FFV1 sample 的 SHA-256 | 判断输入码流是否逐字节相同 |
| 编码结构 | slice 数、slice payload 多重集/序列、CRC 开关与校验结果、FFV1 配置 | 判断 slice 重排、重切、CRC 设置、像素格式变化 |
| 解码输出 | `-hash-a/-hash-b` 提供的 framemd5/framecrc | 判断像素是否真的不同 |

据此得到：

- **解码差异**：码流 sample 逐字节相同，但两边的解码帧哈希不同。相同输入
  得到不同输出，差异只能来自解码端（解码器、输出像素格式、色彩转换等）。
- **编码差异**：码流不同，但解码帧哈希一致；或差异可归因于编码参数
  （slice 数、slice 顺序、CRC、像素格式）。重排 slice 后每帧字节不同，
  但像素不变，属于编码/封装差异。
- **容器时间戳差异**：码流和像素都相同，只有 PTS/时间基不同，不属于编码
  或解码差异。
- **容器配置差异**：sample 字节相同但 FFV1 Configuration Record（像素格式等）
  不同。此时同一码流会被按不同配置解释，应先核对 CodecPrivate。
- **证据不足**：码流不同且解码哈希也不同，同时没有足够证据把差异限定在
  编码端。报告仍会标为编码差异（因为码流本身已不同），但置信度为
  `medium`，并明确提示“不能据此单独证明解码器有缺陷”。

如果只有一边提供了解码哈希，工具不会猜测像素是否相等，只做码流/结构判断。

## 逐帧核对内容

- **slice 顺序**：从每个 FFV1 frame 末尾的 slice footer 链恢复 slice 边界。
  `slice_set_sha256` 对 slice payload 哈希排序后取哈希，因此对顺序不敏感；
  `slice_sequence_sha256` 按存储顺序取哈希。二者相同 → `same`；多重集相同
  但序列不同 → `permuted`；否则 → `different`。
- **CRC**：验证 FFV1 Configuration Record 的 32 位 CRC（多项式
  `0x04C11DB7`，初值 0，不反射）；验证每个 slice footer 的 size/error/CRC；
  验证 Matroska `CRC-32` 元素（标准 IEEE CRC-32，小端存储）。
- **像素格式**：解码 FFV1 Configuration Record 的 range-coded 头，读取
  `version/micro_version/coder_type/colorspace/bits_per_raw_sample/
  chroma_planes/log2_h_chroma_subsample/log2_v_chroma_subsample/
  extra_plane/num_h_slices/num_v_slices`，归一化为 `yuv420p`、`yuv444p`、
  `gray`、`gbrp` 等名称。
- **帧时间戳**：Matroska `TimestampScale × (Cluster Timecode + Block
  relative timecode)`，按纳秒逐帧对照。

## 仓库自带复核集

`testdata/machine-a.mkv` 是 64×48、10 帧、2×2 个带 CRC slice 的
FFV1/Matroska 基线。其余文件构造三类真实故障：

| A | B | 码流 | 解码哈希 | 预期结论 |
| --- | --- | --- | --- | --- |
| machine-a | machine-a-copy | 完全相同 | 相同 | `no_difference` |
| machine-a | machine-a-copy | 完全相同 | 不同（native vs rgb24） | `decoding_difference` |
| machine-a | machine-b-reorder | 不同（slice 1/2 互换） | 相同 | `encoding_difference`，逐帧 `encoding_slice_order` |
| machine-a | machine-b-slices | 不同（4 slice → 1 slice，CRC off） | 相同 | `encoding_difference`，逐帧 `encoding_slice_raster` |
| machine-a | machine-b-444 | 不同（yuv420p → yuv444p） | 不同 | `encoding_difference`，逐帧 `encoding_pixel_format` |
| machine-a | machine-b-shift | 相同 | 相同，PTS +0.5s | `container_timestamp_difference` |

`machine-b-reorder.mkv` 中每个 slice 自带自己的 raster 坐标，交换 slice
blob 后像素不变；`machine-b-slices.mkv` 和 `machine-b-444.mkv` 是从 A
无损重编码得到的检查入夹具。所有 `hashes-*.framemd5` 是解码侧数据，
`TestFFmpegOracle` 在本机有 ffmpeg 时会把它们与真实 framemd5 再核对一遍。
`ffv1-10bit.mkv` 另外覆盖 range coder 自定义状态转移表和 10-bit
`yuv420p10` 像素格式解析。

## 输出示例

```text
结论: 解码差异 (high)
  - 10 frame(s) have byte-identical encoded samples but different decoded hashes

#  A pts     B pts     Δns  A sl  B sl  slice rel  A crc  B crc  A sample      B sample      A decode      B decode      判定
0  0.000000  0.000000  0    4     4     same       ok     ok     7ba0610c4ab2  7ba0610c4ab2  3615caf157ce  84cf7e4fa6b4  解码差异
```

JSON 版本包含完整 SHA-256、slice 多重集/序列哈希、配置字段、容器 CRC
统计和每帧原因。

## 复现夹具

```bash
# 纯 Go：复制基线、交换 slice、平移 block timecode
go run ./cmd/mkfixtures -a testdata/machine-a.mkv -out testdata

# 可选：本机有 ffmpeg 时交叉验证 framemd5 清单
go test ./internal/compare -run TestFFmpegOracle -v
```

`machine-b-slices.mkv` 和 `machine-b-444.mkv` 是生成后长期保留的
“不同编码设置”夹具，不需要每次重建。它们由下面的命令从 A 无损重编码一次：

```bash
ffmpeg -i testdata/machine-a.mkv -c:v ffv1 -level 3 -coder 0 -context 0 \
  -slices 1 -slicecrc 0 -pix_fmt yuv420p -fps_mode passthrough \
  testdata/machine-b-slices.mkv
ffmpeg -i testdata/machine-a.mkv -c:v ffv1 -level 3 -coder 0 -context 0 \
  -slices 1 -slicecrc 0 -pix_fmt yuv444p -fps_mode passthrough \
  testdata/machine-b-444.mkv
```

## 目录

- `internal/ebml`：EBML VINT、元素头和边界读取。
- `internal/mkv`：Matroska Segment/Tracks/Cluster/Block 解析、时间戳、容器 CRC。
- `internal/ffv1`：FFV1 range decoder、Configuration Record、slice footer/CRC。
- `internal/framemd5`：framemd5/framecrc 文本清单解析。
- `internal/compare`：逐帧对齐和编码/解码/容器归因。
- `internal/fixture`：纯 Go 的 Matroska 变更与 CRC 重算。
- `cmd/ffv1compare`、`cmd/mkfixtures`：CLI 和夹具生成器。

## 证据边界

当前解析面向 Matroska 中常见的 FFV1 v3/v4（Configuration Record +
自描述 slice footer）。FFV1 v0/v1 把参数放在帧头、且没有 v3 的 slice
footer 布局，不在本工具的覆盖范围内。

Go 标准库不包含 FFV1 像素解码器。因此本工具**不**声称能从码流直接重建
像素；解码哈希必须由使用方以 framemd5/framecrc 形式提供。这个边界是刻意
的：帧哈希不一致时，最关键的证据正是“同一码流在不同解码端的输出”，而
不是工具自己再实现一个可能同样有差异的解码器。

对于“码流不同且解码哈希也不同”的情况，工具会同时报告码流、slice、
pixel format 和 PTS 证据，并明确说明：码流侧差异已足以解释哈希差异，
但若要证明某个解码器有错，需要把同一份码流分别交给两台机器解码后再比较
（即上表中的 decoding 场景），不能仅凭两份不同的文件下结论。
