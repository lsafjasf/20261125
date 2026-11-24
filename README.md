# MOV Edit List 音频起点审计器

使用 Go 标准库解析 QuickTime/MOV/ISO BMFF 的 `mvhd`、`trak/tkhd`、`trak/edts/elst` 与 `trak/mdia/mdhd`，逐层换算每个 edit 的合成时间、媒体区间和音频实际起点。

审计命令只用 `os.Open`/`O_RDONLY` 打开输入，不会写回、修补或移动输入文件；Markdown/JSON 只写入显式指定的报告路径。

## 运行

```bash
go test ./...
go run ./cmd/mkfixtures            # 重新生成 testdata/cases 下的独立样例
go run ./cmd/editlist-audit \
  --markdown reports/audit-report.md \
  --json reports/audit-report.json \
  testdata/cases
```

也可以审计单个 MOV/MP4 或整个目录：

```bash
go run ./cmd/editlist-audit /path/to/movie.mov
go run ./cmd/editlist-audit --json - /path/to/dir
```

不带输入参数时默认审计 `testdata/cases`；不带报告参数时 Markdown 输出到 stdout。

## 时间换算

设：

- `M = mvhd.timescale`：电影/合成时间尺度；
- `D = mdhd.timescale`：媒体时间尺度；
- `S_i = elst.segment_duration`：edit 时长，单位为 `M`；
- `T_i = elst.media_time`：媒体起点，单位为 `D`；
- `r_i = R_i / 65536`：`media_rate` 的 16.16 定点速率。

则：

```text
C_i = sum(S_0 ... S_(i-1))
合成区间 = [C_i, C_i + S_i)                     # M units
L_i      = S_i * D/M * r_i                      # D units，精确有理数
媒体区间 = [T_i, T_i + L_i)                     # r_i >= 0
t_comp   = C_i + (t_media - T_i) * M/(D*r_i)
media 0  -> C_i - T_i*M/(D*r_i)                 # 可为负
```

报告同时保留精确分数、浮点秒以及 `floor/nearest/ceil` 整数换算，避免跨 timescale 时把 `9187.5` 一类值静默取整。

特殊值：

- `media_time = -1`：标准 empty edit。它只有合成区间，没有媒体区间；开头的 empty edit 会把后一个非空 edit 推迟 `S/M` 秒。
- `media_rate = 0`：ISO BMFF 的 dwell：整个 segment 呈现 `T_i` 这一个媒体时间点，媒体消耗长度为 0。Apple QTFF 规定 MOV rate 不能为 0 或负数，因此报告会给兼容性 warning。
- `media_time < -1`：不是标准 empty edit，FFmpeg 严格模式会拒绝；本工具保留有符号计算并给 warning。
- 对单位速率 edit，报告还输出 FFmpeg `time_offset` 口径的 `T - C*D/M`。它与“media 0 映射到合成时间”符号相反，可用于对照不同代码库中的“负起点”命名。

`media_rate` 不改变当前 edit 的 anchor `(T_i,C_i)`，但会改变媒体区间右端点、时间映射斜率和 media 0 的合成位置；零速率时媒体区间退化为点。

## 独立样例

| 文件 | 必测边界 | 关键结果 |
|---|---|---|
| `testdata/cases/01_empty_edit_delay.mov` | leading empty edit | empty 占合成 `[0, 0.25s)`；音频 anchor 在 `0.25s`，源起点 `0`；`T-C*D/M=-12000` |
| `testdata/cases/02_negative_origin_shift.mov` | 负合成原点 | `T=8820/44100=0.2s`，合成区间 `[0,0.8s)`；media 0 映射到 `-0.2s` |
| `testdata/cases/03_negative_media_time_field.mov` | 字面负 `media_time` | `T=-2`，媒体区间从 `-2/48000s` 开始；标记为非标准并告警 |
| `testdata/cases/04_zero_rate_dwell.mov` | 零速率/dwell | 合成 `[0,0.5s)` 对应媒体点 `12000`，媒体时长为 `0` |
| `testdata/cases/05_cross_timescale_600_to_22050.mov` | 跨 timescale | `M=600,D=22050,S=250` 得 `18375/2=9187.5`，nearest 为 `9188` |
| `testdata/cases/06_media_rate_2x.mov` | 非 1 media rate | `0.5s` 合成区间在 2 倍速下消耗 `48000` 个 `D` units，即 1 秒媒体 |

生成的 Markdown 审计报告：`reports/audit-report.md`；机器可读结果：`reports/audit-report.json`。

## 范围

当前聚焦 progressive MOV/MP4 的 `moov` 元数据与 edit list 换算，不解码 PCM/AAC 数据包，也不展开 fragmented MP4 的 `moof/traf/tfdt`。样例中的 `sowt` PCM 数据仅用于让盒结构可被 FFprobe/FFmpeg 识别。

## 参考口径

- Apple QuickTime File Format, Edit List Table：<https://developer.apple.com/documentation/quicktime-file-format/edit_list_atom/edit_list_table>
- FFmpeg 7.0 `libavformat/mov.c` 的 `mov_read_elst`、`get_edit_list_entry` 与 edit-list offset 处理：<https://ffmpeg.org/doxygen/7.0/mov_8c_source.html>
- ISO/IEC 14496-12 的 edit list 语义：`media_time=-1` 为空编辑，`media_rate=0` 为 dwell；本仓库以精确有理数实现其跨 timescale 换算。
