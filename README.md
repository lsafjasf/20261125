# cbarcheck — 彩条/灰阶可量化合规检查（Go 单模块）

仓库未附带原始故障视频或解码帧，因此无法声称以下坐标来自真实素材。本工具用
可复现的 synthetic 转码样例完成检查：目标为 limited / BT.709 / 10-bit，错误
样例实际按 full / BT.601 / 10-bit 生成。所有计算只使用 Go 标准库。

## 当前检查结论

运行 `go run ./cmd/cbarcheck demo` 后：

- 正确样例：`PASS`，0 个亮度违规、0 个色度违规。100% 饱和红/蓝等彩条码值
  合法，未被当成色彩错误。
- 错误样例：`FAIL`。首个违规像素为 **`(x=0, y=0)`，`white100` 区域**：
  - 原因：`illegal_luma, luma_level`
  - 实测 `Y/Cb/Cr = 1023/512/512`
  - 目标 `Y/Cb/Cr = 940/512/512`
  - 亮度误差 `20.75` 个 8-bit 码值
  - 合法码域 `Y[64,940] Cb[64,960] Cr[64,960]`
- 错误样例候选比较首选 `full/bt601/10-bit`，得分 `0.227`；目标
  `limited/bt709/10-bit` 得分 `31.234`，判定存在范围/矩阵错配。
- 首个色度违规为 **`(x=80, y=0)`，`yellow100` 区域**：实测
  `Cb/Cr=0/595`，目标 `64/553`；该样例共有 `76,800` 个像素色度码值越界。
  JSON 同时保留 `firstViolation`、`firstLumaViolation`、
  `firstChromaViolation` 和逐区域 `candidates`。

完整结果在：

- `artifacts/cbarcheck-demo/clean-report.json` / `.md`
- `artifacts/cbarcheck-demo/bad-report.json` / `.md`

## 判定模型

对每个区域分别比较全部 18 种假设：

```text
{limited, full} × {BT.601, BT.709, BT.2020} × {8, 10, 12}-bit
```

目标是独立输入。候选比较用于诊断实际码流更像哪一种范围/矩阵/位深，但
**不会取消目标违规**。每个候选输出亮度、Cb、Cr 的平均 8-bit 等效误差、越界
像素数和综合得分；JSON 中保留每个区域的完整 18 项，Markdown 中列出全局排名。

目标合规判定只看两类证据：

1. **码域**：limited 8-bit 为 `Y 16..235`、`Cb/Cr 16..240`；10/12-bit 按
   `2^(depth-8)` 缩放。full 为 `0..2^depth-1`。
2. **期望值偏差**：相对已知区域的 R'G'B' 经目标矩阵和范围编码后的码值。
   默认容差为亮度/色度各 2 个 8-bit 码值，可用参数调整。

像素按行优先扫描，第一个失败像素同时给出坐标、区域、原因、实测值、目标值、
8-bit 等效误差和合法边界。原因码为 `illegal_luma`、`illegal_chroma`、
`luma_level`、`chroma_level`。

**饱和色不是错误本身。** 例如 100% red 在 limited BT.709 下 `Cr=240*scale`，
恰在合法上界；只要期望色度与其匹配、码值未越界，就判定通过。只有越界或偏离
期望色度超过阈值才算 `chroma` 违规。

## 输入与用法

检查器读取一帧 planar `yuv444p`。8-bit 每样本 1 字节；10/12-bit 每样本为
小端 `uint16`：

```bash
ffmpeg -i input.mp4 -frames:v 1 -pix_fmt yuv444p10le -f rawvideo frame.yuv

go run ./cmd/cbarcheck check \
  -input frame.yuv -width 1920 -height 1080 -depth 10 \
  -target-range limited -target-matrix bt709 -target-depth 10 \
  -pattern ebu75 -out reports
```

内置布局为上方三分之二 8 根彩条、下方三分之一 11 级 0%..100% 灰阶。
`-pattern` 支持 `ebu75` 和 `smpte100`。退出码：

- `0`：目标合规且没有配置错配；
- `1`：存在像素违规或候选配置显著优于目标；
- `2`：参数、布局或文件格式错误。

演示命令：

```bash
go run ./cmd/cbarcheck demo
go run ./cmd/cbarcheck demo -out /tmp/cbarcheck -write-yuv
```

## 关键实现

- `color.go`：BT.601/709/2020 矩阵，limited/full 正反变换，码域边界。
- `pattern.go`：EBU 75% / SMPTE 100% 彩条与 11 级灰阶布局、合成参考帧。
- `audit.go`：18 候选逐区域评分、行优先首个违规像素、目标合规统计。
- `raw.go`：8/10/12-bit planar YUV 4:4:4 读写。
- `cmd/cbarcheck`：检查、演示、JSON/Markdown 报告与退出码。

## 验证

```bash
go test ./...
go vet ./...
```

测试覆盖：8/10/12-bit 干净 100% 饱和彩条零误报、full/BT.601 相对
limited/BT.709 的首个违规像素、位深错配候选识别、指定坐标的首个违规像素，
以及原始帧读写往返。

边界：工具不解码压缩视频，不猜测未提供的区域几何；实际素材应先用与生产
转码一致的矩阵/范围导出为 4:4:4 帧。若素材含缩放、裁切或非标准彩条布局，
应先将其对齐到内置布局，后续可扩展自定义 layout 清单。
