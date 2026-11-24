# MOV 编辑列表与音频起点审计报告

- 输入方式：仅以 `os.O_RDONLY` 打开；解析过程不写回、不移动、不修补输入文件。
- 换算原则：整数盒字段保持原值；跨 timescale 先用有理数保存精确结果，再给出浮点秒和 floor/nearest/ceil 取整。
- 适用盒：`moov/mvhd`、`moov/trak/tkhd`、`moov/trak/edts/elst`、`moov/trak/mdia/mdhd`。

## 换算公式

设 `M = mvhd.timescale`，`D = mdhd.timescale`，第 `i` 个 edit 的 `segment_duration=S_i`、`media_time=T_i`、速率定点值为 `r_i=R_i/65536`。

- 合成起点：`C_i = Σ(j<i) S_j`，单位为 `M`；合成区间为 `[C_i, C_i+S_i)`。
- 普通/变速 edit 的精确媒体时长：`L_i = S_i × D/M × r_i`，单位为 `D`；正向播放媒体区间为 `[T_i, T_i+L_i)`。
- 时间映射：`t_comp(M) = C_i + (t_media(D)-T_i) × M/(D×r_i)`；因此媒体零点映射到 `C_i - T_i×M/(D×r_i)`，该值可为负。
- `T_i=-1`：标准 empty edit，只占用合成区间，不存在媒体区间；开头的 empty edit 会把后一个非空 edit 推迟其 `S/M` 秒。
- `r_i=0`：ISO BMFF 定义为 dwell，整个合成区间呈现 `T_i` 这一个媒体时间点，媒体消耗长度为 0；Apple QTFF 则规定 MOV 的 media rate 不得为 0 或负数，报告同时给出兼容性告警。
- `T_i<-1`：不是标准 empty edit；FFmpeg 严格模式会拒绝。本审计保留有符号计算并单独告警，避免把异常负起点吞掉。
- 起点语义：empty edit 改变下一个非空 edit 的合成 anchor `C_i`；`media_rate` 不改变当前 anchor `(T_i,C_i)`，但改变媒体区间右端点、映射斜率以及 media 0 对应的合成位置。零速率时媒体区间退化为单个时间点。

## 汇总

| 文件 | trak | edit | empty | zero-rate dwell | 负起点轨 | 跨 timescale 轨 | warning | error |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 6 | 6 | 8 | 1 | 1 | 2 | 6 | 2 | 0 |

## 独立样例覆盖矩阵

| 输入 | empty edit | 负合成原点/负字段 | zero-rate dwell | 跨 timescale | 非 1 速率 |
|---|---|---|---|---|---|
| `testdata/cases/01_empty_edit_delay.mov` | 是 | — | — | 是 | — |
| `testdata/cases/02_negative_origin_shift.mov` | — | 是 | — | 是 | — |
| `testdata/cases/03_negative_media_time_field.mov` | — | 是 | — | 是 | — |
| `testdata/cases/04_zero_rate_dwell.mov` | — | — | 是 | 是 | — |
| `testdata/cases/05_cross_timescale_600_to_22050.mov` | — | — | — | 是 | — |
| `testdata/cases/06_media_rate_2x.mov` | — | — | — | 是 | 是 |

## 1. `testdata/cases/01_empty_edit_delay.mov`

- 文件大小：96589 bytes
- `mvhd`：timescale=`1000`，duration=`1250`（5/4 s）

### trak #1 / track_id=1 / handler=`soun` / 音频轨

| 层 | 原始字段 | 值 | 时间换算 |
|---|---|---:|---|
| `mvhd` | timescale / duration | `1000` / `1250` | duration `5/4 s` |
| `trak/tkhd` | version / track_id / duration | `0` / `1` / `1250` | tkhd duration 与 elst segment 同属 mvhd timescale：`5/4 s` |
| `trak/edts/elst` | offset / version / entry_count | `244` / `0` / `2` | segment_duration 使用 mvhd timescale |
| `mdia/mdhd` | version / timescale / duration | `0` / `48000` / `48000` | duration `1 s`；media_time 使用此 timescale |

**音频实际起点判定**

- 首个有声/驻留 edit：`#1`（`media_playback`）
- 源媒体起点 `media_time`：`0` mdia units = `0 s`
- 合成起点：`250` mvhd units = `1/4 s`
- 开头 empty edit 延迟：`1/4 s`；这是实际可闻起点相对时间线 0 的后移量。
- 媒体零点映射到合成时间：`250` mvhd units = `1/4 s`（float `0.250000`）
- 单位速率时间戳偏移 `T-C×D/M`：`-12000` mdia units = `-1/4 s`；负值对应 empty duration 换算后大于 `media_time` 的 FFmpeg `time_offset` 口径。

