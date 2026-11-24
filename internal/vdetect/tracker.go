package vdetect

import (
	"math"
	"sort"
)

// Tracker links same-position defects across frames.
type Tracker struct {
	nextID int
	tracks []Track
	height int
}

func NewTracker() *Tracker { return &Tracker{nextID: 1} }

func (tr *Tracker) Update(fr *FrameResult) {
	tr.height = maxInt(tr.height, fr.Height)
	gap := 30

	for i := range fr.Tears {
		t := &fr.Tears[i]
		id := tr.matchTear(t, fr.Index, gap)
		if id == 0 {
			id = tr.newTrack("tear", "tear", fr.Index, fr.PTSSeconds)
		}
		t.TrackID = id
		tr.appendPoint(id, TrackPoint{Frame: fr.Index, PTSSeconds: fr.PTSSeconds, Y: t.Y, ShiftPx: t.ShiftPx, Confidence: t.Confidence})
		fr.TrackIDs = appendUniqueInt(fr.TrackIDs, id)
	}

	if fr.FieldOrder != nil && fr.FieldOrder.Detected {
		id := tr.matchSimple("field_order", fr.Index, gap)
		if id == 0 {
			id = tr.newTrack("field_order", "field_order", fr.Index, fr.PTSSeconds)
		}
		fr.FieldOrder.TrackID = id
		tr.appendPoint(id, TrackPoint{Frame: fr.Index, PTSSeconds: fr.PTSSeconds, ShiftPx: fr.FieldOrder.ParityShiftPx, Confidence: fr.FieldOrder.Confidence})
		fr.TrackIDs = appendUniqueInt(fr.TrackIDs, id)
	}

	usedBlockTracks := map[int]bool{}
	for i := range fr.Blocks {
		b := &fr.Blocks[i]
		id := 0
		if b.TrackID != 0 && tr.trackExists(b.TrackID, "block") && !usedBlockTracks[b.TrackID] {
			id = b.TrackID
		}
		if id == 0 {
			id = tr.matchBlock(b, fr.Index, gap)
			if usedBlockTracks[id] {
				id = 0
			}
		}
		if id == 0 {
			id = tr.newTrack("block", "macroblock_corruption", fr.Index, fr.PTSSeconds)
		}
		usedBlockTracks[id] = true
		b.TrackID = id
		tr.appendPoint(id, TrackPoint{Frame: fr.Index, PTSSeconds: fr.PTSSeconds, X: b.X, Y: b.Y, W: b.W, H: b.H, Temporal: b.Temporal, Confidence: b.Conf})
		fr.TrackIDs = appendUniqueInt(fr.TrackIDs, id)
	}
}

func (tr *Tracker) trackExists(id int, kind string) bool {
	for _, t := range tr.tracks {
		if t.ID == id && t.Kind == kind {
			return true
		}
	}
	return false
}

func (tr *Tracker) newTrack(kind, classification string, frame int, pts float64) int {
	id := tr.nextID
	tr.nextID++
	tr.tracks = append(tr.tracks, Track{
		ID: id, Kind: kind, Classification: classification,
		StartFrame: frame, EndFrame: frame, FrameCount: 1,
		Confidence: 0, Positions: []TrackPoint{},
	})
	return id
}

func (tr *Tracker) appendPoint(id int, p TrackPoint) {
	for i := range tr.tracks {
		if tr.tracks[i].ID != id {
			continue
		}
		tr.tracks[i].Positions = append(tr.tracks[i].Positions, p)
		tr.tracks[i].EndFrame = p.Frame
		tr.tracks[i].FrameCount = len(tr.tracks[i].Positions)
		return
	}
}

func (tr *Tracker) matchTear(t *Tear, frame, gap int) int {
	bestID, bestScore := 0, math.MaxFloat64
	for _, track := range tr.tracks {
		if track.Kind != "tear" || frame-track.EndFrame > gap || len(track.Positions) == 0 {
			continue
		}
		last := track.Positions[len(track.Positions)-1]
		predY := last.Y
		if len(track.Positions) >= 2 {
			prev := track.Positions[len(track.Positions)-2]
			dt := maxInt(1, last.Frame-prev.Frame)
			vy := float64(last.Y-prev.Y) / float64(dt)
			predY = last.Y + int(math.Round(vy*float64(maxInt(1, frame-last.Frame))))
		}
		yTol := maxInt(48, tr.height/10)
		yDist := math.Abs(float64(t.Y - predY))
		shiftTol := math.Max(8, math.Abs(last.ShiftPx)*0.45)
		shiftDist := math.Abs(t.ShiftPx - last.ShiftPx)
		if yDist > float64(yTol) || shiftDist > shiftTol {
			continue
		}
		score := yDist/float64(yTol) + shiftDist/shiftTol
		if score < bestScore {
			bestScore, bestID = score, track.ID
		}
	}
	return bestID
}

func (tr *Tracker) matchSimple(kind string, frame, gap int) int {
	for i := len(tr.tracks) - 1; i >= 0; i-- {
		t := tr.tracks[i]
		if t.Kind == kind && frame-t.EndFrame <= gap {
			return t.ID
		}
	}
	return 0
}

