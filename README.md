# avsyncdiag

纯 Go 标准库实现的采访片音画同步定位工具。它不把“最终差了多少”当成唯一证据，而是把同一稳定帧在五个阶段的媒体位置逐帧对齐：

1. `capture`：采集位置（传感器/采集卡时间码或采集时间轴）。
2. `pts`：封装/解码后的 PTS。
3. `edit`：剪辑、EDL/NLE 映射后的时间线位置。
4. `render`：合成、渲染或导出队列中的时间线位置。
5. `playback`：显示/发声或播放器管线时钟位置。

## 先说明数据边界

当前仓库没有真实媒体文件，也没有五段埋点 trace，因此无法诚实指定这起事故的真实帧号。工具会从输入数据计算并输出 `first_abnormal_frame`。

内置 demo 按常见采访片 `25 fps`、`15:00.000` 处发生 6 帧（240 ms）持续阶跃复现“口型慢半拍”。其首个异常帧是 **zero-based `22500`**；如果剪辑软件从 1 开始计数，则是 **第 `22501` 帧**。这是可复现样例结论，不是对未提供素材的事实判断。

## 快速运行

```bash
go test ./...
go run . demo
go build -o bin/avsync ./cmd/avsync

# 15 分钟处局部跳帧样例
./bin/avsync demo   # 或 go run . demo

# 另外两种形态
./bin/avsync demo --pattern linear_drift
./bin/avsync demo --pattern fixed_offset

# 分析真实导出的 trace
./bin/avsync analyze \
  --input artifacts/trace.jsonl \
  --fps 25 \
  --positions-out artifacts/positions.csv \
  --report-out artifacts/report.json
```

`--input` 支持 JSONL、JSON 数组/`{"records":[...]}` 和带表头 CSV；`-` 表示标准输入。

## Trace 格式

推荐用宽字段同时记录视频和音频位置，单位为纳秒：

```json
{"stage":"capture","frame":22499,"wall_ns":1795536000000000000,"video_pos_ns":899960000000,"audio_pos_ns":899960000000}
{"stage":"pts","frame":22500,"wall_ns":1795536000040000000,"video_pos_ns":900000000000,"audio_pos_ns":900000000000}
{"stage":"edit","frame":22500,"video_pos_ns":900000000000,"audio_pos_ns":900240000000}
{"stage":"render","frame":22500,"video_pos_ns":900000000000,"audio_pos_ns":900240000000}
{"stage":"playback","frame":22500,"video_pos_ns":900000000000,"audio_pos_ns":900240000000}
```

也支持长格式：

```json
{"stage":"playback","frame":22500,"stream":"video","position_ns":900000000000}
{"stage":"playback","frame":22500,"stream":"audio","position_ns":900240000000}
```

CSV 可使用 `position_ms`、`video_pos_ms`、`audio_pos_ms`，工具会换算成纳秒。完整示例见 `examples/trace.example.jsonl` 和 `examples/positions.example.csv`。

字段约定：

- `frame`：跨阶段稳定的源帧/采集帧编号，默认从 `0` 开始。
- `output_frame`：可选，阶段内部输出帧、编码帧或显示帧号；其跳号会单独统计。
- `wall_ns`：观察该位置的墙钟时间，用于无帧号记录排序。
- `video_pos_ns` / `audio_pos_ns`：该阶段认为对应帧/采样所在的媒体时间轴位置。
- 唇音偏移统一为 `audio_position - video_position`；正值表示音频超前、视频口型偏慢。

## Go 原生埋点

```go
rec := sync.NewRecorder()

rec.ObserveAV(sync.StageCapture, frame, captureVideoPTS, captureAudioPTS)
rec.ObserveAV(sync.StagePTS, frame, packetVideoPTS, packetAudioPTS)
rec.ObserveAV(sync.StageEdit, frame, timelineVideoPos, timelineAudioPos)
rec.ObserveAV(sync.StageRender, frame, renderedVideoPos, renderedAudioPos)
rec.ObserveAV(sync.StagePlayback, frame, displayedVideoPos, audibleAudioPos)

report, err := sync.Analyze(rec.Records(), sync.DefaultOptions())
```

