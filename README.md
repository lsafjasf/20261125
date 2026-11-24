# RGB ↔ YUV 实现对照（整数 / 定点 / 小数）

同一个输出在两套 RGB/YUV 公式（BT.601 与 BT.709）之间切换时，平均颜色接近，
但高饱和纯色出现系统性舍入偏差。本仓库用 Go 建立三类实现的可复算对照，逐项
检查矩阵顺序、offset、range 与最终量化，并给出 8/10-bit 的误差上限判定。

## 实现层次

| 实现 | 位置 | 系数表示 | 舍入 |
| --- | --- | --- | --- |
| 小数（参考基准） | `colorconv/float.go` | float64，归一化域 | `math.Round` 半入 |
| 定点 | `colorconv/fixed.go` | `2^Q` 整数系数，Q=12/16 | 可配：对称半入 / 算术右移截断 |
| 整数 | `colorconv/int.go` | 经典 Q=8 系数表（libyuv/IJG 风格） | 可配：带修正项 / 朴素 floor 移位 |

规格（矩阵 Kr/Kb/Kg、limited/full 边界、offset、量化区间）集中在
`colorconv/spec.go`，三类实现共用同一套定义，差异只在数值表示与舍入。

## 四个检查点

反面组合在 `colorconv/bugs.go`，由 `TestDefectControlsFail` 强制验证其失败：

1. **矩阵顺序/错配** — 编码 BT.709、解码 BT.601（`MatrixMismatchEncoder`），
   灰阶几乎不变，饱和纯色最大回环误差 37 LSB。
2. **offset 错位/缺失** — limited range 的 16/128 偏移被漏掉
   （`OffsetlessEncoder`），整体平移并在黑白端裁切，最大误差 255 LSB。
3. **range 混淆** — limited 编码、full 解码（`RangeConfusionEncoder`），
   最大误差 20 LSB；full 中性色度是 `max/2`（127.5/511.5），不是 128。
4. **最终量化** — 朴素 Q8 floor 移位（`NewNaiveInteger`）对负值向负无穷漂移，
   饱和纯色带符号 bias 显著为负（见下）。

## 声明的误差上限

完整 RGB→YUV→RGB 回环，码值（LSB）单位，同时满足三条才算通过
（`cmd/compare/tolerance.go` 与测试共用同一谓词）：

| 位深 | 最大绝对误差 | 平均绝对误差 | 带符号偏差 \|bias\| |
| --- | --- | --- | --- |
| 8-bit | ≤ 2 LSB | ≤ 0.5 LSB | ≤ 0.25 LSB |
| 10-bit | ≤ 6 LSB | ≤ 1.5 LSB | ≤ 0.75 LSB |

10-bit 的 LSB 预算是 8-bit 的 4 倍，因为一个 10-bit 码单位仅为 8-bit 的 1/4，
两者描述的是同一物理色差。`bias` 为带符号均值，专门防止系统性舍入偏差被
绝对值平均掩盖。

## 实测结论（摘要）

完整数据见 `artifacts/comparison.txt`（`go run ./cmd/compare` 重新生成）。

- **float64、定点 Q16 半入、定点 Q12 半入**：在 BT.601/BT.709 × limited/full
  × 8/10-bit 的全部 8 个规格下满足误差上限；前向 YUV 相对参考最大仅 1 LSB。
- **整数 Q8 半入**：硬 LSB 上限全部通过，但系数表过粗导致均值/偏差超标——
  8-bit 仅 BT.601 组合完全达标，10-bit 仅 BT.601 达标、BT.709 失败。
- **整数 Q8 floor 截断**：全部规格失败。8-bit 最大误差 3–4 LSB、整体 bias
  约 −1.3 ~ −1.7；在饱和纯色子集上 bias 为 −0.6 ~ −2.0，即颜色被系统性压暗，
  正是题目描述的“平均接近、纯色出偏差”。
- 矩阵错配、漏 offset、range 混淆、U/V 互换四类缺陷组合均被判定 FAIL。

### 满足声明上限的组合

> **对称半入舍入 + 正确的矩阵配对 + 正确阶段的 offset + 与 range 一致的
> 量化边界**。具体实现上，float64 参考与定点 Q12/Q16 半入在全部 8 个规格
> （BT.601/BT.709、limited/full、8/10-bit）通过；Q8 整数仅在 BT.601 下通过，
> 朴素截断与任何缺陷组合均不通过。

## 运行

```sh
go test ./...                 # 固化的通过/失败判定
go run ./cmd/compare          # 打印全量对照表，退出码 0
go run ./cmd/compare -strict  # 任一组合超标时退出码非 0，供 CI 使用
```

样本集（`colorconv/samples.go`）覆盖立方体 8 个极值角点（高饱和纯色）、
灰阶 ramp、粗网格与 2000 个确定性随机样本，8-bit 与 10-bit 同构生成。
