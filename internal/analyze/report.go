package analyze

import (
	"fmt"
	"strings"
)

// FormatText renders the report as human-readable text.
func FormatText(rep *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "FPS: %d   frames: %d   reels: %d\n", rep.FPS, len(rep.Frames), len(rep.ReelOrder))
	fmt.Fprintf(&b, "reel order: %s\n\n", strings.Join(rep.ReelOrder, " -> "))
	b.WriteString("== Playable ranges ==\n")
	if len(rep.Ranges) == 0 {
		b.WriteString("  (none)\n")
	}
	for i, r := range rep.Ranges {
		first, last := r.Frames[0], r.Frames[len(r.Frames)-1]
		fmt.Fprintf(&b, "  #%d  %d frame(s)  %dx%d  keys %d..%d  reels [%s]\n",
			i+1, len(r.Frames), r.Width, r.Height, first.Key, last.Key, rangeReels(r))
		fmt.Fprintf(&b, "      %s  ->  %s\n", short(first.Header.Path), short(last.Header.Path))
		if first.Header.HasTimeCode && last.Header.HasTimeCode {
			fmt.Fprintf(&b, "      TC %s -> %s\n", first.Header.TimeCode, last.Header.TimeCode)
		}
	}
	b.WriteString("\n== Issues ==\n")
	if len(rep.Issues) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, is := range rep.Issues {
		loc := ""
		if is.Reel != "" {
			loc = " [" + is.Reel + "]"
		}
		fmt.Fprintf(&b, "  %-22s%s %s\n", is.Kind, loc, is.Message)
	}
	b.WriteString("\n== Splice suggestions ==\n")
	for _, s := range rep.Suggestions {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	return b.String()
}

func rangeReels(r Range) string {
	seen := map[string]bool{}
	var out []string
	for _, f := range r.Frames {
		if !seen[f.Reel] {
			seen[f.Reel] = true
			out = append(out, f.Reel)
		}
	}
	return strings.Join(out, ", ")
}
