# mp4index — 只修索引、不碰媒体载荷的 MP4/MOV 样本表重建工具

用 Go 编写、零第三方依赖。扫描 `mdat` 中的完整 NAL/sample，重建最小样本表
（`stts/ctts/stsc/stsz/stco`）与关键帧索引（`stss`），输出可播放区间和未恢复
字节。所有修复结果都写入**另行指定的新文件**（同目录临时文件 + `fsync` +
原子 `rename`），源文件始终只读，任何失败都不会覆盖原文件。

## 它解决什么问题

录像异常掉电、拷贝截断或索引损坏后，常见三类故障样本：

1. **截断（truncated）**：文件尾部缺失，最后一个 sample 只写了一半；旧 `stsz`
   仍声称有 N 个样本。工具只承认扫描到的完整 sample，并报告截断尾巴与缺失槽位。
2. **重复 sample（duplicate）**：`stsz/stco` 中被插入或覆写了重复记录。以码流
   扫描出的真值重建表，重复索引记录自然消失；码流内连续重复帧可报告并按需去重。
3. **错误偏移（bad offsets）**：`stco/co64` 偏移整体错位。偏移完全由重新扫描
   得到，不再信任旧值，并统计失配条目。

## 工作流程

- `movie/`：解析 BMFF 盒树，提取 `mdat` 区间、`mdhd` 时标、`stsd/avcC`（原样
  复用，解码器配置一个字节不动）、旧 `stts/ctts/stsz/stsc/stco/stss` 用于诊断。
- `h264scan/`：在 `mdat` 上顺序走 AVCC 长度前缀 NAL（长度宽度取自 `avcC`，
  默认 4 字节），用 slice 头的 `first_mb_in_slice==0` 与 AUD 重组 access unit；
  IDR（AVC type 5 / HEVC 19、20）标记关键帧。遇到坏长度会尝试重同步，跳不过去
  的尾巴记为未恢复区间。
- `repair/`：对齐时长/合成偏移，计算“关键帧起始的连续可播放区间”和未恢复部分，
  构造全新 faststart `ftyp+moov+mdat`（也可 `--moov-last`），`mdat` 载荷按源
  字节逐段原样复制，偏移重映射到新位置。

输出文件默认每个 sample 一个 chunk（`stsc = 1,1,1` + N 条 `stco`），这是最简单
且无歧义的最小映射；偏移超过 32 位自动改用 `co64`。

## 快速开始

```bash
go test -count=1 ./...     # 全部单元/端到端测试（无 ffmpeg 也能跑合成样本组）
go run . -in damaged.mp4   # 检查并生成 damaged.repaired.mp4

# 真实 ffmpeg 媒体的三组样本端到端验证（生成 samples/ 与 output/）
make validate              # 等价于 bash scripts/validate_real.sh
```

常用参数：

```text
-in, -i          损坏的 MP4/MOV 输入（必填）
-out, -o         修复输出（默认 <input>.repaired.mp4），禁止与输入同路径
-ref             同型号设备的健康参考片，moov/stsd 完全丢失时借用解码配置
-fps 30          当时长表不可用时的兜底帧率
-dedupe-frames   重建索引时剔除连续字节相同的重复帧（载荷字节仍原样复制）
-moov-last       输出 ftyp+mdat+moov（默认 faststart）
-inspect         只扫描出报告，不写文件
-json            向 stdout 输出机器可读 JSON 报告
-report-file P   额外把 JSON 报告原子写入 P
```

## 三组样本验证

`repair/` 下的 Go 测试用独立的 `fixture` 包构造已知真值的 MP4（含 AUD/无 AUD、
moov 在前/在后布局），覆盖三组故障，并额外用本机 `ffmpeg` 做真实媒体解码验证
（没有 ffmpeg 时这些用例自动 skip）：

