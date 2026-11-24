package vdetect

import (
	"math"
	"sort"
	"strings"
)

// Analyzer keeps frame history and cross-frame defect tracks.
type Analyzer struct {
	opt        Options
	tracker    *Tracker
	prev       *Frame
	prevG      *GrayFrame
	brightness []float64
	frames     []FrameResult
}

func NewAnalyzer(opt Options) *Analyzer {
	if opt.Sensitivity <= 0 {
		opt.Sensitivity = 0.62
	}
	if opt.MinConfidence <= 0 {
		opt.MinConfidence = 0.55
	}
	if opt.AnalysisWidth <= 0 {
		opt.AnalysisWidth = 448
	}
	if opt.FPS <= 0 {
		opt.FPS = 30
	}
	if opt.MaxShiftRatio <= 0 {
		opt.MaxShiftRatio = 0.09
	}
	return &Analyzer{opt: opt, tracker: NewTracker()}
}

// Process analyzes one frame in decode order.
func (a *Analyzer) Process(f *Frame) FrameResult {
	idx := len(a.frames)
	f.Index = idx
	if f.PTS == 0 && a.opt.FPS > 0 {
		f.PTS = float64(idx) / a.opt.FPS
	}

	mean := f.MeanLuma()
	a.brightness = append(a.brightness, mean)
	slope := brightnessSlope(a.brightness)

	g := f.DownscaleGray(a.opt.AnalysisWidth)
	motion := analyzeMotion(f, g, a.prevG, slope)
	guards := motion.guards

	tears := detectTears(g, a.opt)
	field := detectFieldOrder(f, a.opt)
	lumaDelta := 0.0
	if a.prev != nil && a.prev.W == f.W && a.prev.H == f.H {
		lumaDelta = mean - a.prev.MeanLuma()
	}
	blocks := detectBlockDefectsDelta(f, a.prev, lumaDelta, a.opt)

	var suppressed int
	tears, suppressed = guardTears(tears, motion, g, a.opt)
	guards.SuppressedCandidates += suppressed
	blocks, suppressed = suppressBlocksOnTears(blocks, tears)
	guards.SuppressedCandidates += suppressed
	blocks, suppressed = guardBlocks(blocks, motion, g, guards, a.opt)
	guards.SuppressedCandidates += suppressed

	tears = filterTears(tears, a.opt.MinConfidence)
	blocks = filterBlocks(blocks, a.opt.MinConfidence)
	blocks = a.coastBlocks(blocks, idx)
	if field != nil && (!field.Detected || field.Confidence < a.opt.MinConfidence) {
		if field.Confidence < a.opt.MinConfidence*0.8 {
			field = nil
		}
	}

	effects := append([]EffectRegion(nil), motion.effect...)
	effects = append(effects, motion.foreground...)

	fr := FrameResult{
		Index: idx, PTSSeconds: f.PTS, Width: f.W, Height: f.H,
		Tears: tears, FieldOrder: field, Blocks: blocks, Effects: effects, Guards: guards,
	}

	labels := make([]string, 0, 4)
	conf := 0.0
	if field != nil && field.Detected && field.Confidence >= a.opt.MinConfidence {
		labels = append(labels, "field_order_error")
		conf = maxFloat(conf, field.Confidence)
	}
	if len(tears) > 0 {
		labels = append(labels, "tear")
		for _, t := range tears {
			conf = maxFloat(conf, t.Confidence)
		}
	}
	if len(blocks) > 0 {
		labels = append(labels, "macroblock_corruption")
		for _, b := range blocks {
			conf = maxFloat(conf, b.Conf)
		}
	}
	gameEffects := 0
	for _, e := range motion.effect {
		if e.Kind == "game_effect" {
			gameEffects++
		}
	}
	if gameEffects > 0 {
		labels = append(labels, "game_effect")
		conf = maxFloat(conf, 0.56+0.04*float64(gameEffects))
	}

	fr.Labels = uniqueStrings(labels)
	switch len(fr.Labels) {
	case 0:
		fr.Classification = "none"
	case 1:
		fr.Classification = fr.Labels[0]
	default:
		fr.Classification = "mixed"
	}
	fr.Confidence = round3(clamp01(conf))
	if guards.GlobalBrightening {
		fr.Notes = append(fr.Notes, "global_brightening_normalized")
	}
	if guards.MotionCoverage > 0.18 {
		fr.Notes = append(fr.Notes, "motion_mask_active")
	}

	a.tracker.Update(&fr)
	a.frames = append(a.frames, fr)
	a.prev = f
	a.prevG = g
	return fr
}