**每个 edit 的媒体区间与合成时间**

| edit | 合成区间（mvhd timescale） | 媒体区间（mdia timescale） | media rate 与取整 | 判定/告警 |
|---:|---|---|---|---|
| #0 `empty_edit` | units `[0, 250)`<br>duration `250`<br>seconds `[0, 1/4)`<br>float `[0.000000, 0.250000)` | 无媒体区间（empty edit） | raw `0x00010000`<br>r=`1`（1） | media_time=-1: standard empty edit; composition interval is reserved and has no source media interval |
| #1 `media_playback` | units `[250, 1250)`<br>duration `1000`<br>seconds `[1/4, 5/4)`<br>float `[0.250000, 1.250000)` | units `[0, 48000)`<br>duration `48000`<br>seconds `[0, 1)`<br>float `[0.000000, 1.000000)` | raw `0x00010000`<br>r=`1`（1）<br>duration floor/near/ceil: `48000/48000/48000`<br>media0→comp `250` M units / `1/4 s` |  |

**结构与兼容性检查**

| 级别 | code | 位置 | 信息 |
|---|---|---|---|
| `info` | `cross_timescale` | `/moov/trak/edts/elst` | cross-timescale conversion: mvhd=1000, mdhd=48000 |

## 2. `testdata/cases/02_negative_origin_shift.mov`

- 文件大小：88777 bytes
- `mvhd`：timescale=`1000`，duration=`800`（4/5 s）

### trak #1 / track_id=1 / handler=`soun` / 音频轨

| 层 | 原始字段 | 值 | 时间换算 |
|---|---|---:|---|
| `mvhd` | timescale / duration | `1000` / `800` | duration `4/5 s` |
| `trak/tkhd` | version / track_id / duration | `0` / `1` / `800` | tkhd duration 与 elst segment 同属 mvhd timescale：`4/5 s` |
| `trak/edts/elst` | offset / version / entry_count | `244` / `0` / `1` | segment_duration 使用 mvhd timescale |
| `mdia/mdhd` | version / timescale / duration | `0` / `44100` / `44100` | duration `1 s`；media_time 使用此 timescale |

**音频实际起点判定**

- 首个有声/驻留 edit：`#0`（`media_playback`）
- 源媒体起点 `media_time`：`8820` mdia units = `1/5 s`
- 合成起点：`0` mvhd units = `0 s`
- 媒体零点映射到合成时间：`-200` mvhd units = `-1/5 s`（float `-0.200000`）
- **负起点**：媒体零点落在合成时间 0 之前，样本时间戳需要负偏移；edit anchor 本身仍位于上述合成起点。
- 单位速率时间戳偏移 `T-C×D/M`：`8820` mdia units = `1/5 s`；负值对应 empty duration 换算后大于 `media_time` 的 FFmpeg `time_offset` 口径。

**每个 edit 的媒体区间与合成时间**

| edit | 合成区间（mvhd timescale） | 媒体区间（mdia timescale） | media rate 与取整 | 判定/告警 |
|---:|---|---|---|---|
| #0 `media_playback` | units `[0, 800)`<br>duration `800`<br>seconds `[0, 4/5)`<br>float `[0.000000, 0.800000)` | units `[8820, 44100)`<br>duration `35280`<br>seconds `[1/5, 1)`<br>float `[0.200000, 1.000000)` | raw `0x00010000`<br>r=`1`（1）<br>duration floor/near/ceil: `35280/35280/35280`<br>media0→comp `-200` M units / `-1/5 s` | media time zero maps to a negative composition time; sample timestamps require a negative shift |

**结构与兼容性检查**

| 级别 | code | 位置 | 信息 |
|---|---|---|---|
| `info` | `cross_timescale` | `/moov/trak/edts/elst` | cross-timescale conversion: mvhd=1000, mdhd=44100 |

## 3. `testdata/cases/03_negative_media_time_field.mov`

- 文件大小：96577 bytes
- `mvhd`：timescale=`1000`，duration=`800`（4/5 s）

### trak #1 / track_id=1 / handler=`soun` / 音频轨

| 层 | 原始字段 | 值 | 时间换算 |
|---|---|---:|---|
| `mvhd` | timescale / duration | `1000` / `800` | duration `4/5 s` |
| `trak/tkhd` | version / track_id / duration | `0` / `1` / `800` | tkhd duration 与 elst segment 同属 mvhd timescale：`4/5 s` |
| `trak/edts/elst` | offset / version / entry_count | `244` / `0` / `1` | segment_duration 使用 mvhd timescale |
| `mdia/mdhd` | version / timescale / duration | `0` / `48000` / `48000` | duration `1 s`；media_time 使用此 timescale |

