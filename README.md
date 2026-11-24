# Reversible nested OTIO flatten/unflatten

这个仓库用 Go 标准库实现嵌套 OTIO 子集的可逆扁平化。普通 flatten 只保留线性
clip 列表，因而丢失父序列；这里的每个扁平叶子都携带一条按“发生位置”区分的完整
溯源路径，unflatten 按路径中的兄弟索引重建原始层级。

## 设计

- `Timeline.Tracks` 是 OTIO 风格的 `Stack` / `Track` / `Sequence` 组合树。
- `Node` 可以是组合节点、`Clip`、`Gap`；保留 `OTIO_SCHEMA`、`name`、`id`、
  `metadata`、`source_range`、`duration` 和 `media_reference`。
- `Flatten` 输出 `FlatDocument`：
  - `items[].path`：从根序列到直接父序列的 `Stamp` 路径。
  - 每个路径帧记录父序列 ID/名称/schema/metadata、实例索引、直接父序列中的
    `local_start` 和该嵌套实例自己的 `source_range`。
  - `items[].source_range` 与 `media_reference` 原样保留，因此片段局部时间和媒体
    引用不依赖重新计算。
  - 空父序列生成 `marker: "empty-composition"` 记录，避免空序列丢失。
- `Unflatten` 不按 ID 合并实例，而是按 `path + index` 挂载节点。相同序列或媒体
  被重复引用时，只要发生路径不同，就会恢复为不同位置的重复实例。
- `FlattenOTIO` / `UnflattenOTIO` 把同一份溯源信息放入 OTIO 标准的
  `metadata["dev.userfeedback.otioflat/v1"]`，使导出的 OTIO-like JSON 再导回时
  仍可恢复层级。

时间使用整数 `RationalTime{Value, Rate}`，跨 rate 求和时用 `math/big.Rat` 保持
精确。`local_start` 是直接父序列坐标；原始 `source_range` 不被改写，所以往返
不依赖浮点或帧率取整。

## 循环结构

`Flatten` 在 DFS 的活动路径上记录 `*Node` 指针。再次遇到当前栈上的节点会返回
可被 `errors.Is(err, otioflat.ErrCycle)` 识别的 `*CycleError`，不会无限展开。
同一个无环节点在不同分支或兄弟位置重复出现是允许的，并会展开成多个发生实例。

## API

```go
flat, err := otioflat.Flatten(timeline)
nested, err := otioflat.Unflatten(flat)

data, err := otioflat.FlattenJSON(timeline)
nested, err := otioflat.UnflattenJSON(data)

otioData, err := otioflat.FlattenOTIOJSON(timeline)
nested, err := otioflat.UnflattenOTIOJSON(otioData)
```

## 测试

```sh
go test ./...
```

`roundtrip_test.go` 对 250 个固定随机种子执行属性往返：

- 根 `Stack` → 外层 `Track/Sequence` → 内层 `Sequence/Track/Stack` →
  `Clip/Gap` 的两层嵌套；
- 随机 `source_range`、gap duration、rate、metadata 和媒体引用；
- 重复媒体引用，并故意把同一个内层序列指针重复放到兄弟位置；
- 分别验证内存直接往返、`FlatDocument` JSON 往返和 OTIO metadata JSON 往返都
  与原树 `reflect.DeepEqual`。

`flatten_test.go` 另外覆盖指针环拒绝、重复 DAG 实例、局部偏移、媒体引用和空
父序列。

## CLI

```sh
go run ./cmd/otioflat flatten examples/nested_timeline.json /tmp/flat.json
go run ./cmd/otioflat unflatten /tmp/flat.json /tmp/roundtrip.json
go run ./cmd/otioflat export-otio examples/nested_timeline.json /tmp/flat.otio.json
go run ./cmd/otioflat import-otio /tmp/flat.otio.json /tmp/nested.json
```

示例产物见 `examples/flat_document.json` 和
`examples/flattened.otio.json`。实现和测试只依赖 Go 标准库。
