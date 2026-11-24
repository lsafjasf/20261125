// Command mp4repair rebuilds MP4/MOV sample indices without touching media
// payload bytes. See README.md for the three corruption validation groups.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mp4index/repair"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mp4repair", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		input    = fs.String("in", "", "damaged MP4/MOV file to scan and repair (required)")
		input2   = fs.String("i", "", "alias for -in")
		output   = fs.String("out", "", "repaired output path (default: <input>.repaired.mp4); never overwrites input")
		output2  = fs.String("o", "", "alias for -out")
		ref      = fs.String("ref", "", "optional healthy reference MP4 from the same recorder (for missing moov/stsd)")
		fps      = fs.Float64("fps", 30, "fallback frame rate when timing tables are unusable")
		dedupe   = fs.Bool("dedupe-frames", false, "exclude consecutive byte-identical samples from rebuilt index")
		asJSON   = fs.Bool("json", false, "emit machine readable JSON report")
		inspect  = fs.Bool("inspect", false, "scan and print report only; do not write output")
		repFile  = fs.String("report-file", "", "also write the JSON report to this path")
		moovLast = fs.Bool("moov-last", false, "write moov after mdat (default faststart moov-first)")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	in := firstNonEmpty(*input, *input2)
	out := firstNonEmpty(*output, *output2)
	if in == "" && fs.NArg() > 0 {
		in = fs.Arg(0)
	}
	if in == "" {
		fmt.Fprintln(stderr, "usage: mp4repair -in damaged.mp4 [-out repaired.mp4] [--ref healthy.mp4] [--json]")
		fs.PrintDefaults()
		return 2
	}
	if out == "" && !*inspect {
		out = defaultOutputName(in)
	}

	opts := repair.Options{
		Output:       out,
		Reference:    *ref,
		FPS:          *fps,
		DedupeFrames: *dedupe,
		MoovLast:     *moovLast,
	}

	var rep *repair.Report
	var err error
	if *inspect {
		var plan *repair.Plan
		plan, err = repair.Analyze(in, opts)
		if err == nil {
			rep = plan.Report
		}
	} else {
		rep, err = repair.RepairFile(in, out, opts)
	}
	if err != nil {
		fmt.Fprintf(stderr, "mp4repair: %v\n", err)
		return 1
	}

	if *repFile != "" {
		if werr := writeReportFile(*repFile, rep); werr != nil {
			fmt.Fprintf(stderr, "mp4repair: write report file: %v\n", werr)
			return 1
		}
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
		return 0
	}
	printHumanReport(stdout, rep, *inspect)
	return 0
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func defaultOutputName(in string) string {
	ext := filepath.Ext(in)
	base := strings.TrimSuffix(in, ext)
	if ext == "" {
		ext = ".mp4"
	}
	return base + ".repaired" + ext
}

func writeReportFile(path string, rep *repair.Report) error {
	buf, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func printHumanReport(w io.Writer, r *repair.Report, inspectOnly bool) {
	fmt.Fprintf(w, "文件: %s (%d 字节)\n", r.Input, r.FileSize)
	if r.Output != "" {
		fmt.Fprintf(w, "输出: %s\n", r.Output)
	}
	fmt.Fprintf(w, "扫描到完整 sample: %d，重建索引 sample: %d，关键帧: %d 个 %v\n",
		r.ScannedSamples, r.IndexedSamples, len(r.Keyframes), r.Keyframes)

	d := r.Damage
	issues := []string{}
	if d.MissingMoov {
		issues = append(issues, "moov 缺失")
	}
	if d.Truncated {
		issues = append(issues, "存在截断")
	}
	if d.IndexDuplicateOffsets > 0 {
		issues = append(issues, fmt.Sprintf("重复偏移记录 %d 条", d.IndexDuplicateOffsets))
	}
	if d.IndexDuplicateRecords > 0 {
		issues = append(issues, fmt.Sprintf("重复 sample 记录 %d 条", d.IndexDuplicateRecords))
	}
	if d.BadOffsets > 0 {
		issues = append(issues, fmt.Sprintf("错误偏移 %d 处", d.BadOffsets))
	}
	if d.StreamDuplicateSamples > 0 {
		issues = append(issues, fmt.Sprintf("码流内连续重复 sample %d 个", d.StreamDuplicateSamples))
	}
	if len(issues) == 0 {
		issues = append(issues, "未发现索引损伤")
	}
	fmt.Fprintf(w, "诊断: %s\n", strings.Join(issues, "；"))

	fmt.Fprintln(w, "可播放区间:")
	if len(r.Intervals) == 0 {
		fmt.Fprintln(w, "  (无 — 没有找到以关键帧起始的连续区间)")
	}
	for _, iv := range r.Intervals {
		fmt.Fprintf(w,
			"  #%d sample %d-%d (关键帧起 #%d, 前导非关键帧 %d), 时间 %.3f-%.3fs, 源字节 %d-%d, keys=%v\n",
			iv.Index, iv.FirstSample, iv.LastSample, iv.FirstSyncSample,
			iv.LeadingNonSyncSamples, iv.StartTimeSeconds, iv.EndTimeSeconds,
			iv.StartOffset, iv.EndOffset, iv.Keyframes)
	}

	fmt.Fprintln(w, "未恢复部分:")
	if len(r.Unrecovered) == 0 {
		fmt.Fprintln(w, "  (无)")
	}
	for _, u := range r.Unrecovered {
		if u.Length > 0 {
			fmt.Fprintf(w, "  [%s] 字节 %d-%d (%d 字节): %s\n",
				u.Kind, u.StartOffset, u.EndOffset, u.Length, u.Detail)
		} else if u.MissingSamples > 0 {
			fmt.Fprintf(w, "  [%s] 缺失 sample 槽位 %d 个: %s\n",
				u.Kind, u.MissingSamples, u.Detail)
		} else {
			fmt.Fprintf(w, "  [%s]: %s\n", u.Kind, u.Detail)
		}
	}

	if inspectOnly {
		fmt.Fprintln(w, "仅检查模式，未写入任何文件。")
	} else if r.WroteOutput {
		fmt.Fprintf(w, "修复完成，媒体载荷原样复制 %d 字节；原文件未被修改。\n", r.PayloadBytes)
	}
}
