// Command batchproc is a single-binary, checkpointed batch renderer.
//
// It fixes the classic "restart renders from scratch and leaves truncated
// files" failure mode with:
//   - per-stage state checkpoints (.batchproc/state/<job>.json),
//   - temp-file + fsync + atomic rename publication,
//   - SHA-256 completion markers (<artifact>.ok),
//   - per-job error isolation,
//   - explicit confirmation before overwriting successful artifacts,
//   - a built-in end-to-end comparison via the "demo" subcommand.
package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage(os.Stdout)
		return 0
	}
	switch args[0] {
	case "run":
		return cmdRun(args[1:])
	case "verify":
		return cmdVerify(args[1:])
	case "demo":
		return cmdDemo(args[1:])
	case "__naive":
		return cmdNaive(args[1:])
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %q\n\n", args[0])
		printUsage(os.Stderr)
		return 1
	}
}

func printUsage(w *os.File) {
	fmt.Fprint(w, `batchproc — 带阶段检查点的可靠批处理器（Go 单二进制）

用法:
  batchproc run      --jobs jobs.json --out ./out [--force] [--yes]
                     [--crash job-id:stage] [--run-id ID] [--chunk-delay 2ms]
  batchproc verify   --jobs jobs.json --out ./out
  batchproc demo     [--dir ./demo-work] [--keep] [--report demo-report.md]
  batchproc __naive  （内置旧逻辑模拟器，供 demo 对照，不建议直接使用）

退出码:
  0  全部成功或全部幂等跳过
  1  用法/IO 等未预期错误，或 verify 发现不完整产物
  2  run 存在任务级失败（其他任务已完成）
  3  --force 覆盖成功产物但未获得显式确认
  137 模拟硬崩溃（仅 --crash 注入时）

示例:
  go build -o batchproc .
  ./batchproc demo
  ./batchproc run --jobs jobs.example.json --out ./out
`)
}