// Report finalizes tracks and returns the complete report.
func (a *Analyzer) Report(input string) Report {
	tracks := a.tracker.Finalize()
	dispositions := reconcileBlockTracks(tracks, a.frames)
	a.applyBlockDispositions(dispositions)
	for _, tr := range tracks {
		if tr.Classification != "capture_rolling" {
			continue
		}
		for _, p := range tr.Positions {
			if p.Frame < 0 || p.Frame >= len(a.frames) {
				continue
			}
			fr := &a.frames[p.Frame]
			fr.Labels = appendUniqueString(fr.Labels, "capture_rolling")
			fr.Notes = appendUniqueString(fr.Notes, "seam_position_advances_across_frames")
			if fr.Classification == "tear" {
				fr.Classification = "capture_rolling"
			}
		}
	}

	width, height := a.opt.Width, a.opt.Height
	if len(a.frames) > 0 {
		width = a.frames[0].Width
		height = a.frames[0].Height
	}
	summary := Summary{Frames: len(a.frames), Tracks: len(tracks)}
	for _, fr := range a.frames {
		if fr.Classification != "none" {
			summary.FramesWithFindings++
		}
		if len(fr.Tears) > 0 {
			summary.TearFrames++
		}
		if fr.FieldOrder != nil && fr.FieldOrder.Detected && fr.FieldOrder.Confidence >= a.opt.MinConfidence {
			summary.FieldOrderFrames++
		}
		if hasLabel(fr.Labels, "capture_rolling") || fr.Classification == "capture_rolling" {
			summary.RollingFrames++
		}
		if len(fr.Blocks) > 0 {
			summary.BlockFrames++
		}
		if hasLabel(fr.Labels, "game_effect") {
			summary.EffectFrames++
		}
		summary.MaxConfidence = maxFloat(summary.MaxConfidence, fr.Confidence)
	}

	return Report{
		SchemaVersion: "1.0",
		Tool:          "videotear",
		Input:         input,
		Width:         width,
		Height:        height,
		FPS:           a.opt.FPS,
		Options:       a.opt,
		Summary:       summary,
		Frames:        a.frames,
		Tracks:        tracks,
	}
}

func reconcileBlockTracks(tracks []Track, frames []FrameResult) map[int]string {
	out := map[int]string{}
	for i := range tracks {
		tr := &tracks[i]
		if tr.Kind != "block" || len(tr.Positions) == 0 {
			continue
		}
		nearTear := false
		var velocities []float64
		positive, negative, changes := 0, 0, 0
		var temporalSum float64
		temporalStrong := 0
		maxPointConf := 0.0
		width := 0
		for k, p := range tr.Positions {
			if p.Frame >= 0 && p.Frame < len(frames) {
				fr := frames[p.Frame]
				width = maxInt(width, fr.Width)
				pb := Box{X: p.X, Y: p.Y, W: maxInt(1, p.W), H: maxInt(1, p.H)}
				for _, te := range fr.Tears {
					if pb.X+pb.W > te.XStart-80 && pb.X < te.XEnd+80 && te.Y >= pb.Y-14 && te.Y <= pb.Y+pb.H+14 {
						nearTear = true
					}
				}
			}
			temporalSum += p.Temporal
			if p.Temporal >= 24 {
				temporalStrong++
			}
			maxPointConf = maxFloat(maxPointConf, p.Confidence)
			if k > 0 {
				q := tr.Positions[k-1]
				dt := maxInt(1, p.Frame-q.Frame)
				cx0 := float64(q.X + q.W/2)
				cy0 := float64(q.Y + q.H/2)
				cx1 := float64(p.X + p.W/2)
				cy1 := float64(p.Y + p.H/2)
				v := math.Hypot(cx1-cx0, cy1-cy0) / float64(dt)
				velocities = append(velocities, v)
				if v > 2 {
					changes++
					if cx1 > cx0 || cy1 > cy0 {
						positive++
					} else {
						negative++
					}
				}
			}
		}
		if nearTear {
			out[tr.ID] = "tear_artifact"
			tr.Classification = "tear_artifact"
			continue
		}
		if len(tr.Positions) >= 8 {
			avgTemporal := temporalSum / float64(len(tr.Positions))
			if avgTemporal >= 22 && float64(temporalStrong)/float64(len(tr.Positions)) >= 0.65 {
				out[tr.ID] = "ongoing_motion_or_texture"
				tr.Classification = "ongoing_motion_or_texture"
				continue
			}
		}
		if tr.Confidence < 0.84 && maxPointConf < 0.96 {
			out[tr.ID] = "unconfirmed_texture"
			tr.Classification = "unconfirmed_texture"
			continue
		}
		if len(velocities) >= 2 {
			sorted := append([]float64(nil), velocities...)
			sort.Float64s(sorted)
			median := sorted[len(sorted)/2]
			mono := float64(maxInt(positive, negative)) / float64(maxInt(1, changes))
			speedThreshold := math.Max(11, float64(width)*0.012)
			if median >= speedThreshold && mono >= 0.62 {
				out[tr.ID] = "moving_foreground"
				tr.Classification = "moving_foreground"
			}
		}
	}
	return out
}

