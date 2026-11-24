# rotationaudit — 视频旋转矩阵/显示宽高/真实像素分离审计（Go 单模块）

这是一个无第三方 Go 依赖的单模块命令。它不把“文件声称横屏”“元数据里有 90°”
或“播放器看起来是横屏”混为一谈，而是分别记录：

1. **旋转矩阵**：MP4/MOV `tkhd` 中的 3×3 matrix、FFprobe side-data
   Display Matrix、`rotation` 字段和旧 `rotate` 标签。
2. **显示宽高**：`tkhd.width/height`（16.16 定点数）、SAR/DAR，以及按矩阵
   换轴后的几何尺寸。
3. **真实像素方向**：用 `ffmpeg -noautorotate` 抽出的原始编码帧，并用 Go
   标准库直接读取 PNG 宽高；语义上的“F 是否正向”可由参考图或显式角度确认。

工具会输出 JSON/Markdown 报告、原始像素帧、矩阵播放器帧、二次旋转复现帧、
四张 raw 旋转候选和修正后的对照拼图。

## 快速运行

依赖：Go 1.27+，以及 PATH 中的 `ffmpeg` / `ffprobe`。也可用
`-ffmpeg /path/ffmpeg -ffprobe /path/ffprobe` 指定。

```bash
go build -o rotationaudit .

# 审计你自己的视频；不需要参考图也会生成四张方向候选
./rotationaudit -input input.mp4 -out rotation-out

# 若有正向参考图，工具会自动比较 raw/cw0,cw90,cw180,cw270
./rotationaudit -input input.mp4 -reference upright.png -out rotation-out

# 或人工确认原始像素还需顺时针 270° 才正向
./rotationaudit -input input.mp4 -pixel-correction-cw 270 -out rotation-out
```

常用别名：`-i` / `-video` / `-input`，`-o` / `-out` / `-output`；也支持
`rotationaudit input.mp4` 位置参数。

## 复现仓库中的故障样本

仓库已提交一个 180×320 的“已烘入一次顺时针旋转”的像素样本，并在 `tkhd`
中写入顺时针 90° 矩阵和 320×180 的横屏显示宽高：

```bash
go run . -make-demo examples/double-rotation.mp4
go run . -input examples/double-rotation.mp4 \
  -reference examples/double-rotation.reference.png \
  -out examples/double-rotation-frames
```

已生成的对照结果在
`examples/double-rotation-frames/rotation-report.md` 和
`examples/double-rotation-frames/comparison.png`。

这个样本同时触发：

- 真实像素：`180×320`，portrait；
- `tkhd` 显示宽高：`320×180`，landscape；
- 矩阵：`[0,1,0;-1,0,0;0,0,1073741824]`，FFprobe 标为 `-90°`，
  本工具按实测换算为**顺时针 90°**；
- 参考图证明 raw 像素还需顺时针 `270°` 才正向，而矩阵只给顺时针 `90°`，
  遵循矩阵的画面仍残余 `180°` 错误；
- `tkhd` 还预先写成矩阵后的 `320×180`，导致额外的 SAR/换轴冲突。

## 输出帧含义

| 文件 | 含义 |
|---|---|
| `01-raw-pixels-noautorotate.png` | `ffmpeg -noautorotate` 抽帧，代表未经矩阵处理的真实编码像素。 |
| `02-matrix-respecting-player.png` | FFmpeg 默认 autorotate 抽帧，近似遵循 Display Matrix 的播放器结果。 |
| `03-double-rotated-autorotate-plus-manual.png` | 在第 2 帧上再施加一次相同矩阵旋转，复现“默认自动旋转后又手动 transpose”。 |
| `candidate-raw-cw{0,90,180,270}.png` | 从 raw 像素出发的四个唯一旋转候选，供人工选择。 |
| `04-corrected.png` | 有参考图或显式角度时生成的修正帧；否则生成 `04-matrix-applied-unverified.png`。 |
| `comparison.png` | 1=raw，2=矩阵播放器，3=二次旋转管线，4=所选修正。 |
| `rotation-report.json` / `.md` | 三类事实、C1–C5 判定、帧哈希和复测命令。 |

## 哪些组合会导致二次旋转

报告中的 `doubleRotationCombinations` 会逐项给出 `triggered/status/severity`。

- `C1-landscape-claim-portrait-pixels-quarter-matrix`  
  横屏显示声明 × 竖屏 raw 像素 × 90/270 矩阵。尺寸上可能恰好自洽，因此
  **不能只凭宽高定罪**；若参考图/显式方向显示所需旋转与矩阵不同，升级为
  已确认的二次/冲突旋转。