`Recorder` 是并发安全的，可直接放入采集回调、编码器、NLE 导出器、渲染器和播放器时钟回调。分析过程不依赖 cgo 或 FFmpeg。

## 判定方法

工具会生成以下序列，而不是只取最后一个采样点：

- 每个阶段的 `position - expected_frame * frame_duration` 时间线残差。
- `pts - capture`、`edit - pts`、`render - edit`、`playback - render` 阶段映射差。
- `playback - capture` 端到端差。
- 每个阶段的 `audio - video` 唇音偏移。

分类规则：

- **固定偏移 `fixed_offset`**：中位数显著偏离零，但中位绝对偏差很小，且全段斜率不构成预算内漂移。
- **线性漂移 `linear_drift`**：最小二乘斜率稳定、`R² >= 0.70`，且首末漂移量超过 `--drift-budget-ms`。报告首次超过 `--tolerance-ms` 的帧。
- **局部跳帧 `local_frame_skip` / `local_jump`**：某一帧的相邻增量扣除稳健斜率后仍超过 `--jump-ms`，且后续中位数确认偏移持续。若阶跃量接近整数个帧时长，会估算跳/缺帧数。
- **帧号/输出号跳变**：自动估计常规采样步长；当 `frame` 或 `output_frame` 相对该步长额外跳号时，即使 PTS 差值连续，也会记为疑似丢帧。固定抽稀日志可用 `--frame-stride 25` 显式指定步长。
- **不规则 `irregular`**：误差持续变化但既不稳定也不呈可信线性。

首个异常帧取所有阶段和偏移序列中最早的持久异常；同时单独输出：

- `first_root_cause_frame`：最早阶段异常。
- `first_playback_anomaly_frame`：播放/唇音序列上的首个可见异常。
- `evidence_rows`：首帧前后和片头片尾的位置证据。

常用阈值：

- `--fps 25`：默认值；29.97、30、60 fps 素材必须显式设置。
- `--jump-ms 20`：大于普通测量噪声、小于一帧（25 fps 为 40 ms）的一半。
- `--tolerance-ms 2`：固定偏移和首次越界的噪声容限。
- `--drift-budget-ms 80`：超过该累计漂移即认为可能可闻/可见。
- `--frame-stride 0`：`0` 表示自动估计帧号采样步长。

## 15 分钟对应帧号

`15 * 60 * fps`：

- `23.976 fps`：约 zero-based `21578`（需用精确帧率和时基换算）。
- `24 fps`：zero-based `21600`。
- `25 fps`：zero-based `22500`，从 1 计数为 `22501`。
- `29.97 fps`：约 zero-based `26973`。
- `30 fps`：zero-based `27000`。

“半拍”没有统一毫秒值：节拍速度不同可能约为 150–500 ms。请用 `--jump-ms`、`--drift-budget-ms` 和实际 trace 校准，不要仅凭“半拍”反推帧数。

## 输出文件

- 终端文本：结论、根因首帧、播放首帧、阶段汇总、全部序列指标、异常事件和首异常前后位置。
- `--json`：输出完整 JSON 报告。
- `--report-out report.json`：保存 JSON 报告。
- `--positions-out positions.csv`：保存所有帧 × 五个阶段的位置宽表，包含 video/audio/lip offset 列。

## 对真实事故给出结论所需的数据

请从现场导出至少以下之一：

1. 按本格式记录的五阶段 JSONL/CSV；最好包含 `wall_ns`、稳定 `frame`、`video_pos_ns`、`audio_pos_ns`。
2. 播放器统计、渲染器日志、NLE/EDL 映射表和采集/PTS 表；可先转换成上述 schema。
3. 若只能提供成片，可先用 Go 原生或 `ffprobe -show_packets -of json` 导出 PTS 作为 `pts` 阶段，但这不足以还原采集、编辑映射、渲染和播放位置；后四段仍需管线日志。

拿到真实 `artifacts/trace.jsonl` 后，运行上面的 `analyze` 命令，JSON 中的 `first_abnormal_frame.frame` 就是工具判定的首个异常帧。
