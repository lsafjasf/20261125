// Package emit writes the best-effort migrated text track for strips that
// have text. Properties the target cannot express are simply omitted here;
// the audit report is the authoritative record of what was lost.
package emit

import (
	"fmt"
	"strings"
	"time"

	"subaudit/internal/strip"
)

func vttTime(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

func assTime(d time.Duration) string {
	cs := d.Milliseconds() / 10
	return fmt.Sprintf("%d:%02d:%02d.%02d", cs/360000, cs/6000%60, cs/100%60, cs%100)
}

func srtTime(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

func assColor(hex string) string {
	// #RRGGBB -> ASS &H00BBGGRR
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) < 6 {
		return "&H00FFFFFF"
	}
	return fmt.Sprintf("&H00%s%s%s", hex[4:6], hex[2:4], hex[0:2])
}

// WebVTT renders a WebVTT track; position is approximated with percentages.
func WebVTT(m *strip.Manifest) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n")
	for _, s := range m.Strips {
		if s.Text == "" {
			continue
		}
		start, end, err := s.StartEnd()
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "\n%s --> %s", vttTime(start), vttTime(end))
		if s.Anchor != "" && s.Anchor != "bottom-center" {
			line := 100 * s.Box.Y / m.Video.Height
			pos := 100 * (s.Box.X + s.Box.W/2) / m.Video.Width
			align := s.Align
			if align == "" {
				align = "center"
			}
			fmt.Fprintf(&b, " line:%d%% position:%d%% align:%s", line, pos, align)
		}
		fmt.Fprintf(&b, "\n%s\n", s.Text)
	}
	return b.String()
}

// ASS renders an ASS track with pixel-exact positioning, outline and fades.
func ASS(m *strip.Manifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\n\n",
		m.Video.Width, m.Video.Height)
	b.WriteString("[V4+ Styles]\n" +
		"Format: Name,Fontname,Fontsize,PrimaryColour,OutlineColour,BorderStyle,Outline,Shadow,Alignment,MarginL,MarginR,MarginV\n" +
		"Style: Default,Sans,42,&H00FFFFFF,&H00000000,1,2,0,2,30,30,30\n\n")
	b.WriteString("[Events]\nFormat: Layer,Start,End,Style,Name,MarginL,MarginR,MarginV,Effect,Text\n")
	for _, s := range m.Strips {
		if s.Text == "" {
			continue
		}
		start, end, err := s.StartEnd()
		if err != nil {
			continue
		}
		var tags []string
		cx := s.Box.X + s.Box.W/2
		cy := s.Box.Y + s.Box.H/2
		tags = append(tags, fmt.Sprintf(`\pos(%d,%d)`, cx, cy))
		if s.FadeInMs > 0 || s.FadeOutMs > 0 {
			tags = append(tags, fmt.Sprintf(`\fad(%d,%d)`, s.FadeInMs, s.FadeOutMs))
		}
		if s.Outline != nil && s.Outline.WidthPx > 0 {
			tags = append(tags, fmt.Sprintf(`\bord%.1f\3c%s`, s.Outline.WidthPx, assColor(s.Outline.Color)))
		}
		if s.Fill != "" {
			tags = append(tags, fmt.Sprintf(`\1c%s`, assColor(s.Fill)))
		}
		if s.FontSizePx > 0 {
			tags = append(tags, fmt.Sprintf(`\fs%d`, s.FontSizePx))
		}
		text := "{" + strings.Join(tags, "") + "}" + strings.ReplaceAll(s.Text, "\n", `\N`)
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", assTime(start), assTime(end), text)
	}
	return b.String()
}

// SRT renders a plain SRT track (text and timing only).
func SRT(m *strip.Manifest) string {
	var b strings.Builder
	n := 0
	for _, s := range m.Strips {
		if s.Text == "" {
			continue
		}
		start, end, err := s.StartEnd()
		if err != nil {
			continue
		}
		n++
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", n, srtTime(start), srtTime(end), s.Text)
	}
	return b.String()
}
