package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"mp4diag"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "verify-fixtures" {
		if err := runVerifyFixtures(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "verify-fixtures:", err)
			os.Exit(1)
		}
		return
	}
	if err := runDiagnose(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mp4diag:", err)
		os.Exit(2)
	}
}

func runDiagnose(args []string) error {
	fs := flag.NewFlagSet("diagnose", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "输出机器可读 JSON")
	reference := fs.String("reference", "", "已知良好参考 MP4，用于 sample 区间哈希比对")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mp4diag [--json] [--reference clean.mp4] <video.mp4>")
	}
	report, err := mp4diag.AnalyzeOptions(mp4diag.Options{Path: fs.Arg(0), Reference: *reference})
	if err != nil {
		return err
	}
	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	printText(report)
	return nil
}

func printText(report *mp4diag.Report) {
	fmt.Printf("文件: %s (%d bytes)\n", report.Path, report.FileSize)
	if report.FirstBadBox != nil {
		fmt.Printf("首个坏 box: %s type=%q offset=%d\n", report.FirstBadBox.Path, report.FirstBadBox.Type, report.FirstBadBox.Offset)
	} else {
		fmt.Println("首个坏 box: 无结构性坏 box（问题可能在样本表或压缩数据）")
	}

	fmt.Println("\n四层结论:")
	for _, layer := range []string{mp4diag.LayerContainer, mp4diag.LayerSampleTable, mp4diag.LayerCompressed, mp4diag.LayerDisplay} {
		summary := report.Layers[layer]
		fmt.Printf("- [%s] %s: %s\n", layerLabel(layer), summary.Status, summary.Summary)
	}

	if len(report.Findings) > 0 {
		fmt.Println("\n证据链:")
		for _, finding := range report.Findings {
			location := ""
			if finding.Box != nil {
				location = fmt.Sprintf(" box=%s@%d", finding.Box.Path, finding.Box.Offset)
				if finding.Box.Entry > 0 {
					location += fmt.Sprintf(" entry=%d", finding.Box.Entry)
				}
			}
			samples := ""
			if len(finding.Samples) > 0 {
				samples = " samples=" + strings.Trim(strings.Join(strings.Fields(fmt.Sprint(finding.Samples)), ","), "[]")
			}
			pixel := ""
			if finding.PixelDamage != "" {
				pixel = " pixel=" + finding.PixelDamage
			}
			fmt.Printf("  %s %s/%s%s%s%s\n", finding.ID, finding.Layer, finding.Code, location, samples, pixel)
			fmt.Printf("      %s\n", finding.Message)
		}
	}

	fmt.Println("\n显示/可解码范围:")
	display := report.Display
	fmt.Printf("  受影响 sample: %v\n", display.AffectedSamples)
	fmt.Printf("  受影响显示帧: %v\n", display.AffectedDisplayFrames)
	fmt.Printf("  干净 sample 区间: %s\n", formatCLISpans(display.CleanSampleRanges))
	fmt.Printf("  干净显示帧区间: %s\n", formatCLISpans(display.CleanDisplayFrameRanges))
	if display.NextSyncSample > 0 {
		fmt.Printf("  下一恢复同步 sample: %d\n", display.NextSyncSample)
	}
	fmt.Printf("  判定置信度: %s\n", display.Confidence)
	if display.Note != "" {
		fmt.Printf("  说明: %s\n", display.Note)
	}

	if report.Reference != nil {
		ref := report.Reference
		fmt.Println("\n参考哈希复核:")
		fmt.Printf("  参考: %s\n", ref.Path)
		fmt.Printf("  像素证据级别: %s\n", ref.PixelDamage)
		fmt.Printf("  变化 sample: %v；缺失 sample: %v；一致 sample: %v\n", ref.ChangedSamples, ref.MissingSamples, ref.IntactSamples)
	}
}

func layerLabel(layer string) string {
	switch layer {
	case mp4diag.LayerContainer:
		return "容器  "
	case mp4diag.LayerSampleTable:
		return "样本表"
	case mp4diag.LayerCompressed:
		return "压缩数"
	case mp4diag.LayerDisplay:
		return "显示  "
	default:
		return layer
	}
}

func formatCLISpans(spans []mp4diag.Span) string {
	if len(spans) == 0 {
		return "无"
	}
	parts := make([]string, 0, len(spans))
	for _, span := range spans {
		if span.First == span.Last {
			parts = append(parts, fmt.Sprintf("%d", span.First))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", span.First, span.Last))
		}
	}
	return strings.Join(parts, ",")
}
