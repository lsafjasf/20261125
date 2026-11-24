package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteOutputs(rep *Report, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(outDir, "report.json"), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "report.md"), []byte(markdown(rep)), 0o644); err != nil {
		return err
	}
	return nil
}

func markdown(rep *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# cutmux 双容器剪辑点审计\n\n")
	fmt.Fprintf(&b, "**结论：%s** — %s\n\n", rep.Verdict.Status, rep.Verdict.Summary)
	fmt.Fprintf(&b, "- A：`%s`（%s）\n", rep.ContainerA.Path, rep.ContainerA.Format)
	fmt.Fprintf(&b, "- B：`%s`（%s）\n", rep.ContainerB.Path, rep.ContainerB.Format)
	fmt.Fprintf(&b, "- 生成：%s；工具：cutmux %s\n\n", rep.GeneratedUTC, rep.Version)

	b.WriteString("## 切点与总时长\n\n")
	b.WriteString("| 轨 | A 起点 ms | B 起点 ms | A-B ms | A 终点 ms | B 终点 ms |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|\n")
	writeTimelineRow(&b, "video", rep.Video.Timeline)
	writeTimelineRow(&b, "audio", rep.Audio.Timeline)
	writeTimelineRow(&b, "subtitle", rep.Subtitle.Timeline)
	fmt.Fprintf(&b, "\n| 容器 | declared ms | measured end ms |\n")
	fmt.Fprintf(&b, "|---|---:|---:|\n")
	fmt.Fprintf(&b, "| A | %.3f | %.3f |\n", rep.TotalDuration.A.DeclaredMs, rep.TotalDuration.A.MeasuredMs)
	fmt.Fprintf(&b, "| B | %.3f | %.3f |\n", rep.TotalDuration.B.DeclaredMs, rep.TotalDuration.B.MeasuredMs)
	fmt.Fprintf(&b, "| A-B |  | %+.3f |\n\n", rep.TotalDuration.Delta)

	b.WriteString("## 视频\n\n")
	fmt.Fprintf(&b, "- 帧率：A=`%s`，B=`%s`，match=%t\n", rep.Video.FPSA, rep.Video.FPSB, rep.Video.FPSMatch)
	fmt.Fprintf(&b, "- 帧数（raw/presented）：A=%d/%d，B=%d/%d\n",
		rep.Video.RawFramesA, rep.Video.PresentedA, rep.Video.RawFramesB, rep.Video.PresentedB)
	fmt.Fprintf(&b, "- 编码包对齐：comparable=%t common=%d onlyA=%d onlyB=%d",
		rep.Video.Identity.Comparable, rep.Video.Identity.Common,
		rep.Video.Identity.OnlyA, rep.Video.Identity.OnlyB)
	if rep.Video.Identity.Reason != "" {
		fmt.Fprintf(&b, "（%s）", rep.Video.Identity.Reason)
	}
	fmt.Fprintf(&b, "\n- 分类：`%s` — %s\n\n", rep.Video.Classification, rep.Video.Explanation)
	writeFrameRuns(&b, "A 独有", rep.Video.OnlyA)
	writeFrameRuns(&b, "B 独有", rep.Video.OnlyB)

	b.WriteString("## 音频\n\n")
	fmt.Fprintf(&b, "- 包数：A=%d，B=%d；presented 时长 B-A=`%+.3f ms`\n",
		rep.Audio.PacketsA, rep.Audio.PacketsB, rep.Audio.DurationDeltaMs)
	fmt.Fprintf(&b, "- 折算采样帧：A=%d，B=%d，B-A=`%+d`\n",
		rep.Audio.SampleFramesA, rep.Audio.SampleFramesB, rep.Audio.SampleFrameDelta)
	fmt.Fprintf(&b, "- head trim：A=%.3f ms，B=%.3f ms；tail trim：A=%.3f ms，B=%.3f ms\n",
		rep.Audio.HeadTrimA, rep.Audio.HeadTrimB, rep.Audio.TailTrimA, rep.Audio.TailTrimB)
	fmt.Fprintf(&b, "- CodecDelay：A=%.3f ms，B=%.3f ms\n", rep.Audio.CodecDelayA, rep.Audio.CodecDelayB)
	fmt.Fprintf(&b, "- 编码包对齐：comparable=%t common=%d onlyA=%d onlyB=%d\n",
		rep.Audio.Identity.Comparable, rep.Audio.Identity.Common,
		rep.Audio.Identity.OnlyA, rep.Audio.Identity.OnlyB)
	fmt.Fprintf(&b, "- 分类：`%s` — %s\n\n", rep.Audio.Classification, rep.Audio.Explanation)

	b.WriteString("## 字幕与黑帧\n\n")
	fmt.Fprintf(&b, "- 字幕：`%s` — %s\n", rep.Subtitle.Classification, rep.Subtitle.Explanation)
	fmt.Fprintf(&b, "- 黑场证据 A：source=`%s` definitive=%t matched=%d/%d\n",
		rep.BlackFrames.A.Source, rep.BlackFrames.A.Definitive, rep.BlackFrames.A.Matched, rep.BlackFrames.A.Total)
	fmt.Fprintf(&b, "- 黑场证据 B：source=`%s` definitive=%t matched=%d/%d\n",
		rep.BlackFrames.B.Source, rep.BlackFrames.B.Definitive, rep.BlackFrames.B.Matched, rep.BlackFrames.B.Total)
	fmt.Fprintf(&b, "- 黑帧分类：`%s` — %s\n\n", rep.BlackFrames.Classification, rep.BlackFrames.Explanation)
	writeBlackRuns(&b, "A", rep.BlackFrames.A.Runs)
	writeBlackRuns(&b, "B", rep.BlackFrames.B.Runs)
	if len(rep.BlackFrames.OnlyA)+len(rep.BlackFrames.OnlyB) > 0 {
		b.WriteString("\n")
	}

	b.WriteString("## 逐轨时间线\n\n")
	writeTrackTable(&b, "A", rep.ContainerA)
	b.WriteString("\n")
	writeTrackTable(&b, "B", rep.ContainerB)

	b.WriteString("\n## 发现\n\n")
	for _, f := range rep.Findings {
		fmt.Fprintf(&b, "- **%s** `%s`：%s\n", f.Severity, f.Code, f.Message)
	}
	fmt.Fprintf(&b, "\n## 下一步\n\n%s\n", rep.NextCheck)
	return b.String()
}

