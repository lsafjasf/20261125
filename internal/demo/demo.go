package demo

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Options controls the comparison demo.
type Options struct {
	// BinaryPath is the executable re-run for every scenario. Empty means
	// the currently running executable (the single batchproc binary).
	BinaryPath string
	// RootDir hosts all scenario workspaces. Empty means a temp dir.
	RootDir string
	// Keep preserves RootDir after the demo.
	Keep bool
	// ReportPath receives a Markdown comparison report. Empty means
	// <RootDir>/demo-report.md.
	ReportPath string
	Out        io.Writer
}

type check struct {
	name   string
	ok     bool
	detail string
}

type tableRow struct {
	aspect string
	legacy string
	safe   string
}

type demo struct {
	opt    Options
	bin    string
	root   string
	out    io.Writer
	checks []check
	rows   []tableRow
}

// Run executes all comparison scenarios and returns an error if any
// asserted reliability property fails.
func Run(opt Options) error {
	if opt.Out == nil {
		opt.Out = os.Stdout
	}
	bin := opt.BinaryPath
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		bin = exe
	}
	root := opt.RootDir
	if root == "" {
		dir, err := os.MkdirTemp("", "batchproc-demo-")
		if err != nil {
			return err
		}
		root = dir
	} else {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return err
		}
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}

	d := &demo{opt: opt, bin: bin, root: root, out: opt.Out}
	fmt.Fprintf(d.out, "批处理器可靠性对照演示\n二进制: %s\n工作区: %s\n\n", bin, root)

	var scenarioErrs []error
	for _, fn := range []func() error{
		d.scenarioNaive,
		d.scenarioResume,
		d.scenarioIsolation,
		d.scenarioOverwrite,
	} {
		if err := fn(); err != nil {
			scenarioErrs = append(scenarioErrs, err)
		}
	}

	d.renderTable()
	d.renderChecks()

	reportPath := opt.ReportPath
	if reportPath == "" {
		reportPath = "DEMO-REPORT.md"
	}
	if err := d.writeReport(reportPath); err != nil {
		fmt.Fprintf(d.out, "（写报告失败: %v）\n", err)
	} else {
		fmt.Fprintf(d.out, "对照报告已写入: %s\n", reportPath)
	}

	if !opt.Keep && opt.RootDir == "" {
		_ = os.RemoveAll(root)
	} else {
		fmt.Fprintf(d.out, "工作区已保留: %s\n", root)
	}

	if len(scenarioErrs) > 0 {
		return errors.Join(scenarioErrs...)
	}
	for _, c := range d.checks {
		if !c.ok {
			return fmt.Errorf("对照断言失败: %s (%s)", c.name, c.detail)
		}
	}
	fmt.Fprintln(d.out, "\n全部对照断言通过 ✅")
	return nil
}

func (d *demo) checkf(name string, ok bool, format string, args ...any) {
	detail := fmt.Sprintf(format, args...)
	d.checks = append(d.checks, check{name: name, ok: ok, detail: detail})
	mark := "✅"
	if !ok {
		mark = "❌"
	}
	fmt.Fprintf(d.out, "  %s 断言: %s — %s\n", mark, name, detail)
}

func (d *demo) section(title string) {
	fmt.Fprintf(d.out, "\n=== %s ===\n", title)
}

// resetScenario removes prior scenario state so repeated `demo --dir X`
// invocations stay deterministic. Only known scenario directories are removed.
func (d *demo) resetScenario(name string) string {
	dir := filepath.Join(d.root, name)
	_ = os.RemoveAll(dir)
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func (d *demo) run(args ...string) procResult {
	pretty := strings.Join(args, " ")
	fmt.Fprintf(d.out, "$ batchproc %s\n", pretty)
	res := runBinary(d.bin, "", "", args...)
	if res.output != "" {
		fmt.Fprint(d.out, indentOutput(res.output, "  │ "))
	}
	fmt.Fprintf(d.out, "  ↳ 退出码 %d\n", res.code)
	return res
}

func (d *demo) runWithStdin(stdin string, args ...string) procResult {
	pretty := strings.Join(args, " ")
	fmt.Fprintf(d.out, "$ echo %q | batchproc %s\n", strings.TrimRight(stdin, "\n"), pretty)
	res := runBinary(d.bin, "", stdin, args...)
	if res.output != "" {
		fmt.Fprint(d.out, indentOutput(res.output, "  │ "))
	}
	fmt.Fprintf(d.out, "  ↳ 退出码 %d\n", res.code)
	return res
}

// cjkWidth estimates terminal display width: wide CJK/fullwidth runes count 2.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if isWideRune(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func padWidth(s string, width int) string {
	missing := width - displayWidth(s)
	if missing <= 0 {
		return s
	}
	return s + strings.Repeat(" ", missing)
}

