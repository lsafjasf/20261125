# AES3 BMC 边沿失锁分析器

仅使用 Go 标准库，从 AES3/AES-EBU 的双相标记（Biphase-Mark Code，BMC）边沿时间戳恢复位时钟，识别：

- `X` / `Z`：左声道（Channel A / Sub-frame 1），`Z` 同时表示 192 位通道状态块开始；
- `Y`：右声道（Channel B / Sub-frame 2）；
- 4 个时隙的特殊前导码及两种线电平极性；
- 时隙 4..27 的 24 位音频主字段（LSB first，二进制补码）；
- 时隙 28 的有效位 `V`（`V=0` 表示音频有效）；
- 时隙 29 的用户位 `U`、时隙 30 的通道状态位 `C`、时隙 31 的偶校验位 `P`；
- 失锁区间、重锁前导，以及按左/右和错误边界隔离的通道状态段。

## 快速开始

```sh
go test ./...
go run ./cmd/genfixture            # 生成 testdata/sample-edges.json 和 loss-edges.json
go run . -input testdata/sample-edges.json -text
go run . -input testdata/loss-edges.json -text
go run . -input capture.json       # JSON 报告到 stdout
```

也可以使用 `go run ./cmd/aes3check ...`。

## 输入格式

时间坐标可以是秒、纳秒、逻辑分析仪采样 tick 等任意稳定单位；时钟恢复会自动估计半位时间 `h = UI/2`。

```json
{
  "edges": [
    {"time": 0, "level": 1},
    {"time": 162.76, "level": 0},
    {"time": 325.52, "level": 1}
  ]
}
```

支持的 JSON 形式：

- 裸时间戳数组：`[0, 0.5, 1.5]`；
- `{"edges": [...]}`、`{"transitions": [...]}`、`{"timestamps": [...]}`；
- `[time, level]` 二元数组；
- `level` 可写 `0/1`、`true/false`、`low/high`、`falling/rising`；
- `{"deltas":[...]}` 或 `intervals` / `edge_intervals`：相邻边沿间隔，自动累加为时间戳；
- `.jsonl`：每行一个边沿对象；
- `.csv`：例如 `time,level` 或 `timestamp,direction`，时间支持 `500us`、`1ms` 等 Go duration。

如果时间戳是采样 tick，可直接按 tick 输出区间；也可用 `-time-scale=1e-8` 之类的乘数换算成秒。

## 解码方法

1. 统计边沿间隔。锁定的 BMC 数据间隔约为 `h` 或 `2h`，AES3 前导还包含 `3h` 间隔。
2. 用候选时钟直方图和 `d/k` 中位数恢复稳健的 `h`，避免单个毛刺被当成位时钟。
3. 从每个边沿尝试匹配标准前导的 8 个半位线状态：
   - `X = 11100010` / `00011101`
   - `Y = 11100100` / `00011011`
   - `Z = 11101000` / `00010111`
4. 前导建立半位网格后，对时隙 4..31 取两个半位中心：电平不同表示中点有跳变，恢复为数据 `1`；相同表示 `0`。
5. 校验每个 BMC 时隙边界、`1` 位中点跳变和多余边沿；结构完整才作为锁定子帧输出。
6. 相邻子帧起点应相距 `64h`。无法连续时，把上一个可信子帧结尾到下一个 `X/Y/Z` 重锁前导之间输出为失锁区间。

## 失锁与错误隔离

`lock_intervals`（同时以 `loss_of_lock_intervals` 别名输出）使用半开边沿区间 `[start_edge,end_edge)`：

- `start_time`：上一个完整子帧结束、期待下一个前导开始的位置；
- `end_time` / `end_edge`：重锁前导的起点/边沿索引；
- `missing_slots`：按 `64h` 估算丢失的子帧数量；
- 区间内没有边沿时，会明确报告“无跳变，直到前导重锁”。

结构完整但偶校验失败的子帧仍会显示原始音频位、`V/U/C/P`，同时标记 `parity_ok=false`、`trusted=false`。它的通道状态位不会并入可信的左/右状态段；失锁区间更会把左右通道状态序列彻底切段。后续有效前导开启新段，因此坏位不会跨重锁拼接，也不会把左声道错配到右声道。

## 程序化 API

```go
report, err := aes3.Decode(edges) // []aes3.Edge
for _, interval := range report.LockIntervals { /* ... */ }
for _, leftSegment := range report.ChannelStatus.Left { /* ... */ }
```

常用选项见 `aes3.Options`：可固定 `HalfUnit`、调整抖动容限、降低结构评分阈值或要求偶校验必须通过。

## 验证

`aes3/decode_test.go` 覆盖干净流、24 位负采样、左右帧配对、整子帧丢失、杂散边沿、抖动重锁和奇偶坏位隔离。运行：

```sh
go test ./...
go vet ./...
```
