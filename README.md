# cutcheck — 双成片同源性验证

发布前证明两种格式的成片（CMX3600 EDL 与 FCPXML）来自同一份规范编辑清单
`editlist.json`，定位多帧漂移 / 音频延迟所在的层，修复导出参数后重新打开
两种成片复测。全部逻辑为 Go（仅标准库）。

## 运行

```sh
go run ./cmd/cutcheck      # 完整流程：现状导出 -> 重开诊断 -> 修复参数 -> 重导复测
go test ./...              # 单元 + 端到端测试
```

复测任意两个已存在的成片：

```sh
go run ./cmd/cutcheck verify editlist.json out/fixed/promo.cmx.edl out/fixed/promo.fcpxml
```

退出码：0 = 等价（PASS），1 = 不等价（FAIL），2 = 运行错误。

## 流程

1. **规范编辑清单** `editlist.json`：29.97（30000/1001）掉帧时间线，48 kHz
   音频；视频事件以帧计、音频事件以采样计，全部用有理数精确运算。
2. **跨格式事件映射**：两个导出器在成片中嵌入规范事件 ID（CMX 注释行
   `* EVENT ID:`，FCPXML `note` 属性）；重新打开成片后按 ID 对齐每个事件。
3. **分层诊断**：按（格式 × 层）聚合每个事件的 recordIn/Out 偏差并分类：
   - `cumulative-drift`：偏差随时间线位置线性增长 → 导出帧率错误
     （30/1 当作 30000/1001，片尾多帧漂移）；
   - `constant-offset`：所有事件等量平移 → 音频编码器 priming 未补偿；
   - `scattered` / `match`。
4. **修复导出参数**：CMX 帧率改回 30000/1001 掉帧；FCPXML 音频 priming
   补偿为 0。重新导出到 `out/fixed/` 后**重新打开两种成片复测**。
5. **不可等价项**（显式声明、不计入等价证明）：
   - CMX 无法表达变速（V4 以 1.0x 导出）；
   - CMX 无法表达音频淡入淡出；
   - CMX 音频只能帧级对齐（容差半帧，FCPXML 音频采样级精确）。

## 结构

- `internal/rational` — 精确有理数时间
- `internal/timeline` — 规范编辑清单模型
- `internal/timecode` — SMPTE 掉帧时间码
- `internal/cmx` / `internal/fcpxml` — 两种格式的导出与重新打开
- `internal/compare` — 跨格式映射、分层诊断、等价判定
- `internal/pipeline` — 导出→重开→诊断→复测编排
