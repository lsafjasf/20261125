# 20261125

Repository initialized for 20261125.

## sccparse

SCC (Scenarist Closed Captions) 解析器，按字段解析十六进制字节并重建
每个 caption channel 的 CEA-608 状态。

- 控制码：RU2/RU3/RU4 滚动窗口、PAC 行定位、CR 滚动、EDM/ENM 清屏、EOC 内存翻转
- 区分重复控制码（CEA-608 双发去重）、无效奇偶位（严格模式跳过）、跨帧残留字节
- 未知码保留在事件日志中并继续扫描
- 输出每帧原始码、可见文本，以及严格/朴素两种重建的首个状态分歧

```sh
go run ./cmd/sccparse testdata/sample.scc
go test ./...
```

原始码标记：`!` 奇偶位错误，`^` 从上一帧残留带入，`v` 挂起到下一帧。