func (a *Analyzer) applyBlockDispositions(d map[int]string) {
	if len(d) == 0 {
		return
	}
	for i := range a.frames {
		fr := &a.frames[i]
		if len(fr.Blocks) == 0 {
			continue
		}
		kept := fr.Blocks[:0]
		for _, b := range fr.Blocks {
			disp := d[b.TrackID]
			if disp == "" {
				kept = append(kept, b)
				continue
			}
			if disp == "moving_foreground" {
				fr.Effects = append(fr.Effects, EffectRegion{
					Box: b.Box, Kind: "moving_foreground", Motion: b.Temporal,
				})
			}
		}
		fr.Blocks = kept
		rebuildClassification(fr)
	}
}

func rebuildClassification(fr *FrameResult) {
	labels := make([]string, 0, 5)
	conf := 0.0
	if fr.FieldOrder != nil && fr.FieldOrder.Detected {
		labels = append(labels, "field_order_error")
		conf = maxFloat(conf, fr.FieldOrder.Confidence)
	}
	if len(fr.Tears) > 0 {
		labels = append(labels, "tear")
		for _, t := range fr.Tears {
			conf = maxFloat(conf, t.Confidence)
		}
	}
	if len(fr.Blocks) > 0 {
		labels = append(labels, "macroblock_corruption")
		for _, b := range fr.Blocks {
			conf = maxFloat(conf, b.Conf)
		}
	}
	for _, e := range fr.Effects {
		if e.Kind == "game_effect" {
			labels = append(labels, "game_effect")
			conf = maxFloat(conf, 0.58)
			break
		}
	}
	fr.Labels = uniqueStrings(labels)
	if len(fr.Labels) == 0 {
		fr.Classification = "none"
		fr.Confidence = 0
	} else if len(fr.Labels) == 1 {
		fr.Classification = fr.Labels[0]
		fr.Confidence = round3(clamp01(conf))
	} else {
		fr.Classification = "mixed"
		fr.Confidence = round3(clamp01(conf))
	}
}

func guardTears(tears []Tear, motion motionInfo, g *GrayFrame, opt Options) ([]Tear, int) {
	var out []Tear
	suppressed := 0
	for _, t := range tears {
		gy := clampInt(int(float64(t.Y)/g.ScaleY), 0, g.H-1)
		gx0 := clampInt(int(float64(t.XStart)/g.ScaleX), 0, g.W-1)
		gx1 := clampInt(int(float64(t.XEnd)/g.ScaleX), gx0+1, g.W)
		var hit, n int
		for y := maxInt(0, gy-2); y <= minInt(g.H-1, gy+2); y++ {
			for x := gx0; x < gx1; x++ {
				n++
				if motion.mask != nil && motion.mask[y*g.W+x] {
					hit++
				}
			}
		}
		overlap := float64(hit) / float64(maxInt(1, n))
		if overlap > 0.55 && t.Coverage < 0.64 && t.Confidence < 0.74 {
			t.Confidence = round3(t.Confidence * 0.70)
			t.Evidence = appendUniqueString(t.Evidence, "moving_foreground_overlap_downgrade")
		}
		if overlap > 0.72 && t.Coverage < 0.45 && t.Confidence < 0.62 {
			suppressed++
			continue
		}
		out = append(out, t)
	}
	return out, suppressed
}

