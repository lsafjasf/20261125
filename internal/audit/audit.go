// Package audit checks every strip property against a target format profile.
package audit

import (
	"fmt"
	"sort"

	"subaudit/internal/caps"
	"subaudit/internal/strip"
)

// Verdict is the per-strip migration outcome.
type Verdict string

const (
	Migrated    Verdict = "migrated"     // every property exactly expressible
	Degraded    Verdict = "degraded"     // only approximate expressions needed
	Loss        Verdict = "loss"         // at least one property cannot be expressed
	NotMigrated Verdict = "not_migrated" // strip cannot become a text cue at all
)

// Finding is one property-level audit result for one strip.
type Finding struct {
	Property string
	Level    caps.Level
	Reason   string
	Fix      string // manual repair suggestion
}

// Result is the full audit of one strip.
type Result struct {
	Strip    strip.Strip
	Verdict  Verdict
	Findings []Finding // only non-exact findings are recorded
}

// Report is the whole audit run.
type Report struct {
	Format  string
	Results []Result
}

// fixSuggestions maps properties to manual repair advice.
var fixSuggestions = map[string]string{
	caps.PropFill:     "在目标文件中显式写颜色标签；若播放器忽略，人工抽查实际渲染颜色。",
	caps.PropFontSize: "在目标格式样式中重建字号；SRT 无此能力，需改用 WebVTT/ASS 或接受播放器默认字号。",
	caps.PropAlign:    "改用支持对齐的格式（WebVTT/ASS/TTML），或人工确认默认对齐可接受。",
	caps.PropOutline:  "描边无法直接表达：WebVTT 可在播放器侧注入 ::cue{text-shadow:...} 模拟细描边并人工核对可读性；需要精确描边请改用 ASS(\\bord) 或 TTML(tts:textOutline)。",
	caps.PropShadow:   "阴影需用播放器 CSS(text-shadow) 或改用 ASS(\\shad) 重建；人工核对偏移与颜色。",
	caps.PropFade:     "淡入淡出在目标格式中无对应能力：改用 ASS(\\fad) 可精确保留；若必须留在该格式，人工确认硬切可接受，或在合成阶段将淡变烘回画面。",
	caps.PropPosition: "像素级定位无法精确表达：WebVTT 用 line/position 百分比近似后需人工逐条核对落点；需要精确定位请改用 ASS(\\pos) 或 TTML(region)。",
	caps.PropRotation: "旋转条带无法表达：保留原图作为图片字幕轨，或在合成阶段烘回画面；ASS 仅支持绕文本轴旋转(\\frz)，需人工确认效果等价。",
	caps.PropOverlap:  "同一时刻多条字幕会被播放器重排：改用 ASS/TTML 独立定位，或人工调整时序/合并条带避免遮挡。",
	caps.PropText:     "条带无文本内容（纯图像）：先人工听写或 OCR 补录文本，再重新审计；在此之前该条只能保留为图像轨。",
}

// Run audits every strip in the manifest against the named format.
func Run(m *strip.Manifest, profile caps.Profile) *Report {
	rep := &Report{Format: profile.Name}
	overlaps := findOverlaps(m.Strips)
	for _, s := range m.Strips {
		rep.Results = append(rep.Results, auditStrip(s, profile, overlaps[s.ID]))
	}
	return rep
}

func auditStrip(s strip.Strip, profile caps.Profile, overlap bool) Result {
	r := Result{Strip: s, Verdict: Migrated}
	add := func(prop, reason string) {
		c := profile.Get(prop)
		if c.Level == caps.Exact {
			return
		}
		r.Findings = append(r.Findings, Finding{
			Property: prop,
			Level:    c.Level,
			Reason:   fmt.Sprintf("%s（%s）", reason, c.Note),
			Fix:      fixSuggestions[prop],
		})
	}

	if s.Text == "" {
		r.Verdict = NotMigrated
		r.Findings = append(r.Findings, Finding{
			Property: caps.PropText,
			Level:    caps.None,
			Reason:   "条带无文本内容，无法转换为文本轨",
			Fix:      fixSuggestions[caps.PropText],
		})
		return r
	}

	if s.Fill != "" {
		add(caps.PropFill, "条带使用自定义填充色 "+s.Fill)
	}
	if s.FontSizePx > 0 {
		add(caps.PropFontSize, fmt.Sprintf("条带字号 %dpx", s.FontSizePx))
	}
	if s.Align != "" && s.Align != "center" {
		add(caps.PropAlign, "条带对齐方式 "+s.Align)
	}
	if s.Outline != nil && s.Outline.WidthPx > 0 {
		add(caps.PropOutline, fmt.Sprintf("条带带 %.1fpx 描边(%s)", s.Outline.WidthPx, s.Outline.Color))
	}
	if s.Shadow != nil {
		add(caps.PropShadow, "条带带投影")
	}
	if s.FadeInMs > 0 || s.FadeOutMs > 0 {
		add(caps.PropFade, fmt.Sprintf("淡入 %dms / 淡出 %dms", s.FadeInMs, s.FadeOutMs))
	}
	if s.Anchor != "" && s.Anchor != "bottom-center" {
		add(caps.PropPosition, fmt.Sprintf("条带位于 (%d,%d) 锚点 %s，非默认底部居中", s.Box.X, s.Box.Y, s.Anchor))
	}
	if s.Rotation != 0 {
		add(caps.PropRotation, fmt.Sprintf("条带旋转 %.1f°", s.Rotation))
	}
	if overlap {
		add(caps.PropOverlap, "与同时刻其他条带时间重叠")
	}

	for _, f := range r.Findings {
		if f.Level == caps.None {
			r.Verdict = Loss
			break
		}
		r.Verdict = Degraded
	}
	return r
}

// findOverlaps returns the set of strip IDs that overlap in time with another strip.
func findOverlaps(strips []strip.Strip) map[string]bool {
	type iv struct {
		id         string
		start, end int64
	}
	var ivs []iv
	for _, s := range strips {
		start, end, err := s.StartEnd()
		if err != nil {
			continue
		}
		ivs = append(ivs, iv{s.ID, int64(start), int64(end)})
	}
	sort.Slice(ivs, func(i, j int) bool { return ivs[i].start < ivs[j].start })
	out := map[string]bool{}
	for i := 0; i < len(ivs); i++ {
		for j := i + 1; j < len(ivs) && ivs[j].start < ivs[i].end; j++ {
			out[ivs[i].id] = true
			out[ivs[j].id] = true
		}
	}
	return out
}

// Counts tallies verdicts for the summary.
func (r *Report) Counts() map[Verdict]int {
	c := map[Verdict]int{}
	for _, res := range r.Results {
		c[res.Verdict]++
	}
	return c
}