**音频实际起点判定**

- 首个有声/驻留 edit：`#0`（`media_playback`）
- 源媒体起点 `media_time`：`-2` mdia units = `-1/24000 s`
- 合成起点：`0` mvhd units = `0 s`
- 媒体零点映射到合成时间：`1/24` mvhd units = `1/24000 s`（float `0.000042`）
- 单位速率时间戳偏移 `T-C×D/M`：`-2` mdia units = `-1/24000 s`；负值对应 empty duration 换算后大于 `media_time` 的 FFmpeg `time_offset` 口径。

**每个 edit 的媒体区间与合成时间**

| edit | 合成区间（mvhd timescale） | 媒体区间（mdia timescale） | media rate 与取整 | 判定/告警 |
|---:|---|---|---|---|
| #0 `media_playback` | units `[0, 800)`<br>duration `800`<br>seconds `[0, 4/5)`<br>float `[0.000000, 0.800000)` | units `[-2, 38398)`<br>duration `38400`<br>seconds `[-1/24000, 19199/24000)`<br>float `[-0.000042, 0.799958)` | raw `0x00010000`<br>r=`1`（1）<br>duration floor/near/ceil: `38400/38400/38400`<br>media0→comp `1/24` M units / `1/24000 s` | nonstandard negative media_time (<-1); strict FFmpeg rejects it, but signed arithmetic is retained for audit |

**结构与兼容性检查**

| 级别 | code | 位置 | 信息 |
|---|---|---|---|
| `warning` | `negative_media_time_field` | `/moov/trak/edts/elst` | edit 0 has nonstandard media_time=-2 |
| `info` | `cross_timescale` | `/moov/trak/edts/elst` | cross-timescale conversion: mvhd=1000, mdhd=48000 |

## 4. `testdata/cases/04_zero_rate_dwell.mov`

- 文件大小：96589 bytes
- `mvhd`：timescale=`1000`，duration=`1000`（1 s）

### trak #1 / track_id=1 / handler=`soun` / 音频轨

| 层 | 原始字段 | 值 | 时间换算 |
|---|---|---:|---|
| `mvhd` | timescale / duration | `1000` / `1000` | duration `1 s` |
| `trak/tkhd` | version / track_id / duration | `0` / `1` / `1000` | tkhd duration 与 elst segment 同属 mvhd timescale：`1 s` |
| `trak/edts/elst` | offset / version / entry_count | `244` / `0` / `2` | segment_duration 使用 mvhd timescale |
| `mdia/mdhd` | version / timescale / duration | `0` / `48000` / `48000` | duration `1 s`；media_time 使用此 timescale |

**音频实际起点判定**

- 首个有声/驻留 edit：`#0`（`dwell_zero_rate`）
- 源媒体起点 `media_time`：`12000` mdia units = `1/4 s`
- 合成起点：`0` mvhd units = `0 s`

**每个 edit 的媒体区间与合成时间**

| edit | 合成区间（mvhd timescale） | 媒体区间（mdia timescale） | media rate 与取整 | 判定/告警 |
|---:|---|---|---|---|
| #0 `dwell_zero_rate` | units `[0, 500)`<br>duration `500`<br>seconds `[0, 1/2)`<br>float `[0.000000, 0.500000)` | units `[12000, 12000)`<br>duration `0`<br>seconds `[1/4, 1/4)`<br>float `[0.250000, 0.250000)` | raw `0x00000000`<br>r=`0`（0）<br>duration floor/near/ceil: `0/0/0` | media_rate=0: ISO BMFF dwell; present the single media-time point for the whole segment and consume zero media duration |
| #1 `media_playback` | units `[500, 1000)`<br>duration `500`<br>seconds `[1/2, 1)`<br>float `[0.500000, 1.000000)` | units `[24000, 48000)`<br>duration `24000`<br>seconds `[1/2, 1)`<br>float `[0.500000, 1.000000)` | raw `0x00010000`<br>r=`1`（1）<br>duration floor/near/ceil: `24000/24000/24000`<br>media0→comp `0` M units / `0 s` |  |

**结构与兼容性检查**

| 级别 | code | 位置 | 信息 |
|---|---|---|---|
| `warning` | `zero_rate_dwell` | `/moov/trak/edts/elst` | edit 0 is a zero-rate dwell at media time 12000 |
| `info` | `cross_timescale` | `/moov/trak/edts/elst` | cross-timescale conversion: mvhd=1000, mdhd=48000 |

