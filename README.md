# mp4timeline

从 MP4 视频轨的 `stts`、`ctts`、`stss` 重建逐帧时间线，用于排查
“正常长帧被误判为冻结”一类问题。工具只读取容器盒和 sample table，
不读取、解码或修改 `mdat` 像素。

## 时间线规则

- `DTS[n] = sum(stts[0..n-1].sample_delta)`
- `PTS[n] = DTS[n] + ctts.composition_offset`
- 按 PTS、DTS、sample number 排序，得到展示顺序。
- 每帧展示区间为 `[PTS, next_PTS)`；最后一帧使用自己的 `stts` 时长。
- 多个 sample 有相同 PTS 时，解码顺序最后的一个占据该展示槽，其余为 0 时长。
- 名义帧长取 `stts` 正时长中出现次数最多的值，可用 `-nominal` 覆盖。

## 使用

```bash
go run . video.mp4
go run . -format json video.mp4
go run . -track 1 -long-factor 1.5 video.mp4
```

TSV 默认列：

```text
track_id sample pts dts pts_seconds dts_seconds display_start display_end
display_seconds display_order sync reason detail
```

`pts`、`dts`、`display_start`、`display_end` 是媒体 timescale 下的原始
ticks，秒数仅用于阅读。JSON 输出包含同样的字段及表结构问题。

## 判定原因

| reason | 含义 |
| --- | --- |
| `normal` | 展示区间与 DTS 步长均未超过阈值 |
| `repeat_pts` | 多个 sample 使用相同 PTS；这是 sample table 层面的重复展示冲突 |
| `long_display` | 展示区间超过 `long-factor * nominal`，但可能只是合法的保持帧 |
| `decode_gap` | 单个 `stts` delta 超过阈值，解码时间轴存在大跨步 |
| `zero_decode_delta` | `stts` delta 为 0，DTS 没有推进 |
| `zero_display_duration` | 独立的 PTS 没有得到非零展示区间 |
| `ctts_missing` | `ctts` 覆盖的 sample 数不足，该帧 PTS 回退到 DTS |

原因可以组合，例如 `long_display|decode_gap` 表示展示区间和 `stts`
步长同时偏大。`detail` 会给出实际 ticks、名义帧长和冲突 sample。

`stss` 用于标记 `sync`，并检查样本号是否越界、是否包含重复或倒序项。
`stsz`/`stz2` 与 `stts` 的 sample 数不一致、`ctts` 覆盖不足等会输出到
stderr；`-strict` 会在存在这种表结构问题时返回退出码 2。

## 边界

- “重复”不能在不解码像素的前提下证明两幅画面相同；`repeat_pts` 只表示
  两个 sample 被映射到同一展示时间。工具不会据此修改任何 PTS/DTS。
- 当前处理普通 MP4 的 `moov/trak/mdia/minf/stbl`。检测到
  `moof`/`mvex` 的 fragmented MP4 时会报告 `fragmented_mp4`，但不解析
  `trun`。
- 输出是媒体时间线，不应用 `edts/elst` 编辑列表位移。
- 单个视频轨最多展开 10,000,000 个 sample，表盒载荷上限 64 MiB。

## 验证

```bash
go test ./...
go vet ./...
```
