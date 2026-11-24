# cutmux — 双容器剪辑点/漏帧审计（Go 标准库）

同一个剪辑点导出成 MP4/MOV 与 Matroska 后，常见两种表象：一版多两帧，
另一版音频长 50ms。两者不一定是“漏帧”和“多音频”，也可能只是编辑列表、
编码 delay、片尾 padding 或黑场 handle 的不同表达。

`cutmux` 不做转码、不修改输入，只读解析两个容器，把每条轨换算到
**presented 时间线**，再用编码包哈希序列判断内容是否真的少了一帧/一包。

## 运行

```bash
go test ./...
go run ./cmd/cutmux \
  -a deliver.mp4 \
  -b deliver.mkv \
  -label-a mp4 -label-b mkv \
  -out audit-out \
  -now 2026-11-25T00:00:00Z
```

也接受两个位置参数：

```bash
go run ./cmd/cutmux deliver.mp4 deliver.mkv
```

- `-out`：写 `report.json` 与 `report.md`，默认 `audit-out`。
- `-tolerance-ms`：时间线容差，默认 5ms。
- `-frames-a` / `-frames-b`：可选的逐帧黑场证据 CSV，见下文。
- `-now`：固定报告时间，保证复测字节可复现。
- 退出码：`0` = 只发现合法封装差异；`1` = 有真实差异或需人工确认；
  `2` = 参数/解析/IO 错误。

## 比较链路

### 1. 容器与时间尺度

- MP4/MOV：读 `mvhd/trak/tkhd/edts/elst/mdia/mdhd/hdlr/minf/stbl`，
  以及 `stts/ctts/stsc/stsz/stz2/stco/co64/stss`。
- Matroska：读 EBML 的 `Info/Tracks/Cluster`，支持 `SimpleBlock`、
  `BlockGroup`、Xiph/fixed/EBML lacing、`TimestampScale`、`TrackOffset`。
- 所有输出使用 ns 或 ms；内部保留原始 timescale，跨尺度换算集中在
  少量 scale 函数中，不用浮点累计时间线。

### 2. presented 时间线

- MP4：由 `stts + ctts` 得到 PTS，再逐条应用 `elst`。leading empty edit
  会形成片头空段；非零 `media_time` 是通常的 AAC priming 裁剪。
- Matroska：block 时间先乘 `TimestampScale` 并加 `TrackOffset`，再减去
  `CodecDelay`；最后一块的正 `DiscardPadding` 从尾部裁掉。
- `raw` 区间是容器里编码包的时间；`presented` 区间是播放器实际呈现时间。
- 音频同时报告编码包数、折算采样帧数与 presented 时长；48kHz 下
  50ms 应正好是 2400 个采样帧。

### 3. 帧率

MP4 用 `mdhd.timescale / stts` 的主 duration；MKV 用
`DefaultDuration`/主 block 间隔。结果优先吸附到
`24000/1001、24、25、30000/1001、30、50、60000/1001、60` 等常见帧率。
帧率不一致时会先报 `FRAME_RATE_MISMATCH`，避免把“时间累积漂移”误判成漏帧。

### 4. 编码包哈希与真实漏帧

对每个编码样本/block 的 payload 计算截断 SHA-256，在 presented 顺序上做
对齐（相同前后缀 + 小差异 LCS）：

- 只在序列内部出现 `onlyA`/`onlyB`：编码内容真的缺/多，判为
  `real-frame-loss-or-insert` / `real-audio-loss-or-insert`。
- 只差片头/片尾若干包：可能只是 edit list、CodecDelay、DiscardPadding、
  priming 或尾黑场，继续用下面证据分类。
- 两侧都重新编码、payload 完全不同时，只做时间线比较并标
  `timing-only-unverified`，不会声称“漏帧”。

## 黑帧证据

压缩视频（H.264/HEVC/AV1 等）没有解码器时，标准库无法可靠地看像素。
因此逐帧黑场使用外部解码结果作为 sidecar，而不是用包大小冒充像素结论。

逐帧格式（推荐带表头）：

```csv
pts_seconds,black,luma
0.000,1,16
0.040,1,16
0.080,0,93
```

也支持黑场区间：

```csv
start_seconds,end_seconds
0.000,0.080
```

sidecar 的 PTS 按 presented 时间匹配，允许半帧误差。没有 sidecar 时，
工具只给 `sample-size-candidate`：体积显著小于中位数的帧是“黑帧候选”，
报告明确标记为非像素证据。

## 怎么读“合法封装差异”与“真实漏帧”

| 结论 | 含义 | 是否真实丢内容 |
|---|---|---|
| `identical-payload-and-timeline` | 包序列、presented 起点都一致 | 否 |
| `timeline-shift-only` | 包完全相同，只是容器起点差 | 否；但可能影响同步 |
| `black-handle-only` | 多出的边缘帧由逐帧 sidecar 证明为黑场 | 否 |
| `priming-boundary` | 片头多出的音频包符合 encoder priming/CodecDelay | 否 |
| `padding-boundary` | 片尾多出的音频包符合 encoder padding/DiscardPadding | 否 |
| `edit-or-discard-padding-only` | 音频包相同，仅 edit/discard 后时长不同 | 否 |
| `real-frame-loss-or-insert` | 视频缺失/插入在序列内部 | 是 |
| `real-audio-loss-or-insert` | 音频缺失/插入在序列内部 | 是 |
| `tail/head-frame-difference` | 边缘帧不同，但不是黑场 | 待确认 |

一个 2 帧差在 25fps 是 80ms，在 40fps 才是 50ms；不要直接把“2 帧”和
“50ms”当成同一个量。48kHz 下 50ms 是 2400 个采样，也不是一个整数 AAC
packet（1024 samples），所以必须同时看 `elst`、`CodecDelay`、
`DiscardPadding` 和 packet 哈希，而不能只看轨道时长。

## 范围与限制

- progressive MP4/MOV；不展开 fragmented MP4 的 `moof/traf/trun/tfdt`。
- Matroska 的未知长度 Cluster 不展开；请用已知 Cluster size 重封后审计。
- 输入只读；报告中的 SHA-256 用于确认复测的是同一批文件。
- 不调用 ffmpeg/ffprobe，运行期只使用 Go 标准库。

## 测试

```bash
go test ./...
```

合成夹具覆盖：

- MP4 `elst` leading empty edit、`30000/1001` 精确帧率；
- MKV `CodecDelay` 和 `DiscardPadding` 的 presented 换算；
- 双容器相同 payload 的合法差异；
- 视频序列内部缺一帧的真实漏帧；
- 尾部两个黑帧 handle；
- 尾部两包、共 50ms 的音频 padding；
- 字幕首个非空样本入点偏移。