func guardBlocks(blocks []BlockDefect, motion motionInfo, g *GrayFrame, guards Guards, opt Options) ([]BlockDefect, int) {
	var out []BlockDefect
	suppressed := 0
	for _, b := range blocks {
		gx0 := clampInt(int(float64(b.X)/g.ScaleX), 0, g.W-1)
		gy0 := clampInt(int(float64(b.Y)/g.ScaleY), 0, g.H-1)
		gx1 := clampInt(int(float64(b.X+b.W)/g.ScaleX), gx0+1, g.W)
		gy1 := clampInt(int(float64(b.Y+b.H)/g.ScaleY), gy0+1, g.H)
		var hit, n int
		for y := gy0; y < gy1; y++ {
			for x := gx0; x < gx1; x++ {
				n++
				if motion.mask != nil && motion.mask[y*g.W+x] {
					hit++
				}
			}
		}
		motionFraction := float64(hit) / float64(maxInt(1, n))
		marginX := maxInt(5, int(26/g.ScaleX))
		marginY := maxInt(5, int(26/g.ScaleY))
		ex0, ey0 := maxInt(0, gx0-marginX), maxInt(0, gy0-marginY)
		ex1, ey1 := minInt(g.W, gx1+marginX), minInt(g.H, gy1+marginY)
		var ringHit, ringN int
		for y := ey0; y < ey1; y++ {
			for x := ex0; x < ex1; x++ {
				if x >= gx0 && x < gx1 && y >= gy0 && y < gy1 {
					continue
				}
				ringN++
				if motion.mask != nil && motion.mask[y*g.W+x] {
					ringHit++
				}
			}
		}
		ringMotion := float64(ringHit) / float64(maxInt(1, ringN))
		gridSpecific := hasString(b.Evidence, "concealment_color_cast") ||
			hasString(b.Evidence, "eight_pixel_grid_response") ||
			hasString(b.Evidence, "high_frequency_noise_texture")
		motionContext := ringMotion > 0.10 && motionFraction < ringMotion*2.8
		if motionContext && b.ColorScore < 0.75 {
			suppressed++
			continue
		}
		if ringMotion > 0.18 && b.ColorScore < 0.85 {
			suppressed++
			continue
		}
		if motionFraction > 0.42 && !gridSpecific {
			b.Conf = round3(b.Conf * 0.42)
			b.Evidence = appendUniqueString(b.Evidence, "moving_foreground_explained")
		}
		if motionFraction > 0.62 && !gridSpecific {
			suppressed++
			continue
		}
		if guards.GlobalBrightening && b.Temporal > 20 && b.ColorScore < 0.35 && !hasString(b.Evidence, "block_boundary_spike") {
			b.Conf = round3(b.Conf * 0.72)
			b.Evidence = appendUniqueString(b.Evidence, "brightening_normalized_downgrade")
		}
		if b.Conf < opt.MinConfidence {
			suppressed++
			continue
		}
		out = append(out, b)
	}
	return out, suppressed
}

func suppressBlocksOnTears(blocks []BlockDefect, tears []Tear) ([]BlockDefect, int) {
	if len(tears) == 0 {
		return blocks, 0
	}
	var out []BlockDefect
	suppressed := 0
	for _, b := range blocks {
		onTear := false
		for _, t := range tears {
			horizontalOverlap := b.X+b.W > t.XStart-72 && b.X < t.XEnd+72
			verticalHit := t.Y >= b.Y-10 && t.Y <= b.Y+b.H+10
			if horizontalOverlap && verticalHit {
				onTear = true
				break
			}
		}
		if onTear && !hasString(b.Evidence, "concealment_color_cast") {
			suppressed++
			continue
		}
		out = append(out, b)
	}
	return out, suppressed
}

func (a *Analyzer) coastBlocks(blocks []BlockDefect, frame int) []BlockDefect {
	out := append([]BlockDefect(nil), blocks...)
	for _, tr := range a.tracker.tracks {
		if tr.Kind != "block" || len(tr.Positions) == 0 {
			continue
		}
		last := tr.Positions[len(tr.Positions)-1]
		if last.Frame != frame-1 {
			continue
		}
		lastBox := Box{X: last.X, Y: last.Y, W: maxInt(1, last.W), H: maxInt(1, last.H)}
		matched := false
		for _, b := range blocks {
			centerDist := absFloat(float64(b.X+b.W/2-lastBox.X-lastBox.W/2)) +
				absFloat(float64(b.Y+b.H/2-lastBox.Y-lastBox.H/2))
			centerTol := float64(maxInt(40, maxInt(b.W, lastBox.W)*2))
			if intersectionOverUnion(b.Box, lastBox) > 0.08 || centerDist < centerTol {
				matched = true
				break
			}
		}
		if matched || last.Confidence < 0.72 {
			continue
		}
		coast := BlockDefect{
			Box:       Box{X: last.X, Y: last.Y, W: last.W, H: last.H, Conf: round3(last.Confidence * 0.82), Reason: "persistent_macroblock_track"},
			BlockSize: 16, GridAligned: true, TrackID: tr.ID,
			Evidence: []string{"cross_frame_persistence", "track_coasting"},
		}
		if coast.Conf >= a.opt.MinConfidence {
			out = append(out, coast)
		}
	}
	return out
}

func filterTears(in []Tear, minConf float64) []Tear {
	out := in[:0:0]
	for _, t := range in {
		if t.Confidence >= maxFloat(0.42, minConf) {
			out = append(out, t)
		}
	}
	return out
}

func filterBlocks(in []BlockDefect, minConf float64) []BlockDefect {
	out := in[:0:0]
	for _, b := range in {
		if b.Conf >= maxFloat(0.70, minConf) {
			out = append(out, b)
		}
	}
	return out
}

func uniqueStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" && !hasString(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func appendUniqueString(in []string, s string) []string {
	if hasString(in, s) {
		return in
	}
	return append(in, s)
}

func hasString(in []string, s string) bool {
	for _, v := range in {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

func hasLabel(in []string, label string) bool { return hasString(in, label) }

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