| 样本组 | Go 测试 | 断言 |
| --- | --- | --- |
| 截断 | `TestTruncatedGroup`、`TestRealTruncatedGroup`、`TestRealMissingMoovWithReference` | 只索引完整 sample；报告 `truncated_tail` 与 `missing_index_samples`；输出可解码 |
| 重复 sample | `TestDuplicateIndexGroupBuiltin`、`TestDuplicateIndexGroupSurgery`、`TestRealDuplicateGroup`、`TestDuplicateStreamFrames` | 扫描真值数不受重复记录影响；重复条目被识别；重建表唯一且偏移正确；码流重复帧可检测/去重 |
| 错误偏移 | `TestBadOffsetsGroup`、`TestRealBadOffsetsGroup` | 全部旧偏移失配被计数；重建后每条 `stco` 精确落在 NAL 长度前缀上 |

另有 `TestGarbageGapSplitsIntervals` 验证码流断档重同步与多区间，
`TestWriteNeverOverwritesSource` / `TestReplacesExistingGeneratedOutput`
验证安全落盘，`TestCLIRepairAndJSON` 验证命令行。

`make validate` 会在磁盘上生成并检查：

- `samples/source.mp4`、`samples/source-fast.mp4`：ffmpeg 生成的真实源片；
- `samples/cases/truncated.mp4`、`duplicate.mp4`、`badoffsets.mp4`：三组损伤；
- `output/*.repaired.mp4`：另行落盘的修复结果；
- `output/*.report.json` / `*.report.txt`：区间与未恢复部分报告；
- 对每个修复文件执行 `ffmpeg -f null` 解码并用 ffprobe 数帧。

## 报告中的“可播放区间 / 未恢复部分”

- **可播放区间 `intervals`**：一段字节连续、sample 完整的最大游程；区间从最近的
  关键帧开始（`first_sync_sample`），其前面的 P 帧计入
  `leading_non_sync_samples`（已重建索引，但不能独立作为起播点）。区间给出
  sample 范围、关键帧列表、秒级起止时间和源文件字节范围。
- **未恢复部分 `unrecovered`**：
  - `truncated_tail`：文件尾部不完整 NAL/sample 的字节区间；
  - `scan_gap`：码流中无法解释、被重同步跳过的破坏字节；
  - `no_sync_run`：完整但没有关键帧、无法独立起播的游程；
  - `missing_index_samples`：旧表声称、但扫描不到且不能用重复记录解释的 sample 槽位。

## 写入安全

- 源路径以只读方式打开；输出路径与源相同（含硬链接同一 inode）直接拒绝
  （`repair.ErrSourceDestinationSame`）。
- 先在目标目录创建 `.mp4repair-*.tmp`，写完、`fsync`、重新解析校验
  `stsz/stco` 数量与每条偏移边界后，才 `rename` 原子替换目标。
- 任何错误都会关闭并删除临时文件，绝不触碰源文件；成功/失败后源文件 SHA-256
  不变是测试中的硬性断言。
- `mdat` 载荷按区间逐字节复制，测试直接比较源与输出的 `mdat` payload；真实
  ffmpeg 样本（含 B 帧）修复前后逐帧 `framemd5` 完全一致。

## 目录结构

```text
main.go              CLI 入口（go run .）
bmff/                ISO-BMFF 盒头遍历与盒构造
movie/               moov/stbl 元数据解析（stsd/stts/ctts/stsz/stsc/stco/stss）
h264scan/            AVCC/HEVC NAL 扫描、AU 重组、Exp-Golomb、重同步
repair/              分析、区间/诊断、moov 重建、安全提交与全部 Go 测试
fixture/             确定性 MP4 样本工厂与三种损坏注入器
cmd/mkfixtures/      生成磁盘三组样本的辅助命令
scripts/validate_real.sh
```

## 限制

- 视频识别面向 H.264/AVC 与 H.265/HEVC 的长度前缀 NAL（MP4/MOV 常态）；Annex B
  start-code 主体可识别但未作为主路径。其他轨（如 AAC 音频）的原始 `trak` 会
  原样保留，仅把其中 chunk 偏移重映射到新布局；它们自身的损坏表不做扫描重建。
- `moov` 与带内参数集都彻底丢失时，需要同设备参考片 `-ref healthy.mp4` 提供
  `stsd/avcC` 与时标。
- 扫描置信度依赖 slice 头可解析；极端加密/私有封装可能需要补充 NAL 规则。