func (d *demo) renderTable() {
	if len(d.rows) == 0 {
		return
	}
	const (
		w0 = 26
		w1 = 36
		w2 = 44
	)
	top := "┌" + tableDashes(w0) + "┬" + tableDashes(w1) + "┬" + tableDashes(w2) + "┐"
	mid := "├" + strings.Repeat("─", w0+2) + "┼" + strings.Repeat("─", w1+2) + "┼" + strings.Repeat("─", w2+2) + "┤"
	bottom := "└" + strings.Repeat("─", w0+2) + "┴" + strings.Repeat("─", w1+2) + "┴" + strings.Repeat("─", w2+2) + "┘"

	fmt.Fprintln(d.out, "\n================ 断点恢复 vs 从头重跑：对照 ================")
	fmt.Fprintln(d.out, top)
	d.printRow(
		wrapCell("对照维度", w0),
		wrapCell("旧逻辑（无状态/直接截断写）", w1),
		wrapCell("修复后（检查点/临时文件/标记）", w2),
		w0, w1, w2)
	fmt.Fprintln(d.out, mid)
	for _, r := range d.rows {
		d.printRow(wrapCell(r.aspect, w0), wrapCell(r.legacy, w1), wrapCell(r.safe, w2), w0, w1, w2)
	}
	fmt.Fprintln(d.out, bottom)
}

func tableDashes(width int) string {
	return strings.Repeat("─", width+2)
}

func (d *demo) printRow(c0, c1, c2 []string, w0, w1, w2 int) {
	lines := len(c0)
	if len(c1) > lines {
		lines = len(c1)
	}
	if len(c2) > lines {
		lines = len(c2)
	}
	for i := 0; i < lines; i++ {
		fmt.Fprintln(d.out, "│ "+padWidth(cellAt(c0, i), w0)+" │ "+
			padWidth(cellAt(c1, i), w1)+" │ "+padWidth(cellAt(c2, i), w2)+" │")
	}
}

func cellAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

// wrapCell wraps s to lines whose terminal display width is at most w.
func wrapCell(s string, w int) []string {
	var out []string
	for _, hard := range strings.Split(s, "\n") {
		var line strings.Builder
		width := 0
		for _, r := range hard {
			rw := 1
			if isWideRune(r) {
				rw = 2
			}
			if width+rw > w && line.Len() > 0 {
				out = append(out, line.String())
				line.Reset()
				width = 0
			}
			line.WriteRune(r)
			width += rw
		}
		out = append(out, line.String())
	}
	return out
}

func isWideRune(r rune) bool {
	return r >= 0x1100 && (r <= 0x115F ||
		r == 0x2329 || r == 0x232A ||
		(r >= 0x2E80 && r <= 0xA4CF) ||
		(r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE30 && r <= 0xFE4F) ||
		(r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x20000 && r <= 0x2FFFD) ||
		(r >= 0x30000 && r <= 0x3FFFD))
}

func (d *demo) renderChecks() {
	fmt.Fprintln(d.out, "\n---------------- 断言汇总 ----------------")
	passed := 0
	for _, c := range d.checks {
		mark := "PASS"
		if !c.ok {
			mark = "FAIL"
		} else {
			passed++
		}
		fmt.Fprintf(d.out, "[%s] %s — %s\n", mark, c.name, c.detail)
	}
	fmt.Fprintf(d.out, "通过 %d/%d\n", passed, len(d.checks))
}
