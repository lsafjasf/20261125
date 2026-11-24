# 20261125

纯 Go 播放卡死检测器（仅标准库，`CGO_ENABLED=0` 可构建）。

## 判定模型

两条信号独立评估，不再把"画面不变"直接等同于卡死：

- `PictureStatic`：画面层是否静止，基于像素变化比例与平均灰度差。
- `TimeStalled`：PTS 是否连续不前进，与像素内容完全无关。

综合结论（`types.go` 中的 `Verdict`）：

| 画面 | 时间轴 | 结论 |
| --- | --- | --- |
| 变化 | 前进 | `normal` |
| 静止 | 前进 | `still_scene`（长静止镜头，非卡死） |
| 变化 | 停止 | `time_stalled`（时间戳异常） |
| 静止 | 停止 | `frozen`（采集/播放卡死） |

确认窗口（`StallMinConfirms`、`FreezeMinConfirms`、`StillMinConfirms`）
避免单帧抖动触发结论。

## 三类证据

证据写入 `FrameReport.Evidence`，只修正画面静止判断，不参与时间轴判断：

- 低照噪声 `HasLowLightNoise`：暗场传感器残差方差，噪声抖动不算画面运动。
- 闪烁 `HasFlicker`：整体亮度跳变，证明采集链路仍在输出，防止误判冻结。
- 字幕变化 `SubtitleChanged`：字幕/OSD 层独立比较，字幕切换不影响画面层。

## 人工修正

`Override(index, verdict)` 可修正任意单帧结论，报告保留 `Overridden`
标记与修正记录，且不影响其他帧的自动判定。

## 用法

```go
d := freezedetect.NewDetector(freezedetect.DefaultConfig())
for frame := range stream {
    report, err := d.Push(freezedetect.Frame{
        Img: frame.Image, Subtitle: frame.OSD, PTS: frame.PTS,
    })
    // report.Verdict / report.PictureStatic / report.TimeStalled
}
_ = d.Override(42, freezedetect.VerdictStillScene) // 人工修正单帧
```

## 复测

`go test ./...` 覆盖：正常静物、静态编码量化抖动、采集冻结、
低照噪声、字幕切换、闪烁防误冻、时间戳停滞、人工修正、冻结恢复。
