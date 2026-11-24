# hdrmeta

按显示时刻（PTS）选择动态 HDR 参数的 Go 状态机。它不按包到达顺序或重复的
场景号直接覆盖当前参数，而是只选择激活时间不晚于当前显示时刻的最新代次，
从而避免把下一场景的元数据提前应用到当前帧。

仅依赖 Go 标准库；测试素材全部在 `*_test.go` 中用代码构造。

## 核心规则

- `Metadata.PTS` 是该参数允许上屏的显示时刻；查询 `at` 时只接受
  `PTS <= at` 的元数据。
- 同一 PTS 存在多个修订时，依次按 `UpdatedAt` 和状态机分配的 `Generation.ID`
  选择最新代次。
- `Scene` 只作为审计字段。场景号重复、回绕或复用时不会覆盖另一个时间区间的
  参数。
- 缺帧由 `Frame.Number` 跳号识别，记录 `FrameGap{From,To,Count}`；缺少某一帧
  的元数据时，显示状态保持上一有效代次。
- 每次 `Present` 都追加不可变的 `FrameRecord`，其中包含
  `GenerationID`、上一代次、采用状态和缺帧区间。元数据迟到不会改写历史记录。

状态值：

- `idle`：尚未摄入任何元数据。
- `waiting_for_activation`：元数据已到达，但其 PTS 仍在查询时刻之后。
- `active`：已找到可在当前显示时刻使用的参数代次。

采用方式：

- `new_generation`：本帧切换到不同的有效代次。
- `held_generation`：继续采用上一帧的代次，可跨越缺帧或元数据缺块。
- `none`：当前 PTS 之前还没有可激活的元数据。

## 使用示例

```go
package main

import (
    "fmt"
    "hdrmeta"
)

func main() {
    m := hdrmeta.NewMachine()

    m.Ingest(hdrmeta.Metadata[hdrmeta.Params]{
        Scene: 10, FrameNumber: 0, PTS: 0, UpdatedAt: 100,
        Parameters: hdrmeta.Params{Label: "dark", PeakNits: 120},
    })
    m.Ingest(hdrmeta.Metadata[hdrmeta.Params]{
        Scene: 20, FrameNumber: 2, PTS: 6000, UpdatedAt: 102,
        Parameters: hdrmeta.Params{Label: "bright", PeakNits: 1000},
    })

    for n := int64(0); n <= 2; n++ {
        record := m.Present(hdrmeta.Frame{
            Number: n,
            PTS:    hdrmeta.Tick(n) * 3000, // 30 fps，90 kHz PTS
        })
        fmt.Printf("frame=%d generation=%d adoption=%s\n",
            n, record.GenerationID, record.Adoption)
    }
    // frame=0 generation=1 adoption=new_generation
    // frame=1 generation=1 adoption=held_generation
    // frame=2 generation=2 adoption=new_generation
}
```

需要携带业务自定义参数时使用泛型：

```go
type MyHDRParams struct {
    Payload []byte
    Version string
}

m := hdrmeta.New[MyHDRParams]()
```

## 时间单位

`Tick` 是整数时间戳。`PTSRate = 90000` 对应 MPEG-TS/MP4 常见的 90 kHz PTS
时钟；也可以继续使用调用方自洽的毫秒或帧 tick。`TickFromDuration` 和
`Tick.Duration()` 提供标准库 `time.Duration` 换算。

## 验证

```sh
gofmt -w .
go test ./... -count=1
go test -race ./...
go vet ./...
```

测试覆盖：

- 下一场景元数据已到达但 PTS 未到时不得提前采用。
- 缺帧号、缺元数据块和跨缺口保持上一代次。
- 重复场景号产生并保留独立参数代次。
- 乱序摄入后仍按显示 PTS 建立索引。
- 连续 `UpdatedAt` 的顺序与每帧实际采用代次可审计。
- 完全相同更新的幂等重放，以及同键修正的新代次。
- 元数据迟到时只影响后续显示，不重写已发生帧记录。
