# subaudit — 图形字幕 → 文本轨迁移审计

逐条审计图形字幕条带的每个属性（描边、投影、淡入淡出、像素定位、旋转、重叠等）
在目标文本轨格式中能否表达。只用 Go 标准库。

## 用法

```sh
# 生成演示数据（清单 + 占位原图）
go run ./cmd/subaudit sample -out demo

# 审计：目标格式 srt | webvtt | ass | ttml
go run ./cmd/subaudit audit -manifest demo/strips.json -format webvtt -out out_webvtt
```

## 输入

`strips.json` 清单描述每条图形字幕：时间区间、文本（无文本=纯图像条带）、
原图路径、像素框、锚点、旋转角、填充色、字号、描边、投影、淡入淡出、对齐。

## 输出（写入 -out 目录）

- `report.md` / `report.html`：逐条总览 + 每条未完全迁移条目的明细，
  附原图、逐项丢失原因和人工修复建议；汇总数字不替代逐条核对。
- `images/`：所有需人工处理条带的原图归档。
- `track.vtt` / `track.ass` / `track.srt`：尽力转换的目标轨（仅含已有文本的条带）。

## 结论分级

- `migrated`：全部属性可精确表达
- `degraded`：仅有近似表达（如 WebVTT 用百分比近似像素定位）
- `loss`：至少一项属性完全无法表达（如 WebVTT 的淡入淡出、旋转）
- `not_migrated`：无文本的纯图像条带，需先听写/OCR

## 退出码（防止多数成功掩盖少数丢失）

- 任一条带 `loss` 或 `not_migrated` → 退出码 1
- 加 `--strict` 时，仅有 `degraded` 也返回 1

## 测试

```sh
go test ./...
```