func writeBlackRuns(b *strings.Builder, label string, runs []BlackRun) {
	for _, r := range runs {
		fmt.Fprintf(b, "- 黑场 %s：%.3f–%.3f ms，%d 帧\n", label, r.StartMs, r.EndMs, r.Frames)
	}
}

func writeTimelineRow(b *strings.Builder, name string, p TimelinePair) {
	fmt.Fprintf(b, "| %s | %.3f | %.3f | %+.3f | %.3f | %.3f |\n",
		name, p.A.StartMs, p.B.StartMs, p.Delta, p.A.EndMs, p.B.EndMs)
}

func writeFrameRuns(b *strings.Builder, title string, runs []FrameRun) {
	for _, r := range runs {
		fmt.Fprintf(b, "- %s：%d 帧，%.3f–%.3f ms", title, r.Frames, r.StartMs, r.EndMs)
		if len(r.Hashes) > 0 {
			fmt.Fprintf(b, "，hash[0]=`%s`", r.Hashes[0])
		}
		b.WriteString("\n")
	}
}

func writeTrackTable(b *strings.Builder, label string, c ContainerSummary) {
	fmt.Fprintf(b, "### %s\n\n", label)
	b.WriteString("| id | kind | codec | samples | raw ms | presented ms | head trim | tail trim | delay | fps | edits |\n")
	b.WriteString("|---:|---|---|---:|---:|---:|---:|---:|---:|---|---:|\n")
	for _, t := range c.Tracks {
		fmt.Fprintf(b, "| %d | %s | `%s` | %d | %.3f–%.3f | %.3f–%.3f | %.3f | %.3f | %.3f | %s | %d |\n",
			t.ID, t.Kind, t.Codec, t.Samples, t.RawStartMs, t.RawEndMs,
			t.PresentedStartMs, t.PresentedEndMs, t.HeadTrimMs, t.TailTrimMs,
			t.CodecDelayMs, t.FrameRate, len(t.Edits))
	}
}
