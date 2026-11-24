// subaudit audits graphic subtitle strips for migration to a text track.
//
// Usage:
//
//	subaudit sample -out demo
//	subaudit audit -manifest demo/strips.json -format webvtt -out audit_out
//
// Exit code is 1 when any strip suffers property loss or cannot be migrated,
// so a majority of successes can never silently mask minority losses.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"subaudit/internal/audit"
	"subaudit/internal/caps"
	"subaudit/internal/emit"
	"subaudit/internal/report"
	"subaudit/internal/sample"
	"subaudit/internal/strip"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(64)
	}
	var code int
	switch os.Args[1] {
	case "sample":
		code = cmdSample(os.Args[2:])
	case "audit":
		code = cmdAudit(os.Args[2:])
	default:
		usage()
		code = 64
	}
	os.Exit(code)
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  subaudit sample -out DIR
  subaudit audit -manifest strips.json -format srt|webvtt|ass|ttml -out DIR [--emit] [--strict]`)
}

func cmdSample(args []string) int {
	fs := flag.NewFlagSet("sample", flag.ExitOnError)
	out := fs.String("out", "demo", "output directory")
	fs.Parse(args)
	if err := sample.Write(*out); err != nil {
		fmt.Fprintln(os.Stderr, "sample:", err)
		return 1
	}
	fmt.Println("样例已生成:", filepath.Join(*out, "strips.json"))
	return 0
}

func cmdAudit(args []string) int {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	manifest := fs.String("manifest", "", "strip manifest JSON")
	format := fs.String("format", "webvtt", "target format: srt|webvtt|ass|ttml")
	out := fs.String("out", "audit_out", "output directory")
	emitTrack := fs.Bool("emit", true, "also emit the best-effort migrated track")
	strict := fs.Bool("strict", false, "exit 1 also on degraded (approx) strips")
	fs.Parse(args)

	profile, ok := caps.Profiles()[*format]
	if !ok {
		fmt.Fprintln(os.Stderr, "unknown format:", *format)
		return 64
	}
	m, dir, err := strip.Load(*manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		return 1
	}
	rep := audit.Run(m, profile)
	flagged, err := report.Write(rep, dir, *out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "report:", err)
		return 1
	}
	if *emitTrack {
		if err := writeTrack(*format, m, *out); err != nil {
			fmt.Fprintln(os.Stderr, "emit:", err)
			return 1
		}
	}
	c := rep.Counts()
	fmt.Printf("目标格式 %s：共 %d 条，可迁移 %d，降级 %d，丢失 %d，未迁移 %d\n",
		*format, len(rep.Results), c[audit.Migrated], c[audit.Degraded],
		c[audit.Loss], c[audit.NotMigrated])
	for _, r := range flagged {
		fmt.Printf("  需人工处理: %s (%s) — %d 项属性问题，详见 report.md\n",
			r.Strip.ID, r.Verdict, len(r.Findings))
	}
	fmt.Println("报告:", filepath.Join(*out, "report.md"))
	if c[audit.Loss] > 0 || c[audit.NotMigrated] > 0 {
		return 1
	}
	if *strict && c[audit.Degraded] > 0 {
		return 1
	}
	return 0
}

func writeTrack(format string, m *strip.Manifest, out string) error {
	var name, body string
	switch format {
	case "webvtt":
		name, body = "track.vtt", emit.WebVTT(m)
	case "ass":
		name, body = "track.ass", emit.ASS(m)
	case "srt":
		name, body = "track.srt", emit.SRT(m)
	default:
		return nil // ttml emitter not implemented; audit still runs
	}
	return os.WriteFile(filepath.Join(out, name), []byte(body), 0o644)
}
