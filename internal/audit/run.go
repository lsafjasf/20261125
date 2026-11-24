package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const ToolVersion = "1.0.0"

func LoadManifest(path string) ([]ClipSpec, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, "", fmt.Errorf("parse manifest: %w", err)
	}
	if len(m.Clips) == 0 {
		return nil, "", errors.New("manifest contains no clips")
	}
	baseDir := filepath.Dir(path)
	specs := make([]ClipSpec, 0, len(m.Clips))
	seen := make(map[string]int)
	for i, clip := range m.Clips {
		merged := mergeSpec(m.Defaults, clip)
		if merged.Path == "" {
			return nil, "", fmt.Errorf("manifest clip %d has no path", i+1)
		}
		if merged.ID == "" {
			merged.ID = strings.TrimSuffix(filepath.Base(merged.Path), filepath.Ext(merged.Path))
		}
		seen[merged.ID]++
		if seen[merged.ID] > 1 {
			merged.ID = fmt.Sprintf("%s-%d", merged.ID, seen[merged.ID])
		}
		specs = append(specs, merged)
	}
	return specs, baseDir, nil
}

func mergeSpec(defaults, clip ClipSpec) ClipSpec {
	out := clip
	if out.Declared == "" {
		out.Declared = defaults.Declared
	}
	if out.Width == 0 {
		out.Width = defaults.Width
	}
	if out.Height == 0 {
		out.Height = defaults.Height
	}
	if out.PixelFormat == "" {
		out.PixelFormat = defaults.PixelFormat
	}
	if out.MaxSamples == 0 {
		out.MaxSamples = defaults.MaxSamples
	}
	return out
}

func SpecsFromPaths(paths []string, declared DeclaredRange, width, height int, pixelFormat string, maxSamples uint64) []ClipSpec {
	specs := make([]ClipSpec, 0, len(paths))
	for _, path := range paths {
		id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		specs = append(specs, ClipSpec{
			ID:          id,
			Path:        path,
			Declared:    declared,
			Width:       width,
			Height:      height,
			PixelFormat: pixelFormat,
			MaxSamples:  maxSamples,
		})
	}
	return specs
}

func Run(specs []ClipSpec, baseDir string, defaultMax uint64) Report {
	report := Report{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC(),
		ToolVersion:   ToolVersion,
		Policy:        "只根据采样的 8-bit code value 判定；不假设播放器标签正确，证据不足时保留两个范围假设。",
		Clips:         make([]ClipResult, 0, len(specs)),
	}
	for _, spec := range specs {
		declared, err := ParseDeclared(string(spec.Declared))
		if err != nil {
			report.Clips = append(report.Clips, ClipResult{ID: spec.ID, Error: err.Error()})
			continue
		}
		spec.Declared = declared
		info, acc, err := LoadClip(spec, baseDir, defaultMax)
		if err != nil {
			report.Clips = append(report.Clips, ClipResult{
				ID:    spec.ID,
				Input: LoadInfo{Path: spec.Path},
				Error: err.Error(),
			})
			continue
		}
		stats := acc.Stats(info.Width, info.Height, info.Frames)
		report.Clips = append(report.Clips, ClipResult{
			ID:         spec.ID,
			Input:      info,
			Stats:      stats,
			Assessment: Assess(declared, stats),
		})
	}
	report.Batch = summarize(report.Clips)
	return report
}

func summarize(clips []ClipResult) BatchSummary {
	s := BatchSummary{
		Clips:           len(clips),
		Decisions:       map[string]int{},
		Declared:        map[string]int{},
		CorrectedLabels: map[string]int{},
	}
	for _, clip := range clips {
		if clip.Error != "" || clip.Stats.Samples == 0 {
			continue
		}
		s.Analyzed++
		s.Decisions[clip.Assessment.Decision]++
		s.Declared[string(clip.Assessment.Declared)]++
		s.CorrectedLabels[clip.Assessment.CorrectedLabel]++
		if clip.Assessment.LabelStatus == "incorrect" || clip.Assessment.LabelStatus == "likely-incorrect" {
			s.LabelMismatches++
		}
		if clip.Assessment.Decision == "ambiguous" {
			s.Indeterminate++
		}
	}
	type pair struct {
		name  string
		count int
	}
	var ranked []pair
	for name, count := range s.Decisions {
		ranked = append(ranked, pair{name, count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count == ranked[j].count {
			return ranked[i].name < ranked[j].name
		}
		return ranked[i].count > ranked[j].count
	})
	if len(ranked) > 0 {
		if ranked[0].count*2 > s.Analyzed {
			s.DominantDecision = ranked[0].name
		} else {
			s.DominantDecision = "mixed"
		}
	}
	s.BatchConclusion = batchConclusion(s)
	return s
}

func batchConclusion(s BatchSummary) string {
	switch {
	case s.Analyzed == 0:
		return "没有成功分析的样片，无法做出批次结论。"
	case s.DominantDecision == "mixed":
		return fmt.Sprintf("批次判决不一致（%s）；不能用一个标签覆盖全部样片，应按逐片修正标签处理，并保留未决项的两个解释。", formatCounts(s.Decisions))
	case s.DominantDecision == "limited" && s.LabelMismatches > 0:
		return fmt.Sprintf("%d/%d 个样片倾向 limited 16–235；其中 %d 个声明标签不符，应优先修正这些样片的范围标签，并保留 full+抬黑/风格化暗部作为残余解释。", s.Decisions["limited"], s.Analyzed, s.LabelMismatches)
	case s.DominantDecision == "full" && s.LabelMismatches > 0:
		return fmt.Sprintf("%d/%d 个样片倾向 full 0–255；多数样片标签可保留，但 %d 个样片的声明与证据不符，黑电平应按 full 锚点复核。", s.Decisions["full"], s.Analyzed, s.LabelMismatches)
	case s.Indeterminate > 0:
		return fmt.Sprintf("%d/%d 个样片未决；批次层面不能把 16–235 自动解释为标签错误，必须同时保留 limited+标签错误与 full+抬黑/风格化暗部。", s.Indeterminate, s.Analyzed)
	default:
		return "样片判决与声明范围一致；未发现需要整体改标的证据。"
	}
}

func formatCounts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}
