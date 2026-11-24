# artifactdiag

局部视频伪影诊断工具，**仅使用 Go 标准库**实现分析与渲染；ffmpeg/ffprobe 仅通过
`os/exec` 作为外部解码器被调用（Go 代码本身不依赖任何第三方包）。

它把每一帧切成 16×16（宏块大小）的 tile，对暗部色带、文字边缘振铃、压缩块效应做
空间-时间联合诊断，并明确区分真实内容边缘、快速运动差异与调色/海报化问题，输出热点
帧、区域坐标、编码参数、热力叠加图和可放大裁剪图。

## 区分逻辑

| 类别 | 判据 |
| --- | --- |
| `motion` | 时间梯度：平均变化量与强变化像素占比同时超阈值，并做 1-tile 膨胀覆盖运动影响区。快速运动只标注、不计伪影 |
| `real_edge` / `text_edge` | Sobel 强边缘；文字为高密度边缘 tile。有参考时，参考中已存在的边缘判定为内容 |
| `blocking_artifact` | 8px 编码网格上的边界能量显著高于 tile 内部能量；有参考时要求源画面该区域平滑（网格突变为压缩新引入） |
| `ringing_artifact` | 边缘邻域盒式模糊残差的符号振荡（回声/halo），并用回声能量/边缘能量比率与干净锐边区分 |
| `banding_compression` | 暗部（luma ≤ 80）平滑区直方图出现空档、占用灰度级稀少，且伴随块边界能量 |
| `banding_tone_posterization` | 同样的稀疏台阶，但块能量极低、台阶跨度大——属于调色/海报化而非编码量化 |
| `clean` | 平坦无结构区域 |

同一 tile 在相邻帧重复出现同类伪影时标记 `persistent` 并加权，降低单帧误报。

## 用法

```bash
go build -o artifactdiag .

# 视频 + 干净参考（推荐，运动掩膜和内容/伪影区分最准）
./artifactdiag -input clip.mp4 -ref source.yuv -width 1920 -height 1080 -out report_out

# 只有视频：内部用自身时间梯度估计运动
./artifactdiag -input clip.mp4 -out report_out

# 也支持 PNG 帧目录或裸 I420
./artifactdiag -input frames/ -fps 25 -out report_out
./artifactdiag -input clip.yuv -width 320 -height 256 -fps 25 -out report_out
```

主要参数：`-top`（渲染的热点帧数）、`-zoom`（裁剪放大倍数）、`-max-frames`、
`-debug`（打印 tile 特征分布用于校准阈值）。

## 输出（`-out` 目录）

- `report.json`：编码参数（codec/profile/分辨率/帧率/码率/B 帧/关键帧索引）、每帧每
  tile 标签与特征、热点区域 bbox、汇总统计。
- `frame_scores.csv`：逐帧各类标签计数与伪影总分。
- `frame_XXXX_overlay.png`：热点帧热力叠加图，含 8px/16px 编码网格和热点白框。
- `frame_XXXX_zoom_*.png`：热点区域最近邻放大裁剪（像素级可见），叠加 8px 网格。
- `index.html`：自包含查看器（无外部依赖），方向键切换热点帧/区域，点击图片二次放大。

## 合成验证素材

`cmd/mkfixture` 用纯标准库生成确定性测试序列（暗部平滑渐变、锐利点阵文字、8 级海报
化区、高速运动方块），可配合 ffmpeg 压制端到端验证：

```bash
go run ./cmd/mkfixture -yuv testdata/src.yuv -pngdir testdata/ref_png -width 320 -height 256
ffmpeg -f rawvideo -pix_fmt yuv420p -s 320x256 -i testdata/src.yuv \
  -c:v libx264 -crf 38 -x264-params keyint=8:bframes=2 testdata/clip.mp4
```

## 测试

```bash
go test ./...
```

单元测试覆盖：运动 tile 不得误报为伪影、8px 网格块效应、平坦区/真实边缘分类、暗部量
化色带、连通域热点聚类。

## 局限

- 无参考输入时块效应判定更保守也更容易误报；提供同源未压缩/高码率参考可显著提升区分度。
- 分析基于 8-bit 亮度平面，色度伪影（色度色带/色度溢出）暂不单独建模。
- 阈值在 `Config` 中集中暴露，可按内容类型（动画、实景、字幕卡）通过 `-debug` 校准。
