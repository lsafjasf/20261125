# MP4 四层故障定位器（Go 标准库）

工作区没有提供原始故障视频，因此这里交付一套可复现的最小 MP4/H.264 复核集：
`testdata/clean.mp4` 是 8 帧、GOP=`1,5`、无 B 帧的已知良好基线；`injections/`
中有且仅有三个 `.mp4` 注入文件（另有 `manifest.json` 预期清单）。诊断器只使用 Go 标准库解析 ISO-BMFF/MP4 box、
样本表和 AVC 长度前缀样本，不依赖第三方 Go 包。

## 快速复核

```bash
go test ./...
go run ./cmd/mkinject
go run ./cmd/mp4diag verify-fixtures
go run ./cmd/mp4diag --reference testdata/clean.mp4 \
  injections/03-compressed-sample2-display-2-4.mp4
./scripts/ffmpeg-crosscheck.sh
```

`go test` 会运行清单断言；如果本机有 `ffmpeg`，还会自动用 `framemd5`
验证真实解码帧。没有 `ffmpeg` 时该测试自动跳过。

## 三个注入文件

| 文件 | 层 | 首个坏 box / 位置 | 受影响 sample | 仍可解码范围 | 像素结论 |
| --- | --- | --- | --- | --- | --- |
| `injections/01-container-first-bad-box.mp4` | 容器 | `/mdat` @ 878，声明结束 4351，实际文件结束 3975 | 6–8 缺失 | sample/显示帧 1–5 | `not_assessed`：是截断，不是像素翻转 |
| `injections/02-sample-table-metadata-only.mp4` | 样本表 | `/moov/trak/mdia/minf/stbl/stsz` @ 701，第 2 项 619→620 | 元数据入口 sample 2；错误映射会漂移到 8 | 参考负载证明 1–8 字节均未变；修表后可解码 | `ruled_out`：不得报像素损坏 |
| `injections/03-compressed-sample2-display-2-4.mp4` | 压缩数据 → 显示 | 无结构性坏 box；sample 2 的绝对偏移 1989 单字节 XOR `0x01` | 压缩故障 sample 2；参考传播 sample 2–4 | 干净区间 1、5–8；sample 5 是 IDR 恢复点 | 哈希 `suspected`；ffmpeg 实测显示帧 2–4 不同 |

机器可读预期在 `injections/manifest.json`，可用
`mp4diag verify-fixtures` 一键校验。

## 四层模型

- **容器层 `container`**：扫描 `ftyp/moov/mdat/free` 等 box 的 32/64 位大小、
  header、父子边界和截断。目录 box `moov` 完整时，即使 `mdat` 越界也能先读出目录。
- **样本表层 `sample_table`**：解析 `stsd/avcC`、`stts`、`ctts`、`stss`、
  `stsc`、`stsz`、`stco/co64`，按 chunk 和 sample size 重建每个 sample 的
  绝对偏移、长度、同步帧、DTS/PTS 和显示顺序。
- **压缩数据层 `compressed_data`**：遍历 AVC length-prefixed NAL，并用
  `--reference clean.mp4` 对已知良好 sample 区间做 SHA-256。表坏时不采信坏映射，
  而是按参考绝对区间复核原始负载。
- **显示层 `display`**：用 `stss` 同步样本和 GOP 前向参考关系计算花屏传播。
  当前最小样本无 B 帧，sample 顺序等于显示顺序；有 `ctts` 时会单独计算
  `display_order` 和显示帧区间。

## 证据边界

Go 标准库不提供 H.264 解码器，因此静态分析不会把“字节不同”直接夸大成
“像素已损坏”：

- 无参考、无解码器时，压缩层返回 `not_assessed`。
- 与参考 sample 哈希不同，返回 `suspected`。
- 参考 sample 区间全部一致而样本表不同，返回 `ruled_out`，并把故障保留在元数据层。
- 若环境有 ffmpeg，`TestFFmpegFrameHashOracle` 用 `framemd5` 把显示层结论升级为
  外部解码器实测：压缩注入确实只有显示帧 2、3、4 的哈希变化。

## 常用命令

```bash
# 中文文本报告
go run ./cmd/mp4diag --reference testdata/clean.mp4 injections/02-sample-table-metadata-only.mp4

# JSON 报告
go run ./cmd/mp4diag --json --reference testdata/clean.mp4 injections/03-compressed-sample2-display-2-4.mp4

# 从固定基线重生成三个注入文件和 manifest
go run ./cmd/mkinject -clean testdata/clean.mp4 -out injections
```

JSON 顶层包含 `first_bad_box`、`layers`、`findings`、`boxes`、`samples`、
`reference` 和 `display`。每条 finding 都带层、错误码、box 路径/偏移/表项、
sample 列表以及 `pixel_damage` 证据级别。

## 目录

- `box.go` / `bytesx.go`：box 扫描和大端读取。
- `tables.go` / `boxes_parse.go`：样本表和 sample 映射。
- `reference.go`：参考区间 SHA-256 与元数据差异定位。
- `display.go`：同步样本/GOP 传播和可解码区间。
- `manifest.go`：注入预期清单及校验器。
- `cmd/mp4diag`：CLI；`cmd/mkinject`：三注入生成器。
- `ffmpeg_oracle_test.go`：可选的真实解码器帧哈希测试。
