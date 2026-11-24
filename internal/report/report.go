// Package report renders the per-strip audit report and archives originals.
package report

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"subaudit/internal/audit"
)

// Write renders markdown + html reports into outDir and copies the original
// image of every strip that is not fully migrated into outDir/images.
// It returns the list of strips that need human attention.
func Write(rep *audit.Report, manifestDir, outDir string) ([]audit.Result, error) {
	imgDir := filepath.Join(outDir, "images")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return nil, err
	}
	var flagged []audit.Result
	for _, r := range rep.Results {
		if r.Verdict == audit.Migrated {
			continue
		}
		flagged = append(flagged, r)
		if r.Strip.Image != "" {
			dst := filepath.Join(imgDir, r.Strip.ID+filepath.Ext(r.Strip.Image))
			if err := copyFile(filepath.Join(manifestDir, r.Strip.Image), dst); err != nil {
				return nil, fmt.Errorf("archive image for strip %s: %w", r.Strip.ID, err)
			}
		}
	}
	md := renderMarkdown(rep, flagged)
	if err := os.WriteFile(filepath.Join(outDir, "report.md"), []byte(md), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "report.html"), []byte(renderHTML(rep, flagged)), 0o644); err != nil {
		return nil, err
	}
	return flagged, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func verdictLabel(v audit.Verdict) string {
	switch v {
	case audit.Migrated:
		return "✅ 可迁移"
	case audit.Degraded:
		return "⚠️ 降级迁移"
	case audit.Loss:
		return "❌ 属性丢失"
	default:
		return "⛔ 未迁移"
	}
}

func renderMarkdown(rep *audit.Report, flagged []audit.Result) string {
	var b strings.Builder
	c := rep.Counts()
	fmt.Fprintf(&b, "# 图形字幕 → 文本轨迁移审计报告（目标格式：%s）\n\n", rep.Format)
	fmt.Fprintf(&b, "总条数 %d：可迁移 %d，降级 %d，属性丢失 %d，未迁移 %d。\n\n",
		len(rep.Results), c[audit.Migrated], c[audit.Degraded], c[audit.Loss], c[audit.NotMigrated])
	b.WriteString("> 注意：以下为逐条明细，汇总数字不替代逐条核对。\n\n")

	b.WriteString("## 逐条总览\n\n")
	b.WriteString("| 条带 | 区间 | 结论 | 丢失/降级属性 |\n|---|---|---|---|\n")
	for _, r := range rep.Results {
		props := make([]string, 0, len(r.Findings))
		for _, f := range r.Findings {
			props = append(props, f.Property+"("+f.Level.String()+")")
		}
		fmt.Fprintf(&b, "| %s | %s–%s | %s | %s |\n",
			r.Strip.ID, r.Strip.Start, r.Strip.End, verdictLabel(r.Verdict), strings.Join(props, ", "))
	}

	b.WriteString("\n## 需人工处理条目（附原图、原因、修复建议）\n")
	if len(flagged) == 0 {
		b.WriteString("\n无。\n")
		return b.String()
	}
	for _, r := range flagged {
		fmt.Fprintf(&b, "\n### %s — %s\n\n", r.Strip.ID, verdictLabel(r.Verdict))
		if r.Strip.Image != "" {
			ext := filepath.Ext(r.Strip.Image)
			fmt.Fprintf(&b, "![原图](images/%s%s)\n\n", r.Strip.ID, ext)
		}
		if r.Strip.Text != "" {
			fmt.Fprintf(&b, "文本：%s\n\n", r.Strip.Text)
		}
		b.WriteString("| 属性 | 保真度 | 原因 | 人工修复建议 |\n|---|---|---|---|\n")
		for _, f := range r.Findings {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", f.Property, f.Level, f.Reason, f.Fix)
		}
	}
	return b.String()
}

func renderHTML(rep *audit.Report, flagged []audit.Result) string {
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="zh"><head><meta charset="utf-8">` +
		`<title>迁移审计报告</title><style>` +
		`body{font-family:sans-serif;max-width:1100px;margin:2em auto;padding:0 1em}` +
		`table{border-collapse:collapse;width:100%}td,th{border:1px solid #ccc;padding:4px 8px;text-align:left}` +
		`.loss{background:#fde2e2}.degraded{background:#fff6d6}.not_migrated{background:#e2e2e2}` +
		`img{max-width:420px;border:1px solid #999;display:block;margin:8px 0}` +
		`</style></head><body>`)
	c := rep.Counts()
	fmt.Fprintf(&b, "<h1>图形字幕 → 文本轨迁移审计报告（%s）</h1>", esc(rep.Format))
	fmt.Fprintf(&b, "<p>总条数 %d：可迁移 %d，降级 %d，属性丢失 %d，未迁移 %d。汇总不替代逐条核对。</p>",
		len(rep.Results), c[audit.Migrated], c[audit.Degraded], c[audit.Loss], c[audit.NotMigrated])

	b.WriteString("<h2>逐条总览</h2><table><tr><th>条带</th><th>区间</th><th>结论</th><th>丢失/降级属性</th></tr>")
	for _, r := range rep.Results {
		props := make([]string, 0, len(r.Findings))
		for _, f := range r.Findings {
			props = append(props, esc(f.Property)+"("+f.Level.String()+")")
		}
		fmt.Fprintf(&b, `<tr class="%s"><td>%s</td><td>%s–%s</td><td>%s</td><td>%s</td></tr>`,
			r.Verdict, esc(r.Strip.ID), esc(r.Strip.Start), esc(r.Strip.End),
			verdictLabel(r.Verdict), strings.Join(props, ", "))
	}
	b.WriteString("</table>")

	b.WriteString("<h2>需人工处理条目</h2>")
	if len(flagged) == 0 {
		b.WriteString("<p>无。</p>")
	}
	for _, r := range flagged {
		fmt.Fprintf(&b, `<h3 class="%s">%s — %s</h3>`, r.Verdict, esc(r.Strip.ID), verdictLabel(r.Verdict))
		if r.Strip.Image != "" {
			fmt.Fprintf(&b, `<img src="images/%s%s" alt="strip %s 原图">`,
				esc(r.Strip.ID), esc(filepath.Ext(r.Strip.Image)), esc(r.Strip.ID))
		}
		if r.Strip.Text != "" {
			fmt.Fprintf(&b, "<p>文本：%s</p>", esc(r.Strip.Text))
		}
		b.WriteString("<table><tr><th>属性</th><th>保真度</th><th>原因</th><th>人工修复建议</th></tr>")
		for _, f := range r.Findings {
			fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>",
				esc(f.Property), f.Level, esc(f.Reason), esc(f.Fix))
		}
		b.WriteString("</table>")
	}
	b.WriteString("</body></html>")
	return b.String()
}

// SortFlagged orders flagged strips by ID for stable output.
func SortFlagged(flagged []audit.Result) {
	sort.Slice(flagged, func(i, j int) bool { return flagged[i].Strip.ID < flagged[j].Strip.ID })
}