## 5. `testdata/cases/05_cross_timescale_600_to_22050.mov`

- 文件大小：44677 bytes
- `mvhd`：timescale=`600`，duration=`250`（5/12 s）

### trak #1 / track_id=1 / handler=`soun` / 音频轨

| 层 | 原始字段 | 值 | 时间换算 |
|---|---|---:|---|
| `mvhd` | timescale / duration | `600` / `250` | duration `5/12 s` |
| `trak/tkhd` | version / track_id / duration | `0` / `1` / `250` | tkhd duration 与 elst segment 同属 mvhd timescale：`5/12 s` |
| `trak/edts/elst` | offset / version / entry_count | `244` / `0` / `1` | segment_duration 使用 mvhd timescale |
| `mdia/mdhd` | version / timescale / duration | `0` / `22050` / `22050` | duration `1 s`；media_time 使用此 timescale |

**音频实际起点判定**

- 首个有声/驻留 edit：`#0`（`media_playback`）
- 源媒体起点 `media_time`：`0` mdia units = `0 s`
- 合成起点：`0` mvhd units = `0 s`
- 媒体零点映射到合成时间：`0` mvhd units = `0 s`（float `0.000000`）
- 单位速率时间戳偏移 `T-C×D/M`：`0` mdia units = `0 s`；负值对应 empty duration 换算后大于 `media_time` 的 FFmpeg `time_offset` 口径。

**每个 edit 的媒体区间与合成时间**

| edit | 合成区间（mvhd timescale） | 媒体区间（mdia timescale） | media rate 与取整 | 判定/告警 |
|---:|---|---|---|---|
| #0 `media_playback` | units `[0, 250)`<br>duration `250`<br>seconds `[0, 5/12)`<br>float `[0.000000, 0.416667)` | units `[0, 18375/2)`<br>duration `18375/2`<br>seconds `[0, 5/12)`<br>float `[0.000000, 0.416667)` | raw `0x00010000`<br>r=`1`（1）<br>duration floor/near/ceil: `9187/9188/9188`<br>media0→comp `0` M units / `0 s` |  |

**结构与兼容性检查**

| 级别 | code | 位置 | 信息 |
|---|---|---|---|
| `info` | `cross_timescale` | `/moov/trak/edts/elst` | cross-timescale conversion: mvhd=600, mdhd=22050 |

## 6. `testdata/cases/06_media_rate_2x.mov`

- 文件大小：96577 bytes
- `mvhd`：timescale=`1000`，duration=`500`（1/2 s）

### trak #1 / track_id=1 / handler=`soun` / 音频轨

| 层 | 原始字段 | 值 | 时间换算 |
|---|---|---:|---|
| `mvhd` | timescale / duration | `1000` / `500` | duration `1/2 s` |
| `trak/tkhd` | version / track_id / duration | `0` / `1` / `500` | tkhd duration 与 elst segment 同属 mvhd timescale：`1/2 s` |
| `trak/edts/elst` | offset / version / entry_count | `244` / `0` / `1` | segment_duration 使用 mvhd timescale |
| `mdia/mdhd` | version / timescale / duration | `0` / `48000` / `48000` | duration `1 s`；media_time 使用此 timescale |

**音频实际起点判定**

- 首个有声/驻留 edit：`#0`（`media_playback`）
- 源媒体起点 `media_time`：`0` mdia units = `0 s`
- 合成起点：`0` mvhd units = `0 s`
- 媒体零点映射到合成时间：`0` mvhd units = `0 s`（float `0.000000`）

**每个 edit 的媒体区间与合成时间**

| edit | 合成区间（mvhd timescale） | 媒体区间（mdia timescale） | media rate 与取整 | 判定/告警 |
|---:|---|---|---|---|
| #0 `media_playback` | units `[0, 500)`<br>duration `500`<br>seconds `[0, 1/2)`<br>float `[0.000000, 0.500000)` | units `[0, 48000)`<br>duration `48000`<br>seconds `[0, 1)`<br>float `[0.000000, 1.000000)` | raw `0x00020000`<br>r=`2`（2）<br>duration floor/near/ceil: `48000/48000/48000`<br>media0→comp `0` M units / `0 s` | non-unit media rate 2: media interval duration is rate-scaled |

**结构与兼容性检查**

| 级别 | code | 位置 | 信息 |
|---|---|---|---|
| `info` | `cross_timescale` | `/moov/trak/edts/elst` | cross-timescale conversion: mvhd=1000, mdhd=48000 |
| `info` | `non_unit_media_rate` | `/moov/trak/edts/elst` | non-unit positive media rate 2 |

