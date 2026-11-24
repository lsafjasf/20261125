# 图形字幕 → 文本轨迁移审计报告（目标格式：srt）

总条数 7：可迁移 0，降级 0，属性丢失 6，未迁移 1。

> 注意：以下为逐条明细，汇总数字不替代逐条核对。

## 逐条总览

| 条带 | 区间 | 结论 | 丢失/降级属性 |
|---|---|---|---|
| 0001 | 00:00:01.000–00:00:04.000 | ❌ 属性丢失 | fillColor(approx), fontSize(none) |
| 0002 | 00:00:05.000–00:00:08.500 | ❌ 属性丢失 | fillColor(approx), fontSize(none), outline(none) |
| 0003 | 00:00:09.000–00:00:12.000 | ❌ 属性丢失 | fillColor(approx), fontSize(none), fade(none) |
| 0004 | 00:00:13.000–00:00:16.000 | ❌ 属性丢失 | fillColor(approx), fontSize(none), align(none), absolutePosition(none), overlap(approx) |
| 0005 | 00:00:13.500–00:00:15.500 | ❌ 属性丢失 | fillColor(approx), fontSize(none), overlap(approx) |
| 0006 | 00:00:17.000–00:00:20.000 | ❌ 属性丢失 | fillColor(approx), fontSize(none), absolutePosition(none), rotation(none) |
| 0007 | 00:00:21.000–00:00:24.000 | ⛔ 未迁移 | text(none) |

## 需人工处理条目（附原图、原因、修复建议）

### 0001 — ❌ 属性丢失

![原图](images/0001.png)

文本：你好，世界

| 属性 | 保真度 | 原因 | 人工修复建议 |
|---|---|---|---|
| fillColor | approx | 条带使用自定义填充色 #FFFFFF（<font color> is a non-standard extension, many players ignore it） | 在目标文件中显式写颜色标签；若播放器忽略，人工抽查实际渲染颜色。 |
| fontSize | none | 条带字号 42px（no font size control） | 在目标格式样式中重建字号；SRT 无此能力，需改用 WebVTT/ASS 或接受播放器默认字号。 |

### 0002 — ❌ 属性丢失

![原图](images/0002.png)

文本：带描边的台词

| 属性 | 保真度 | 原因 | 人工修复建议 |
|---|---|---|---|
| fillColor | approx | 条带使用自定义填充色 #FFFFFF（<font color> is a non-standard extension, many players ignore it） | 在目标文件中显式写颜色标签；若播放器忽略，人工抽查实际渲染颜色。 |
| fontSize | none | 条带字号 42px（no font size control） | 在目标格式样式中重建字号；SRT 无此能力，需改用 WebVTT/ASS 或接受播放器默认字号。 |
| outline | none | 条带带 2.5px 描边(#000000)（no stroke/outline concept） | 描边无法直接表达：WebVTT 可在播放器侧注入 ::cue{text-shadow:...} 模拟细描边并人工核对可读性；需要精确描边请改用 ASS(\bord) 或 TTML(tts:textOutline)。 |

### 0003 — ❌ 属性丢失

![原图](images/0003.png)

文本：淡入淡出的台词

| 属性 | 保真度 | 原因 | 人工修复建议 |
|---|---|---|---|
| fillColor | approx | 条带使用自定义填充色 #FFFFFF（<font color> is a non-standard extension, many players ignore it） | 在目标文件中显式写颜色标签；若播放器忽略，人工抽查实际渲染颜色。 |
| fontSize | none | 条带字号 42px（no font size control） | 在目标格式样式中重建字号；SRT 无此能力，需改用 WebVTT/ASS 或接受播放器默认字号。 |
| fade | none | 淡入 400ms / 淡出 600ms（no transition effects） | 淡入淡出在目标格式中无对应能力：改用 ASS(\fad) 可精确保留；若必须留在该格式，人工确认硬切可接受，或在合成阶段将淡变烘回画面。 |

### 0004 — ❌ 属性丢失

![原图](images/0004.png)

文本：左上角标题

| 属性 | 保真度 | 原因 | 人工修复建议 |
|---|---|---|---|
| fillColor | approx | 条带使用自定义填充色 #FFE080（<font color> is a non-standard extension, many players ignore it） | 在目标文件中显式写颜色标签；若播放器忽略，人工抽查实际渲染颜色。 |
| fontSize | none | 条带字号 54px（no font size control） | 在目标格式样式中重建字号；SRT 无此能力，需改用 WebVTT/ASS 或接受播放器默认字号。 |
| align | none | 条带对齐方式 left（no alignment control） | 改用支持对齐的格式（WebVTT/ASS/TTML），或人工确认默认对齐可接受。 |
| absolutePosition | none | 条带位于 (80,60) 锚点 top-left，非默认底部居中（cues always stack at the default position） | 像素级定位无法精确表达：WebVTT 用 line/position 百分比近似后需人工逐条核对落点；需要精确定位请改用 ASS(\pos) 或 TTML(region)。 |
| overlap | approx | 与同时刻其他条带时间重叠（simultaneous cues stack in player-defined order） | 同一时刻多条字幕会被播放器重排：改用 ASS/TTML 独立定位，或人工调整时序/合并条带避免遮挡。 |

### 0005 — ❌ 属性丢失

![原图](images/0005.png)

文本：同时出现的底部台词

| 属性 | 保真度 | 原因 | 人工修复建议 |
|---|---|---|---|
| fillColor | approx | 条带使用自定义填充色 #FFFFFF（<font color> is a non-standard extension, many players ignore it） | 在目标文件中显式写颜色标签；若播放器忽略，人工抽查实际渲染颜色。 |
| fontSize | none | 条带字号 42px（no font size control） | 在目标格式样式中重建字号；SRT 无此能力，需改用 WebVTT/ASS 或接受播放器默认字号。 |
| overlap | approx | 与同时刻其他条带时间重叠（simultaneous cues stack in player-defined order） | 同一时刻多条字幕会被播放器重排：改用 ASS/TTML 独立定位，或人工调整时序/合并条带避免遮挡。 |

### 0006 — ❌ 属性丢失

![原图](images/0006.png)

文本：竖排旋转文字

| 属性 | 保真度 | 原因 | 人工修复建议 |
|---|---|---|---|
| fillColor | approx | 条带使用自定义填充色 #FFFFFF（<font color> is a non-standard extension, many players ignore it） | 在目标文件中显式写颜色标签；若播放器忽略，人工抽查实际渲染颜色。 |
| fontSize | none | 条带字号 40px（no font size control） | 在目标格式样式中重建字号；SRT 无此能力，需改用 WebVTT/ASS 或接受播放器默认字号。 |
| absolutePosition | none | 条带位于 (1700,300) 锚点 top-right，非默认底部居中（cues always stack at the default position） | 像素级定位无法精确表达：WebVTT 用 line/position 百分比近似后需人工逐条核对落点；需要精确定位请改用 ASS(\pos) 或 TTML(region)。 |
| rotation | none | 条带旋转 90.0°（no rotation） | 旋转条带无法表达：保留原图作为图片字幕轨，或在合成阶段烘回画面；ASS 仅支持绕文本轴旋转(\frz)，需人工确认效果等价。 |

### 0007 — ⛔ 未迁移

![原图](images/0007.png)

| 属性 | 保真度 | 原因 | 人工修复建议 |
|---|---|---|---|
| text | none | 条带无文本内容，无法转换为文本轨 | 条带无文本内容（纯图像）：先人工听写或 OCR 补录文本，再重新审计；在此之前该条只能保留为图像轨。 |
