# FFV1/Matroska 逐帧审计工具

纯 Go 标准库实现，不依赖 `ffmpeg`、`mkvinfo` 或第三方 Go 包。工具逐 block/frame 输出：

- FFV1 slice 数量及全片一致性；
- 每个 slice 的 32 位 CRC（FFV1 使用多项式 `0x04C11DB7`、初值 0、不做输入/输出反射）；
- Matroska `CRC-32` 容器校验（标准 IEEE CRC32、小端存储）；
- block 元素偏移、block 头位置、与 `ffprobe -show_packets` 对齐的 `pos`、codec sample 偏移和大小；
- SimpleBlock 关键帧位，以及 BlockGroup 无 `ReferenceBlock` 的关键帧推断；
- 每帧 SHA-256 和汇总 `frame_hashes`；
- 未知 EBML/Matroska 元素的 ID、父元素、偏移和长度；未知元素只记录并跳过，不会导致整段失败；
- 损坏的 Matroska block/FFV1 slice、同 block 内仍通过 CRC 的 slice，以及连续可恢复帧区间。

## 运行

```bash
go test ./...
go run . -slices 4 /path/to/package.mkv
go run . -json -o report.json /path/to/package.mkv
```

常用参数：

- `-json`：向标准输出输出完整 JSON。
- `-o report.json`：同时写入 JSON 报告。
- `-slices N`：指定期望 slice 数；不传时用全片众数作为一致性基线。
- `-expect expected.json`：按清单核对哈希、大小、位置、关键帧和 slice 数。
- `-ffv1-crc auto|on|off`：强制 FFV1 slice 尾部是否含 CRC；默认自动并用健康帧校正全轨模式。
- `-hash-file`：额外计算整个输入文件的 SHA-256。
- `-fail-on error|warning|none`：CI 退出策略；默认发现错误时退出 `1`，用法/IO 错误退出 `2`。

仓库自带一个由 FFmpeg 生成的最小真实样本 `testdata/ffv1-4slices.mkv`：10 帧、2 个关键帧、每帧 4 个带 CRC 的 slice，并含 Cues。`go test ./...` 会验证干净样本、单字节 CRC 注入、未知元素和合成 BlockGroup。

工作区初始化时没有实际来件，因此另提供一组可复核注入样本：

```bash
go run ./cmd/mkinject
go run . injections/02-ffv1-slice-crc-block.mkv
```

- `injections/01-unknown-matroska-element.mkv`：Segment 内加入未知 ID `0x1FC0FFEE`，仍应得到 10 帧且无错误。
- `injections/02-ffv1-slice-crc-block.mkv`：第 4 帧（1 基；JSON `index=3`）slice 1 单比特翻转；应定位 block 偏移 `3906`、slice 范围 `4332-4763`，好 slice 为 `0,2,3`。终端显示恢复帧 `1-3` 和 `5-10`，JSON 0 基区间为 `0-2` 和 `4-9`。
- `injections/03-truncated-block.mkv`：第 5 个 block 截断；终端可恢复帧为 `1-4`，JSON 0 基区间为 `0-3`。
- `injections/manifest.json`：干净样本的逐帧预期哈希、位置、关键帧和注入结论。
- `injections/expectations.json`：可直接传给 `-expect` 的顶层期望值清单。

## 输出要点

```json
{
  "frames": [
    {
      "index": 3,
      "position": 3909,
      "frame_offset": 3913,
      "size": 1035,
      "keyframe": false,
      "sha256": "...",
      "slice_count": 4,
      "crc_ok": false,
      "status": "corrupt",
      "slices": [
        {"index": 1, "offset": 4332, "end_offset": 4763, "crc_ok": false}
      ]
    }
  ],
  "corrupt_blocks": [
    {
      "frame_index": 3,
      "element_id": "0xA3",
      "element_offset": 3906,
      "kind": "ffv1_slice_crc_mismatch",
      "corrupt_offset": 4332,
      "corrupt_end_offset": 4763,
      "recoverable_slices": [0, 2, 3]
    }
  ],
  "recoverable_ranges": [
    {"start_frame_index": 0, "end_frame_index": 2, "frame_count": 3},
    {"start_frame_index": 4, "end_frame_index": 9, "frame_count": 6}
  ]
}
```

`position` 是 Matroska block 结构起点（track VINT），与 `ffprobe` 的 packet `pos` 一致；`frame_offset` 是跳过 track/timecode/flags 后的 FFV1 sample 起点。两者都保留，避免“样本位置”口径混淆。

## 恢复语义

FFV1 是帧内编码，slice 之间也可独立校验。因此：

- 一个坏 slice 通常只使该 slice 的空间区域不可信，报告列出同 block 中 CRC 正常的 slice；
- 一个坏帧不会自动污染后续帧，连续 CRC/结构正常的帧组成 `recoverable_ranges`；
- 如果 block 被截断，报告已读到的字节并给出部分 SHA-256，同时标记 `truncated`；
- 未知元素不会改变帧状态；只有无法读取 EBML 根或 Segment 时才返回致命错误。

## 期望值清单

```json
{
  "expected_slices": 4,
  "frames": [
    {
      "index": 0,
      "sha256": "7ba0610c4ab26e86...",
      "size": 1174,
      "position": 627,
      "keyframe": true,
      "slices": 4
    }
  ]
}
```

未提供 `index` 的帧条目按数组顺序匹配。位置字段使用 block 结构起点 `position`。

## 代码结构

- `ebml/`：EBML 头/VINT 与官方 Matroska ID 表（生成文件）。
- `ffv1/`：FFV1 Configuration Record/slice CRC 与反向尾链解析。
- `audit/`：Matroska tracks、clusters、blocks、Cues、未知元素、报告和恢复区间。
- `main.go`、`render.go`：CLI 与中文终端摘要。
