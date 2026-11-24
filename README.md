# 20261125

Repository initialized for 20261125.

## FFV1 周追加归档检查器

纯 Go 标准库实现（无第三方依赖）的 FFV1 风格每周追加归档：每周一个独立分段，
清单（manifest）记录块状态与帧索引，检查器按块增量核对完整性。

### 磁盘布局

```
<root>/
  index/manifest.json     # 清单：块状态 + 每帧索引（号/偏移/长度/sha256）
  seg/block-0001-<章节>.mkv  # 分段：魔数 + 头部 + 帧序列 + SEAL 封盘尾标
```

分段为自描述二进制格式，帧偏移按文件绝对位置记录；只有带上 `SEAL` 尾标的块
才算封盘。清单通过临时文件 + 原子重命名持久化，每写入一帧即落盘一次。

### 关键行为

- **增量帧索引**：检查/恢复时只重解析并重建受影响块的帧索引，其他块字节不动，
  重算范围通过 `CheckReport.ReindexedBlocks/ReindexedFrames` 暴露。
- **分级报警**：
  - `payload_tampered`：帧载荷哈希不符（篡改不会被静默吸收进索引）。
  - `segment_name_changed`：清单文件名与磁盘不符（按分段内嵌块 id 反查定位）。
  - `chapter_changed`：章节在清单侧或分段头部被改，独立于前两类报警。
  - 另有 `segment_missing`、`block_incomplete`、`unregistered_segment`、
    `frame_index_drift`、`corrupt_segment`。
- **封盘块不可改写**：已封盘块的重复 `Append`、`Resume` 一律拒绝；重复片段按
  载荷 sha256+大小识别（`ErrDuplicateAppend`），拒绝时不产生任何写入。
- **中断恢复**：`Recover` 对账开放块，`Resume` 校验已落盘前缀后补齐剩余帧并封盘。
- **正常改名**：`Rename` 原子更新文件名与清单，帧偏移不变，检查零报警零重算。

### 测试

`go test ./...` 覆盖四类场景（均为 Go 原生 `testing`，无外部工具）：

- 中断：`interrupt_test.go` 崩溃注入、重开进程、恢复、续传封盘。
- 重复追加：`duplicate_test.go` 重复片段拒绝、封盘块写保护、错片续传拒绝。
- 局部篡改：`tamper_test.go` 载荷翻转、带外改名、清单/分段头部章节篡改。
- 正常改名：`rename_test.go` 托管改名零报警、偏移与载荷保持有效。

另含 `reindex_test.go` 验证漂移修复严格限定在单个块。

```
go vet ./...
go test -race ./...
```
