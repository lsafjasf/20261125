# Rec.2020 竖屏 → Rec.709 高光三版对比

纯 Go 标准库实现的单二进制/帧序列工具。它把 Rec.2020 RGB 经 D65 XYZ 转到 Rec.709，再用相同的负色域压缩隔离高光策略，生成：

- `hard_clip/`：逐通道硬裁切到 `[0,1]`
- `fixed_shoulder/`：所有场景共用 `knee=0.75` 的 C1 软肩
- `adaptive_shoulder/`：鲁棒统计 + 时间中值 + 滞回/限速的场景自适应软肩

默认还会输出 `report.json`、`frame_metrics.csv`、`report.md`、`adaptive_decisions.json` 和对比 PNG。仓库内已经生成一套 12 帧、`540x960` 的确定性竖屏诊断片及 MP4。

## 构建与运行

```bash
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/rec709shoulder .

# 无素材时：生成 Rec.2020 PFM 源、三版 PNG、指标和对比图
./bin/rec709shoulder all -out outputs

# 处理自己的 PNG/JPEG/PFM 帧序列
./bin/rec709shoulder process \
  -input /path/to/frames \
  -out outputs-my-video \
  -transfer rec709 \
  -range full
```

旧的 SDR Rec.2020 母版通常使用 BT.709 风格 OETF；程序按完整电平解码。若抽帧保留了 16–235 legal range，加 `-range limited`。PQ 帧可用 `-transfer pq -exposure 0`，程序会按 100 nit 参考白自动乘 `100`；HLG 建议先用 ffmpeg 转成线性帧或 PFM。

从视频抽帧并编码 Rec.709 MP4：

```bash
ffmpeg -i input_rec2020.mov -vsync 0 frames_in/frame_%04d.png
./bin/rec709shoulder process -input frames_in -out outputs_job

for v in hard_clip fixed_shoulder adaptive_shoulder; do
  ffmpeg -y -framerate 30 -i "outputs_job/$v/frame_%04d.png" \
    -c:v libx264 -crf 16 -pix_fmt yuv420p \
    -color_primaries bt709 -color_trc bt709 -colorspace bt709 \
    "outputs_job/${v}_rec709.mp4"
done
```

## 颜色与肩曲线

所有版本共享：

1. 输入信号反 OETF 到线性 Rec.2020 RGB。
2. `Rec.2020 → XYZ D65 → Rec.709` 矩阵。
3. 对负通道做保持 Rec.709 亮度 `Y` 的去饱和，避免把色域映射和高光肩混在一起。

软肩公式为：

```text
f(x) = x,                                  x <= knee
f(x) = 1 - (1-k) exp(-(x-k)/(1-k)),       x > knee
```

它在 `k=knee` 处函数值和一阶导数连续，并渐近到 1，因此不会像硬裁切那样产生突然的纹理平台。默认使用 max-RGB 比例肩：先压缩 `max(R,G,B)`，再把同一比例应用到三通道，以保持 RGB 比例和色相；如需观察常见逐通道肩的色相漂移，加 `-hue-preserve=false`。

## 自适应触发

每帧先计算鲁棒特征：P95/P99.5 max-RGB、`max(R,G,B)>1` 超域面积、`max>0.75` 高光面积、Rec.709 亮度 Sobel 梯度的高光/全局比，以及亮度直方图切换分数。

压力分：

```text
pressure = .30*over + .25*headroom + .20*area + .25*detail
knee     = clamp(.86 - .24*pressure, .62, .86)
```

显式审计触发条件为：

- 超域像素 `>=1.20%`
- P99.5 max-RGB `>=1.20`
- 高光面积 `>=12%`
- 高光/全局梯度比 `>=1.25`，且高光面积至少 `2.5%`

防抖不是只靠阈值：所有标量先经过默认 5 帧居中中位数；压力进入/退出使用 `0.58/0.43` 与 `0.20/0.34` 滞回，且 texture/specular 状态需连续两次观察；最终 knee 每帧最多变化 `0.035`。因此单个噪声帧即使原始 P99.5 或超域比例尖峰，也不会直接驱动参数。离线居中窗口会使用少量后续帧；流式处理可把窗口改为过去 N 帧并保留同样的中位数、滞回和限速。

每帧的原始统计、稳定统计、命中的条件、原因文本和 knee 都写入 `adaptive_decisions.json`。

## 指标与图

- 色相：Rec.2020 源和 Rec.709 输出都转到 CIE Lab/D65，计算 hue angle 的最短角差；低彩度像素不参与角差，另统计高彩度源被去饱和到近灰的比例。
- 亮度：平均 Rec.709 Y、平均 CIE L*、P95 Y、输出峰值/裁切像素比例。
- 纹理：Rec.709 Y 的 Sobel 梯度能量保持率；除全局/高光区外，还用源图 `maxRGB>1.15` 且八邻域 `>1.05` 的腐蚀掩膜统计“严重高光内部纹理”，避免硬裁切边界的强梯度造成误判。
- 图：`contact_compare.png` 左中右依次为硬裁切、固定肩、自适应；`hue_shift_map.png`、`texture_map.png`、`clip_map.png` 是同序热力/掩膜图。

运行测试：

```bash
go test ./...
```

## 已生成样例的实测

样例为 12 帧、`540x960`，默认色相保持软肩；汇总自 `outputs/report.json`：

| 版本 | 平均色相偏移 | P95 色相 | >10° 像素 | 平均 L* | 峰值/裁切像素 | 全局纹理保持 | 严重高光内部纹理 |
|---|---:|---:|---:|---:|---:|---:|---:|
| `hard_clip` | 2.66° | 13.08° | 7.77% | 54.19 | 14.27% | 0.776 | 0.159 |
| `fixed_shoulder` | 2.09° | 11.97° | 6.01% | 53.71 | 1.69% | 0.743 | 0.176 |
| `adaptive_shoulder` | 2.09° | 11.98° | 6.01% | 53.60 | 0.46% | 0.739 | 0.180 |

解读：

- **色相**：两种 max-RGB 软肩的平均/P95 色相偏移均小于硬裁切；比例肩保持通道比例，自适应改变 knee 时几乎不再引入额外色相变化。逐通道肩可用 `-hue-preserve=false` 复现，它会显示更明显的色相/饱和取舍。
- **亮度**：硬裁切平均 L* 略高，但那是削平高光后的“死白”；软肩平均亮度略低，却把 14.27% 的触顶像素降到固定肩 1.69%、自适应 0.46%。
- **纹理**：软肩的全局 Sobel 能量保持率略低，因为它主动降低高光反差；但在最容易被硬裁切抹掉的严重高光内部，纹理保持从 `0.159` 提升到固定肩 `0.176`、自适应 `0.180`，对应对比图中的白核缩小和内部条纹恢复。
- **自适应**：样例第 4 帧是单帧噪声，原始 P99.5 max-RGB 冲到约 `3.76`，稳定窗口仍为约 `0.94`，knee 保持 `0.800`；第 7 帧开始的持续亮场景才在限速下从 `0.800` 过渡到 `0.634` 并进入 `texture_guard`。

可直接查看 `outputs/three_way_compare_rec709.mp4`（左 hard、中 fixed、右 adaptive）和三个独立 MP4。MP4 是用系统 ffmpeg 编码的可选交付封装；Go 二进制本身不依赖 ffmpeg 即可生成 PNG/PFM、JSON、CSV 和对比图。
