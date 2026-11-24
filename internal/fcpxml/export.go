// Package fcpxml exports and re-opens FCPXML (Final Cut Pro XML) files.
package fcpxml

import (
	"fmt"
	"strings"

	"cutcheck/internal/rational"
	"cutcheck/internal/timeline"
)

// ExportConfig controls FCPXML rendering.
type ExportConfig struct {
	// AudioPrimingSamples models an audio codec encoder delay (e.g. AAC
	// priming) baked into the exported offsets without compensation.
	// Must be 0 for a faithful export (PCM or compensated compressed audio).
	AudioPrimingSamples int64
}

// CorrectConfig returns the export config that faithfully represents el.
func CorrectConfig(el *timeline.EditList) ExportConfig {
	return ExportConfig{AudioPrimingSamples: 0}
}

// rat formats exact seconds in FCPXML style: "1001/300s" or "5s".
func rat(r rational.Rat) string {
	if r.Den == 1 {
		return fmt.Sprintf("%ds", r.Num)
	}
	return fmt.Sprintf("%d/%ds", r.Num, r.Den)
}

// Export renders the edit list as FCPXML.
func Export(el *timeline.EditList, cfg ExportConfig) (string, []string) {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<!DOCTYPE fcpxml>\n")
	b.WriteString("<fcpxml version=\"1.8\">\n")
	b.WriteString("  <resources>\n")
	fmt.Fprintf(&b, "    <format id=\"r1\" frameDuration=\"%d/%ds\" width=\"1920\" height=\"1080\"/>\n",
		el.FPSDen, el.FPSNum)
	b.WriteString("  </resources>\n")
	fmt.Fprintf(&b, "  <library>\n    <event name=%q>\n      <project name=%q>\n", el.Title, el.Title)
	b.WriteString("        <sequence format=\"r1\" tcStart=\"0s\" tcFormat=\"DF\">\n")
	b.WriteString("          <spine>\n")

	for _, ev := range el.Video {
		attrs := []string{
			fmt.Sprintf("name=%q", ev.Clip),
			fmt.Sprintf("note=%q", ev.ID),
			fmt.Sprintf("offset=%q", rat(el.FrameSeconds(ev.RecordIn))),
			fmt.Sprintf("duration=%q", rat(el.FrameSeconds(ev.RecordOut-ev.RecordIn))),
			fmt.Sprintf("start=%q", rat(el.FrameSeconds(ev.SourceIn))),
			fmt.Sprintf("srcDuration=%q", rat(el.FrameSeconds(ev.SourceOut-ev.SourceIn))),
			fmt.Sprintf("lane=%q", fmt.Sprintf("%d", ev.Track-1)),
		}
		if ev.IsRetime() {
			attrs = append(attrs, fmt.Sprintf("speed=%q", ev.Speed().String()))
		}
		if ev.Transition == timeline.TransitionDissolve {
			attrs = append(attrs, "transition=\"dissolve\"",
				fmt.Sprintf("transitionDuration=%q", rat(el.FrameSeconds(ev.TransitionDur))))
		}
		fmt.Fprintf(&b, "            <asset-clip %s/>\n", strings.Join(attrs, " "))
	}

	for _, ev := range el.Audio {
		recIn := ev.RecordIn + cfg.AudioPrimingSamples
		attrs := []string{
			fmt.Sprintf("name=%q", ev.Clip),
			fmt.Sprintf("note=%q", ev.ID),
			"role=\"audio\"",
			fmt.Sprintf("offset=%q", rat(el.SampleSeconds(recIn))),
			fmt.Sprintf("duration=%q", rat(el.SampleSeconds(ev.RecordOut-ev.RecordIn))),
			fmt.Sprintf("start=%q", rat(el.SampleSeconds(ev.SourceIn))),
			fmt.Sprintf("lane=%q", fmt.Sprintf("a%d", ev.Track)),
		}
		if ev.FadeIn > 0 {
			attrs = append(attrs, fmt.Sprintf("fadeIn=%q", rat(el.SampleSeconds(ev.FadeIn))))
		}
		if ev.FadeOut > 0 {
			attrs = append(attrs, fmt.Sprintf("fadeOut=%q", rat(el.SampleSeconds(ev.FadeOut))))
		}
		fmt.Fprintf(&b, "            <asset-clip %s/>\n", strings.Join(attrs, " "))
	}

	b.WriteString("          </spine>\n        </sequence>\n")
	b.WriteString("      </project>\n    </event>\n  </library>\n</fcpxml>\n")
	return b.String(), nil
}
