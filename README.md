# mediahash

胶片归档副本的多层哈希与差异定位工具。只使用 Go 标准库，不依赖 ffmpeg 或第三方包。

它按以下层次建立 SHA-256：

1. **容器块**：整个文件的固定大小块；MP4 另外按 `stsc/stco/co64` 生成媒体 chunk 哈希。
2. **媒体 sample**：解析 MP4/MOV 的 `stsz/stz2`、`stsc`、`stco/co64`、`stts`、`ctts`、`stss`，定位每个音视频 sample 的物理区间并哈希。
3. **音频采样区间**：WAV/PCM 按 PCM 帧区间哈希；MP4 音轨把编码音频 sample 按窗口组成区间，并在 `stts`/timescale 上换算时间。

比对时同时给出：

- `earliest_byte`：由块哈希引导找到的最早原始字节差异。
- `earliest_media_change`：最早的媒体内容差异，包含原文件和副本各自的绝对偏移、sample/区间内偏移。
- 受影响画面：track、sample number、显示顺序、PTS/DTS、时间点、是否 sync sample。
- 受影响声音：音频区间、采样数范围、PCM 最早帧或 MP4 音频 sample、时间范围。

## 正常重排

文件名和路径不参与内容哈希；格式按文件内容探测。下列变化不会造成媒体误报：

- 改名、移动目录。
- `moov`/`mdat` 等顶层盒位置变化、faststart。
- `stco/co64` 偏移变化、chunk 交错或重新分块。
- track ID 或同类型 track 顺序变化时，按 sample/音频区间哈希配对。
- `free`、`skip`、`uuid`、`udta/meta`、RIFF `JUNK/LIST` 等附属或无关盒块变化。
- sample 字节不变时的位置置换；这会记录为 `rearrangements`，而不是媒体损坏。

默认情况下，只要 sample 和音频区间内容一致，`diff` 退出码为 `0`；容器布局差异只作取证记录。加 `-strict-container` 可让容器差异也返回 `1`。

## 使用

```bash
go test ./...

# 生成清单
go run . manifest film.mp4
go run . manifest -json -out film.mediahash.json film.mp4
go run . hash -json film.wav

# 比较原始归档和多次复制后的副本
go run . diff original.mp4 copy.mp4
go run . diff -json --audio-window 1024 original.mp4 copy.mp4

# 校验清单中的整文件 SHA-256
go run . verify film.mediahash.json film.mp4
```

也可以直接传一个或两个文件：

```bash
go run . film.mp4
go run . original.mp4 copy.mp4
```

`diff` 退出码：

- `0`：整文件一致，或媒体 sample/音频区间一致（可能存在正常容器重排）。
- `1`：检测到画面/声音内容差异；或 `-strict-container` 下容器层不一致。
- `2`：参数、IO 或文件解析错误。

## JSON 结构

`Manifest` 的主要数组：

- `blocks[]`：固定大小原始容器块。
- `boxes[]`：MP4 box 或 RIFF chunk 的区间和哈希。
- `chunks[]`：MP4 sample table 映射出的容器 chunk。
- `tracks[]`：track ID、handler、类型、timescale、sample/chunk 数量。
- `samples[]`：每个媒体 sample 的偏移、大小、哈希、PTS/DTS、显示顺序和 sync 标记。
- `audio_intervals[]`：音频采样区间哈希。采样区间使用从 `0` 开始的半开区间 `[start_sample, end_sample)`；`samples[].index` 使用 MP4 常见的从 `1` 开始的 sample number。

`Report` 中 `changes[]` 是媒体层变化，`container_changes[]` 是容器/块/chunk 变化，`ignored_changes[]` 是已知无关盒块，`rearrangements[]` 是内容哈希未变的位置重排。

## 边界

- 支持常见非分片 MP4/MOV 和 RIFF WAV；fragmented MP4（`moof/trun`）暂不展开 sample。
- 不做视频像素或 AAC/PCM 解码；哈希证明归档字节/sample 未变。转码、响度处理或重新编码通常会改变哈希。
- WAV 支持 PCM 和 WAVE_FORMAT_EXTENSIBLE 中的 PCM；压缩 WAV 只能按数据区间取证。
- 表盒载荷上限为 64 MiB，单 track 最多展开 10,000,000 个 sample。
