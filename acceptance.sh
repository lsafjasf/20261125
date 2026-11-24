#!/usr/bin/env bash
# 可复测验收：单测 + 构建 + 四类图结构判定 + 在仓库提供的测试帧上执行。
# 用法: bash acceptance.sh
set -euo pipefail

cd "$(dirname "$0")"

echo "==> go vet / go test"
go vet ./...
go test ./...

BIN="$(mktemp -t colorgraph.XXXXXX)"
trap 'rm -f "$BIN"' EXIT
go build -o "$BIN" .

WORK="$(mktemp -d)"
trap 'rm -f "$BIN"; rm -rf "$WORK"' EXIT

echo "==> 合法图：拓扑序 / 分叉 / 合并 / 未使用节点 / 输入色彩空间"
"$BIN" analyze -config configs/color_graph.json > "$WORK/main.txt"
for token in "Topological order" "合法分叉 fork" "合法合并 merge" \
             "未使用节点" "unused" "Effective input color space" \
             "adopted=" "Linear sRGB"; do
  grep -q "$token" "$WORK/main.txt" || { echo "缺少输出标记: $token"; exit 1; }
done

echo "==> 最小菱形图（合法分叉+合并，无未使用节点）"
"$BIN" analyze -config configs/diamond.json > "$WORK/diamond.txt"
grep -q "合法分叉 fork" "$WORK/diamond.txt"
grep -q "合法合并 merge" "$WORK/diamond.txt"

echo "==> 循环图必须以退出码 2 拒绝并给出闭环路径"
set +e
"$BIN" analyze -config configs/cycle.json > "$WORK/cycle.txt" 2>&1
code=$?
set -e
[ "$code" -eq 2 ] || { echo "cycle exit=$code, want 2"; exit 1; }
grep -q "真正循环 cycle" "$WORK/cycle.txt"
grep -q "grade_a -> blur_b -> grade_c -> grade_a" "$WORK/cycle.txt"

echo "==> 重复输出必须以退出码 2 拒绝"
set +e
"$BIN" analyze -config configs/duplicate_outputs.json > "$WORK/dup.txt" 2>&1
code=$?
set -e
[ "$code" -eq 2 ] || { echo "duplicate exit=$code, want 2"; exit 1; }
grep -q "duplicate output" "$WORK/dup.txt"
grep -q "load_camera_a" "$WORK/dup.txt"
grep -q "load_archive_a" "$WORK/dup.txt"

echo "==> 在提供的 3 帧上执行主图（未使用节点跳过）"
"$BIN" run -config configs/color_graph.json -frames frames/input -out "$WORK/out" > "$WORK/run.txt"
[ "$(find "$WORK/out" -name 'frame_*__deliver.png' | wc -l | tr -d ' ')" -eq 3 ]
[ -f "$WORK/out/preflight.txt" ]
[ -f "$WORK/out/preflight.json" ]
[ -f "$WORK/out/run-report.json" ]
grep -q "backup_grade" "$WORK/out/run-report.json"

echo
echo "ACCEPTANCE OK — 拓扑排序、分叉/合并/循环区分、重复输出、未使用节点、"
echo "每个节点的实际输入色彩空间说明与帧执行均符合预期。"
