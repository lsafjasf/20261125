package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const defaultConfigPath = "configs/color_graph.json"

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

// runCLI returns the process exit code. It is separated from main so tests
// can invoke the full command surface without forking.
func runCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return runExec([]string{}, stdout, stderr)
	}
	switch args[0] {
	case "analyze", "plan", "check":
		return runAnalyze(args[1:], stdout, stderr)
	case "run", "exec":
		return runExec(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		// `colorgraph -config x.json` (no subcommand) is treated as analyze.
		if strings.HasPrefix(args[0], "-") {
			return runAnalyze(args, stdout, stderr)
		}
		// Allow `colorgraph path/to/config.json` as a shortcut for analyze.
		if _, err := os.Stat(args[0]); err == nil || filepath.Ext(args[0]) == ".json" {
			return runAnalyze([]string{"-config", args[0]}, stdout, stderr)
		}
		fmt.Fprintf(stderr, "未知子命令: %s\n\n", args[0])
		printUsage(stderr)
		return 1
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `colorgraph — 色彩转换图拓扑/循环/重复输出/未使用节点分析与帧执行

用法:
  colorgraph                         # 等价于 run，使用仓库自带 configs + frames
  colorgraph analyze -config <g.json> [-json]
  colorgraph plan    -config <g.json> [-json]   # analyze 的别名，强调执行前色彩空间说明
  colorgraph run     -config <g.json> [-frames <dir|glob>] [-out <dir>]

退出码: 0 成功；2 图配置不合法（循环/重复输出/悬空引用/色彩空间冲突）；1 其他错误。
仓库自带示例:
  configs/color_graph.json      合法图：含分叉、合并与未使用节点
  configs/diamond.json          最小合法分叉+合并
  configs/cycle.json            真正循环
  configs/duplicate_outputs.json 重复输出声明
  frames/input/                 提供的测试帧`)
}

func runAnalyze(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(stderr)
	defaultPath := locateRepoFile(defaultConfigPath)
	var configPath string
	fs.StringVar(&configPath, "config", defaultPath, "graph config JSON path")
	fs.StringVar(&configPath, "c", defaultPath, "alias of -config")
	fs.StringVar(&configPath, "graph", defaultPath, "alias of -config")
	asJSON := fs.Bool("json", false, "emit structured JSON report")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintln(stderr, "colorgraph:", err)
		return 1
	}
	report := BuildReport(cfg)
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else {
		fmt.Fprint(stdout, report.RenderText())
	}
	if !report.Valid {
		return 2
	}
	return 0
}

func runExec(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	defaultPath := locateRepoFile(defaultConfigPath)
	var configPath string
	fs.StringVar(&configPath, "config", defaultPath, "graph config JSON path")
	fs.StringVar(&configPath, "c", defaultPath, "alias of -config")
	fs.StringVar(&configPath, "graph", defaultPath, "alias of -config")
	framesFlag := fs.String("frames", "", "frame directory or glob (defaults to config frames / frames/input)")
	outDir := fs.String("out", "colorgraph-out", "report and PNG output directory")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintln(stderr, "colorgraph:", err)
		return 1
	}
	report := BuildReport(cfg)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(stderr, "colorgraph: 创建输出目录失败:", err)
		return 1
	}
	_ = writeJSON(filepath.Join(*outDir, "preflight.json"), report)
	if err := os.WriteFile(filepath.Join(*outDir, "preflight.txt"), []byte(report.RenderText()), 0o644); err != nil {
		fmt.Fprintln(stderr, "colorgraph: 写 preflight.txt 失败:", err)
		return 1
	}

	// The color-space explanation is always emitted before touching frames.
	fmt.Fprint(stdout, report.RenderText())
	if !report.Valid {
		fmt.Fprintln(stderr, "colorgraph: 图不合法，已阻止执行；详见", filepath.Join(*outDir, "preflight.txt"))
		return 2
	}

	framesTarget := firstNonEmpty(*framesFlag, cfg.FramesTarget(), locateRepoFile(filepath.Join("frames", "input")))
	framePaths, err := ListFrames(framesTarget)
	if err != nil {
		fmt.Fprintln(stderr, "colorgraph: 枚举测试帧失败:", err)
		return 1
	}
	if len(framePaths) == 0 {
		fmt.Fprintf(stderr, "colorgraph: 在 %q 未找到 .png/.jpg 测试帧\n", framesTarget)
		return 1
	}
	fmt.Fprintf(stdout, "\n开始按拓扑序执行 %d 帧 / executing %d frame(s) from %s\n", len(framePaths), len(framePaths), framesTarget)

	execReport, err := RunFrames(report, framePaths, *outDir)
	if err != nil {
		fmt.Fprintln(stderr, "colorgraph: 执行失败:", err)
		return 1
	}
	if err := writeJSON(filepath.Join(*outDir, "run-report.json"), execReport); err != nil {
		fmt.Fprintln(stderr, "colorgraph: 写 run-report.json 失败:", err)
		return 1
	}
	for _, fr := range execReport.Frames {
		for _, o := range fr.Outputs {
			fmt.Fprintf(stdout, "  帧 %s -> 目标节点 %s: %s (%dx%d, space=%s)\n",
				fr.Frame, o.Node, o.Path, o.Width, o.Height, o.Space)
		}
	}
	fmt.Fprintln(stdout, "完成 / done. 预检与执行报告已写入", *outDir)
	return 0
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
