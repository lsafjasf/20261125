# vfrmap

变速（VFR）视频的可逆镜头映射库，仅依赖 Go 标准库（`math/big`、`sort` 等）。
解决的问题：变速后的输出帧要能回到源帧编辑；用户移动切点后，音轨与字幕
位置随之重算；映射不唯一时返回候选而不是静默猜一个。

## 时间模型

- 所有时间用整数 tick 表示，`TickRate = 90000`（每秒 9 万 tick，与 MPEG-TS
  的 PTS 时钟一致），类型为 `Ticks`。
- `Segment` 是一段恒速区间：源时间 `[SrcStart, SrcEnd)` 以有理数速度
  `Num/Den` 播放，映射到输出 `[OutStart, OutEnd)`。
- `BuildTimeline` 校验区间连续、速度为正，并推导各段输出边界。
- 精确映射值用 `ExactTime`（内部 `big.Rat`）保存；`Ticks()` 向下取整。
  因此 `Forward` 后再 `InverseExact` 可以无损往返，不受整数取整影响。

## 核心能力

- **速度段**：`Mapper.Forward`（源→输出）与 `Mapper.InverseTime/InverseExact`
  （输出→源），乘法用 `big.Rat` 精确计算，不溢出 int64。
- **稀疏采样**：`SampleConfig.SourceFrameInterval` 每 n 帧保留一帧；落在采样
  间隙里的输出 PTS 反查时返回前后两个候选，原因标记为 `ReasonSparseGap`。
- **重复 PTS**：`MinOutputInterval` 控制慢动作下同一源帧的重复输出，重复副本
  共享 `SrcPTS`、拥有独立 `OutPTS` 与 `Repeat` 序号；反查重复 PTS 时
  `Mapping.Unique()` 为 false，候选带 `ReasonRepeatedPTS`。
- **段接缝**：输出 PTS 恰好落在速度段边界时 `Mapping.Boundary` 为 true。
- **切点编辑**：`Editor.MoveCut` 按输出 PTS 移动切点；映射不唯一时不落地，
  返回候选，由 `MoveCutSource` 用选定的源 PTS 提交。越序移动返回
  `ErrInvalidTimeline`。
- **音轨/字幕重算**：`Marker` 锚定在源时间，`Editor.Recalc` 每次把全部锚点
  投影到当前输出时间轴，切点移动后位置自动正确。

## 快速上手

```go
tl, _ := vfr.BuildTimeline([]vfr.Segment{
    {SrcStart: 0, SrcEnd: 4 * vfr.TickRate, Num: 2, Den: 1}, // 2 倍速
    {SrcStart: 4 * vfr.TickRate, SrcEnd: 7 * vfr.TickRate, Num: 1, Den: 2},
    {SrcStart: 7 * vfr.TickRate, SrcEnd: 10 * vfr.TickRate, Num: 1, Den: 1},
})
mapper := vfr.NewMapper(tl)

src := vfr.SourceFrames(vfr.TickRate/2, 20)
frames, _ := vfr.Sample(tl, src, vfr.SampleConfig{
    SourceFrameInterval: 2,                 // 稀疏：隔帧保留
    MinOutputInterval:   vfr.TickRate / 4, // 重复 PTS 节奏
})

mapping, _ := mapper.MapOutput(outPTS, frames)
if !mapping.Unique() {
    // 让用户在 mapping.Candidates 里选，再用 MoveCutSource 提交
}

ed, _ := vfr.NewEditor(tl, []string{"cut-a", "cut-b"}, markers)
res, _ := ed.MoveCut("cut-a", newOutPTS, frames)
placed := ed.Recalc() // 音轨/字幕新位置
```

可运行示例：`go run ./cmd/vfrdemo`；测试：`go test ./...`。

## 包结构

- `timeline.go`：时间类型、速度段、时间轴构建与校验。
- `mapper.go`：精确正/逆向时间映射、候选与歧义原因。
- `frames.go`：源帧、稀疏采样、重复 PTS 与按输出 PTS 反查。
- `edit.go`：切点移动、候选提交、音轨/字幕锚点重算。
- `*_test.go`：往返精度、采样、重复/稀疏候选、切点重算等用例。
