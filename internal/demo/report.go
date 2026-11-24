package demo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (d *demo) writeReport(path string) error {
	if dir := filepath.Dir(path); dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	var b strings.Builder
	b.WriteString("# 断点恢复 / 重复执行 对照报告\n\n")
	fmt.Fprintf(&b, "- 生成时间: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "- 二进制: `%s`\n", d.bin)
	fmt.Fprintf(&b, "- 工作区: `%s`\n\n", d.root)

	b.WriteString("## 对照表\n\n")
	b.WriteString("| 对照维度 | 旧逻辑（无状态 / 直接截断写） | 修复后（检查点 / 临时文件 / 完成标记） |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, r := range d.rows {
		fmt.Fprintf(&b, "| %s | %s | %s |\n",
			escapeMD(r.aspect), escapeMD(r.legacy), escapeMD(r.safe))
	}

	b.WriteString("\n## 断言结果\n\n")
	passed := 0
	for _, c := range d.checks {
		status := "PASS"
		if c.ok {
			passed++
		} else {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "- [%s] **%s** — %s\n", status, c.name, escapeMD(c.detail))
	}
	fmt.Fprintf(&b, "\n通过 %d/%d。\n\n", passed, len(d.checks))

	b.WriteString("## 手动复现\n\n")
	b.WriteString("```bash\n")
	b.WriteString("go build -o batchproc .\n")
	b.WriteString("./batchproc demo            # 端到端对照（自动构造崩溃/重启）\n")
	b.WriteString("./batchproc verify --jobs jobs.example.json --out ./out\n")
	b.WriteString("```\n")

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
