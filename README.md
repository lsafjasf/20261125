# videotear

一个纯 Go 标准库实现的录像帧内断层检测器，编译为单个无 CGO 二进制。它逐帧寻找水平撕裂线、奇偶场错位、网格对齐的编码块损坏和游戏/运动区域，并通过跨帧轨迹保留断层位置、连续性与置信度。

## 构建

```bash
go test ./...
CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o bin/videotear .
```

产物是 `bin/videotear`。核心检测库位于 `internal/vdetect`，无第三方 Go 依赖。

## 输入

- PNG/JPEG：文件、glob、目录或重复的 `-i` 参数。
- Raw：`.gray`、`.rgb24`、`.yuv/.i420`，需提供 `-width`、`-height`、`-pixel-format`。
- 常见视频：`.mp4`、`.mkv`、`.mov`、`.ts` 等；如果系统存在 `ffmpeg`/`ffprobe`，二进制会自动探测分辨率/FPS 并通过 raw RGB 管道解码。
- 也可用 `FFMPEG_BIN`、`FFPROBE_BIN` 指定解码器路径。

视频容器解码是可选外部能力；PNG/JPEG/raw 路径完全内置。对于严格无 FFmpeg 的环境，可先解码成图片序列或 raw 帧。

## 常用命令

```bash
# 图片目录，输出格式化 JSON 报告
./bin/videotear -format json -o report.json /path/to/frames

# 实时友好的 JSONL：每行一帧，最后一行是 tracks/summary
./bin/videotear -format jsonl /path/to/frames

# 人工阅读
./bin/videotear -format text video.mp4

# Raw YUV420P
./bin/videotear -width 1920 -height 1080 \
  -pixel-format yuv420p -fps 29.97 input.yuv

# 高灵敏度/更低报告门槛
./bin/videotear -sensitivity 0.75 -min-confidence 0.45 frames/
```

## 输出模型

每帧包含：

- `classification` / `labels`：主分类和多标签。
- `tears[]`：断层的源分辨率 `y`、`x_start/x_end`、水平错位像素、空间覆盖率、零位移代价、对齐代价、证据和置信度。
- `field_order`：奇偶场视差、梳齿行支持、置信度。
- `block_defects[]`：16px/8px 网格块框、边界突增、内部纹理、色偏、帧间突变和证据。
- `effects[]`：游戏特效或运动前景区域。
- `guards`：平均亮度、亮度斜率、全局渐亮、运动/前景/特效覆盖率和被抑制候选数。

报告级 `tracks[]` 保存跨帧点列、轨迹连续性、置信度、`velocity_y_per_frame`、`monotonic_ratio` 和回绕次数。

分类值：

- `tear`：单条连贯水平移位断层。
- `capture_rolling`：断层位置跨帧近似匀速移动或回绕，符合采集/滚动扫描。
- `field_order_error`：大量行对呈现一致奇偶场视差/梳齿。
- `macroblock_corruption`：强网格、时间突发、异常纹理/色偏且未被运动上下文解释的块损坏。
- `game_effect`：高饱和/高运动特效区域。
- `mixed`：同帧存在多个高置信类别。

## 防误报设计

- 使用横向梯度匹配，降低全局亮度变化影响；渐亮由亮度斜率和去均值帧差二次确认。
- 断层要求跨多个块的同向移位一致和足够宽度覆盖，普通水平边缘不会仅凭两侧亮度差上报。
- 运动估计先做全局平移补偿，再统计局部残差；运动前景形成连续上下文，而编码块通常是运动孤岛。
- 块检测要求网格边界、8/16px 结构、时间突发或异常色偏等证据组合；轨迹级再排除持续运动纹理。
- 断层线附近的网格候选归为撕裂响应，避免同一断层被重复报成宏块损坏。

## 验证

`internal/vdetect/classification_test.go` 覆盖：

- 固定断层的位置、位移、置信度和轨迹连续性。
- 滚动断层的速度、单调性和 `capture_rolling` 分类。
- 场序错误。
- 编码噪声块及第二帧轨迹滑行保持。
- 渐亮序列零结构误报。
- 大幅运动前景零断层/块/场序误报。
- 偶发断层在时间间隔后仍关联为同一条轨迹。