func (tr *Tracker) matchBlock(b *BlockDefect, frame, gap int) int {
	bestID, bestScore := 0, math.MaxFloat64
	box := b.Box
	for _, track := range tr.tracks {
		if track.Kind != "block" || frame-track.EndFrame > gap || len(track.Positions) == 0 {
			continue
		}
		last := track.Positions[len(track.Positions)-1]
		lastBox := Box{X: last.X, Y: last.Y, W: maxInt(1, last.W), H: maxInt(1, last.H)}
		iou := intersectionOverUnion(box, lastBox)
		centerDist := math.Hypot(float64(box.X+box.W/2-lastBox.X-lastBox.W/2), float64(box.Y+box.H/2-lastBox.Y-lastBox.H/2))
		centerTol := float64(maxInt(40, maxInt(box.W, lastBox.W)*2))
		if iou < 0.04 && centerDist > centerTol {
			continue
		}
		score := centerDist / centerTol * (1 - clamp01(iou))
		if score < bestScore {
			bestScore, bestID = score, track.ID
		}
	}
	return bestID
}

func intersectionOverUnion(a, b Box) float64 {
	x0 := maxInt(a.X, b.X)
	y0 := maxInt(a.Y, b.Y)
	x1 := minInt(a.X+a.W, b.X+b.W)
	y1 := minInt(a.Y+a.H, b.Y+b.H)
	if x1 <= x0 || y1 <= y0 {
		return 0
	}
	inter := (x1 - x0) * (y1 - y0)
	union := a.W*a.H + b.W*b.H - inter
	if union <= 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// Finalize computes continuity and rolling/field/block classifications.
func (tr *Tracker) Finalize() []Track {
	for i := range tr.tracks {
		t := &tr.tracks[i]
		span := maxInt(1, t.EndFrame-t.StartFrame+1)
		density := float64(t.FrameCount) / float64(span)
		longest := longestConsecutiveRun(t.Positions)
		t.GapFrames = maxInt(0, span-t.FrameCount)
		t.Continuity = round3(clamp01(density * (0.72 + 0.28*float64(longest)/float64(maxInt(1, t.FrameCount)))))

		var conf float64
		for _, p := range t.Positions {
			conf += p.Confidence
		}
		conf /= float64(maxInt(1, len(t.Positions)))
		t.Confidence = round3(clamp01(conf * (0.78 + 0.22*t.Continuity)))

		switch t.Kind {
		case "tear":
			tr.finalizeTearTrack(t)
		case "field_order":
			t.Classification = "field_order_error"
		case "block":
			t.Classification = "macroblock_corruption"
		}
	}
	sort.Slice(tr.tracks, func(i, j int) bool { return tr.tracks[i].ID < tr.tracks[j].ID })
	return append([]Track(nil), tr.tracks...)
}

func (tr *Tracker) finalizeTearTrack(t *Track) {
	if len(t.Positions) < 2 {
		t.Classification = "tear"
		return
	}
	t.YStart = t.Positions[0].Y
	t.YEnd = t.Positions[len(t.Positions)-1].Y

	var velocities []float64
	positive, negative, changes, wraps := 0, 0, 0, 0
	for i := 1; i < len(t.Positions); i++ {
		p0, p1 := t.Positions[i-1], t.Positions[i]
		dt := maxInt(1, p1.Frame-p0.Frame)
		dy := p1.Y - p0.Y
		unwrappedDY := dy
		if tr.height > 0 {
			if dy < -tr.height/2 {
				unwrappedDY += tr.height
				wraps++
			} else if dy > tr.height/2 {
				unwrappedDY -= tr.height
				wraps++
			}
		}
		v := float64(unwrappedDY) / float64(dt)
		velocities = append(velocities, v)
		if math.Abs(v) > 0.15 {
			changes++
			if v > 0 {
				positive++
			} else {
				negative++
			}
		}
	}
	sortedV := append([]float64(nil), velocities...)
	sort.Float64s(sortedV)
	medianV := sortedV[len(sortedV)/2]
	t.VelocityYPx = round2(medianV)
	t.WrapCount = wraps
	if changes > 0 {
		t.MonotonicRatio = round3(float64(maxInt(positive, negative)) / float64(changes))
	}

	speed := math.Abs(medianV)
	minSpeed := math.Max(2.5, float64(tr.height)*0.012)
	looksRolling := len(t.Positions) >= 3 && t.MonotonicRatio >= 0.68 &&
		(wraps >= 1 || speed >= minSpeed) && t.Continuity >= 0.45
	if looksRolling {
		t.Classification = "capture_rolling"
		t.Confidence = round3(clamp01(t.Confidence + 0.12*t.MonotonicRatio + 0.05*float64(wraps)))
	} else {
		t.Classification = "tear"
	}
}

func longestConsecutiveRun(ps []TrackPoint) int {
	if len(ps) == 0 {
		return 0
	}
	best, run := 1, 1
	for i := 1; i < len(ps); i++ {
		if ps[i].Frame == ps[i-1].Frame+1 {
			run++
			best = maxInt(best, run)
		} else {
			run = 1
		}
	}
	return best
}

func appendUniqueInt(in []int, v int) []int {
	for _, x := range in {
		if x == v {
			return in
		}
	}
	return append(in, v)
}
