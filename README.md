# 4:2:2 Chroma Siting 测试与修复

本项目只使用 Go 标准库，生成带像素标尺、彩色文字、1px 竖线、斜线和彩条的测试帧，并量化 4:2:2 色度站点解释错误。

## 坐标约定

- 左对齐（left / cosited）：第 `k` 个色度样本位于亮度像素 `2k` 的中心，坐标为 `2k`。
- 中心对齐（center / interstitial）：第 `k` 个色度样本位于亮度像素 `2k` 和 `2k+1` 之间，坐标为 `2k+0.5`。
- 把中心对齐素材误按左对齐复制，会把 `2k+0.5` 的样本放到 `2k`，相差 `0.5` 个色度采样单元，即 `1` 个亮度像素间隔。
- 正确的中心对齐重建按站点坐标做线性插值；模糊只扩大过渡宽度，不会移动插值零相位。

## 运行

```bash
go test ./...
go run . --outdir artifacts
```

输出：

- `artifacts/test_frame_source.png`：原始 RGB 测试帧。
- `artifacts/left_siting_correct.png`：左对齐素材的正确重建。
- `artifacts/center_siting_wrong_left.png`：中心对齐素材误按左对齐重建。
- `artifacts/center_siting_corrected.png`：站点感知的正确重建。
- `artifacts/blur_only_control.png`：只做模糊、不改相位的对照组。
- `artifacts/comparison.png`：四宫格对比图。
- `artifacts/metrics.json`：边缘位置、色度 MAE 和过渡宽度。

当前合成度量结果：

- 错误左对齐解释：边缘 MAE 为 `1.000` 个亮度像素。
- 站点感知修复：边缘 MAE 降为 `0.000`。
- 只模糊：边缘 MAE 仍约为 `1.006`，且过渡宽度从 `1` 扩大到 `5`。

因此修复依据是显式保留并使用色度站点坐标，而不是依赖模糊掩盖边缘误差。