- `C2-baked-pixels-plus-active-display-matrix`  
  像素在编码前已经被旋转/烘正，但非单位矩阵仍保留。矩阵感知播放器会再转
  一次。若“烘入方向”和矩阵同向，90+90 常得到 180° 颠倒画面。

- `C3-default-autorotate-plus-manual-transpose`  
  FFmpeg/许多播放器默认 autorotate；此时再按 ffprobe 的 rotation 手动加
  `transpose/rotate`，就是在已归一化的帧上第二次应用旋转。报告生成的
  `03-double-rotated-*.png` 直接复现该错误。

- `C4-legacy-rotate-tag-plus-display-matrix`  
  旧 `rotate=90` 标签和 Display Matrix 通常是同一元数据的不同表示，不是
  两次独立变换；但自定义播放器若分别转成滤镜和 side-data 变换，就会转两次。
  两者角度不一致时是高优先级元数据冲突。

- `C5-display-width-height-vs-matrix-axis-swap`  
  方形像素的普通 MP4/MOV 中，`tkhd` 宽高通常位于矩阵前的 track 坐标系，
  即与编码宽高一致，换轴由矩阵完成。若 `tkhd` 已预先写成矩阵后的宽高，
  同时矩阵仍然生效，FFmpeg 会反推出非 1 SAR；SAR 感知旋转后还可能取倒数，
  造成“尺寸是横的、显示比例却是竖的”等播放器差异，相当于显示尺寸被重复换轴。

安全组合是：参考方向所需的 raw 旋转量 `C` 等于矩阵旋转量 `M`，且没有第二个
手动旋转滤镜；或者像素已烘正（`C=0`）且矩阵为单位矩阵。

## 方向判定边界

`width > height` 只能说明**像素网格**是横版还是竖版，不能判断内容中的文字、
人脸或 F 标记是否正向。没有原始正向参考时，任意视频都无法仅靠几何元数据
证明语义方向已经被烘入像素。因此工具会：

- 确定已知事实：像素网格方向、矩阵 quarter-turn、显示宽高、SAR/DAR；
- 标记 `suspect` / `needs_visual_check`，而不是武断声称所有横竖不一致都是
  二次旋转；
- 生成全部四张 raw 旋转候选；
- 可用 `-reference upright.png` 通过降采样 RGB 平均绝对误差自动选择，或用
  `-pixel-correction-cw` 接受人工判定。

## 矩阵符号约定

FFprobe 的 `rotation` 使用带符号的逆时针角度语义。在 FFmpeg 7 上实测：

```text
[0, 65536, 0; -65536, 0, 0; 0, 0, 1073741824]
FFprobe rotation = -90
autorotate 视觉结果 = 顺时针 90°
```

所以报告统一输出 `turnsClockwise = normalize(-round(ffprobeRotation / 90))`，
避免把 `rotation=-90` 再错误地顺时针/逆时针解释一遍。

## 复测命令

```bash
go test ./...
go vet ./...
go build -o rotationaudit .
```

测试包含：

- 已知 Display Matrix 的顺/逆时针换算；
- FFprobe `-90/90/180` 符号；
- MP4 `tkhd` 矩阵和 16.16 显示宽高解析/补丁；
- C1/C2/C5 的安全、可疑、已确认组合；
- 若本机存在 `ffmpeg`/`ffprobe`，自动创建合成 MP4 并跑完整 CLI、参考图
  方向推断和帧产物断言。

## 模块结构

这是仓库根目录下唯一的 Go module（`module rotationaudit`），没有嵌套
`go.mod`：

- `main.go`：CLI 与输出编排；
- `probe.go`：FFprobe JSON 适配；
- `mp4.go`：ISO BMFF/MP4/MOV box、`tkhd` 解析和矩阵补丁；
- `geometry.go`：方向、矩阵、SAR/DAR 纯逻辑；
- `diagnose.go`：C1–C5 组合判定；
- `frames.go`：FFmpeg 抽帧、Go 旋转、参考图比较、对照拼图；
- `demo.go`：可复现故障样本和方向标记；
- `report.go`：JSON/Markdown 报告。

## 实践建议

- 抽“真实像素帧”必须加 `-noautorotate`；普通 `ffmpeg -i in.mp4 out.png`
  可能已经应用矩阵。
- 修复链路只能选择一次旋转：
  - 保留矩阵：不要在 autorotate 后加 `transpose`；
  - 烘入像素：使用 `-noautorotate` + 唯一正确的 `transpose`，并把容器矩阵
    清除/写成单位矩阵；
  - 自定义播放器：先确认它是否自动应用 side-data/`tkhd` matrix，再决定是否
    读取旧 `rotate` 标签，不能两者叠加。
- 不要把 DAR 或 SAR 当成像素分辨率；它们只描述缩放比例。
