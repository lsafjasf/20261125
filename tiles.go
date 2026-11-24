package main

import "math"

func (d *Detector) scoreTile(f *Frame, feat frameFeatures, motion []float64, tx, ty int) TileReport {
	cfg := d.cfg
	ts := cfg.TileSize
	w := f.W
	x0, y0 := tx*ts, ty*ts
	t := TileReport{TX: tx, TY: ty, X: x0, Y: y0, Size: ts,
		Motion: motion[ty*d.cols+tx]}

	edgeCount, edgeSum := 0, 0.0
	newEdgeCount := 0
	lumaSum := 0
	for yy := y0; yy < y0+ts; yy++ {
		for xx := x0; xx < x0+ts; xx++ {
			i := yy*w + xx
			lumaSum += int(f.Y[i])
			if feat.edgePix[i] {
				edgeCount++
				edgeSum += feat.sobel[i]
				if feat.refEdge != nil && !feat.refEdge[i] {
					newEdgeCount++
				}
			}
		}
	}
	n := ts * ts
	t.MeanLuma = float64(lumaSum) / float64(n)
	if edgeCount > 0 {
		t.EdgeEnergy = edgeSum / float64(edgeCount)
	}

	t.BlockBoundary, t.BlockInterior = d.blockEnergy(f, feat, x0, y0)
	t.BlockRatio = t.BlockBoundary / (t.BlockInterior + 0.5)
	t.Ringing = d.ringingEnergy(feat, x0, y0)
	gap, levels := d.bandingStats(f, x0, y0)
	t.BandingGap, t.BandingLevels = gap, levels
	gridEdgeOnly := true
	noRefGridThr := cfg.BlockBoundaryThr
	if feat.refBoundaryByTile != nil {
		tileID := ty*d.cols + tx
		refBound := feat.refBoundaryByTile[tileID]
		refInter := feat.refInteriorByTile[tileID]
		dRatio := t.BlockBoundary / (t.BlockInterior + 0.5)
		rRatio := refBound / (refInter + 0.5)
		// A genuine content boundary already has a high grid ratio in the
		// source. Codec blocking is a grid spike on pixels that were smooth in
		// the source (low reference ratio and low reference interior energy).
		gridEdgeOnly = rRatio < cfg.BlockRefSmooth && refInter < cfg.BlockInteriorMax &&
			dRatio >= cfg.BlockRatioThr
	} else {
		// Without a source, demand a strong absolute grid spike so smooth
		// natural gradients are not mistaken for codec blocking.
		noRefGridThr = cfg.BlockBoundaryNoRef
	}

	switch {
	case t.Motion >= cfg.MotionMeanThr:
		t.Label = LabelMotion
		t.Confidence = clamp01(t.Motion / (cfg.MotionMeanThr * 3))
	case edgeCount > 0 && edgeCount >= n/8:
		// Dense genuine edge structure (text/line art). Even when echoes are
		// present, edge density dominates: this is content, not ringing damage.
		t.Label = LabelTextEdge
		t.Confidence = clamp01(t.EdgeEnergy / 400)
	// Blocking needs an 8px-grid discontinuity inside otherwise smooth content.
	// Hard posterization steps are tone damage, and edges already present in the
	// clean reference are content; neither may count as codec blocking.
	case t.BlockRatio >= cfg.BlockRatioThr && t.BlockBoundary >= cfg.BlockBoundaryThr &&
		t.BlockBoundary >= noRefGridThr && edgeCount < n/6 &&
		t.BlockInterior < cfg.BlockInteriorMax && gridEdgeOnly:
		t.Label = LabelBlocking
		t.Score = (t.BlockRatio-cfg.BlockRatioThr)*2 + t.BlockBoundary*0.25
		t.Confidence = clamp01(t.Score / 8)
	case d.isBanding(t):
		t.Label = LabelBandingComp
		t.Score = bandingSeverity(t)
		t.Confidence = clamp01(t.Score / 8)
	case t.Ringing >= cfg.RingingThr && edgeCount > 0:
		// Ringing is echo energy around an edge. A clean content edge (present
		// in the reference, or a sparse single line without one) keeps echo
		// small relative to its edge energy; compression raises the echo ratio.
		echoRatio := t.Ringing / (t.EdgeEnergy + 1)
		if feat.refEdge == nil {
			// Without a reference, echo-to-edge ratio is the only discriminator.
			if echoRatio < cfg.RingingEchoRatio {
				t.Label = LabelRealEdge
				t.Confidence = clamp01(t.EdgeEnergy / 400)
			} else {
				t.Label = LabelRinging
				t.Score = t.Ringing
				t.Confidence = clamp01(t.Score / 8)
			}
		} else if newEdgeCount == 0 && echoRatio < cfg.RingingEchoRatio {
			t.Label = LabelRealEdge
			t.Confidence = clamp01(t.EdgeEnergy / 400)
		} else {
			t.Label = LabelRinging
			t.Score = t.Ringing
			t.Confidence = clamp01(t.Score / 8)
		}
	case edgeCount > 0:
		t.Label = LabelRealEdge
		t.Confidence = clamp01(t.EdgeEnergy / 400)
	default:
		t.Label = LabelClean
		t.Confidence = 0.9
	}
	return t
}

// blockEnergy separates discontinuity energy on the 8/16-sample codec grid
// from the energy of ordinary interior pixel steps.
func (d *Detector) blockEnergy(f *Frame, feat frameFeatures, x0, y0 int) (boundary, interior float64) {
	ts := d.cfg.TileSize
	w := f.W
	bc, ic := 0, 0
	bs, is := 0.0, 0.0
	for yy := y0; yy < y0+ts; yy++ {
		for xx := x0; xx < x0+ts; xx++ {
			i := yy*w + xx
			if xx > 0 && xx%8 == 0 {
				bs += feat.vDiff[i]
				bc++
			} else if xx > x0 {
				is += feat.vDiff[i]
				ic++
			}
			if yy > 0 && yy%8 == 0 {
				bs += feat.hDiff[i]
				bc++
			} else if yy > y0 {
				is += feat.hDiff[i]
				ic++
			}
		}
	}
	if bc > 0 {
		boundary = bs / float64(bc)
	}
	if ic > 0 {
		interior = is / float64(ic)
	}
	return boundary, interior
}

// ringingEnergy measures high-pass oscillation around strong edges. A genuine
// sharp edge is a one-sided transition (residual keeps one sign on each side),
// while codec ringing is an echo: residual signs alternate on the same side.
// We therefore combine echo magnitude with the sign-change density.
func (d *Detector) ringingEnergy(feat frameFeatures, x0, y0 int) float64 {
	ts := d.cfg.TileSize
	w := feat.width()
	sum, n, flips := 0.0, 0, 0
	for yy := y0 + 1; yy < y0+ts-1; yy++ {
		for xx := x0 + 1; xx < x0+ts-1; xx++ {
			i := yy*w + xx
			near := feat.edgePix[i] || feat.edgePix[i-1] || feat.edgePix[i+1] ||
				feat.edgePix[i-w] || feat.edgePix[i+w]
			if near {
				sum += math.Abs(feat.resid[i])
				n++
				if signChange(feat.resid[i], feat.resid[i-1]) ||
					signChange(feat.resid[i], feat.resid[i-w]) {
					flips++
				}
			}
		}
	}
	if n == 0 {
		return 0
	}
	echo := sum / float64(n)
	flipRate := float64(flips) / float64(n)
	return echo * (0.5 + flipRate)
}

func signChange(a, b float64) bool {
	return (a > 0.6 && b < -0.6) || (a < -0.6 && b > 0.6)
}

// bandingStats inspects the luma histogram: quantization banding shows few
// occupied levels separated by empty gaps inside smooth dark gradients.
func (d *Detector) bandingStats(f *Frame, x0, y0 int) (maxGap, levels int) {
	ts := d.cfg.TileSize
	var hist [256]int
	for yy := y0; yy < y0+ts; yy++ {
		for xx := x0; xx < x0+ts; xx++ {
			hist[f.Y[yy*f.W+xx]]++
		}
	}
	minV, maxV := 255, 0
	for v, c := range hist {
		if c > 0 {
			if v < minV {
				minV = v
			}
			maxV = v
			levels++
		}
	}
	if levels == 0 {
		return 0, 0
	}
	run, gap := 0, 0
	bigRun := 0
	for v := minV; v <= maxV; v++ {
		if hist[v] == 0 {
			gap++
			if gap > maxGap {
				maxGap = gap
			}
			run = 0
		} else {
			run++
			if run > bigRun {
				bigRun = run
			}
			gap = 0
		}
	}
	// A long single populated run (e.g. a flat panel plus a few text pixels)
	// is not banding: require repeated empty steps, not range-edge emptiness.
	if bigRun >= ts*ts/2 {
		return 0, levels
	}
	return maxGap, levels
}

func (d *Detector) isBanding(t TileReport) bool {
	cfg := d.cfg
	return t.MeanLuma <= float64(cfg.DarkLumaThr) &&
		t.BandingLevels <= cfg.BandingMaxLevels &&
		t.BandingGap >= cfg.BandingMinGap && t.BandingGap > 0 &&
		t.EdgeEnergy < float64(cfg.EdgeMagThr)*3
}

func bandingSeverity(t TileReport) float64 {
	gapPenalty := float64(t.BandingGap) * 1.4
	levelPenalty := float64(16-min(t.BandingLevels, 16)) * 0.25
	darkness := 1.0 + (80.0-t.MeanLuma)/80.0
	return (gapPenalty + levelPenalty) * darkness
}

// classifyBandingCause splits codec quantization from tone/grade damage:
// codec banding co-occurs with block energy, tone banding is spatially wide
// with perfectly smooth steps and no block signature.
func (d *Detector) classifyBandingCause(tiles []TileReport) {
	for i := range tiles {
		t := &tiles[i]
		if t.Label != LabelBandingComp {
			continue
		}
		// Tone/grade posterization: wide, stable steps with no codec block
		// signature. Compression banding co-occurs with block boundary energy.
		toneLike := t.BandingGap >= d.cfg.BandingMinRun &&
			t.BlockRatio < 1.15 && t.BlockBoundary < d.cfg.BlockBoundaryThr*0.7
		if t.BlockRatio >= d.cfg.BlockRatioThr*0.9 ||
			t.BlockBoundary >= d.cfg.BlockBoundaryThr*0.7 {
			t.Label = LabelBandingComp
		} else if toneLike {
			t.Label = LabelBandingTone
		}
	}
}

func (d *Detector) applyPersistence(t TileReport) TileReport {
	id := t.TY*d.cols + t.TX
	hist := append(d.history[id], t.Label)
	if len(hist) > d.cfg.PersistFrames+1 {
		hist = hist[len(hist)-(d.cfg.PersistFrames+1):]
	}
	d.history[id] = hist
	if isArtifact(t.Label) {
		count := 0
		for _, l := range hist {
			if l == t.Label {
				count++
			}
		}
		t.Persistent = count >= d.cfg.PersistFrames
	}
	return t
}

func (d *Detector) frameScore(tiles []TileReport) float64 {
	score := 0.0
	for _, t := range tiles {
		if !isArtifact(t.Label) {
			continue
		}
		w := t.Score * t.Confidence
		if t.Persistent {
			w *= 1.25
		}
		score += w
	}
	return score
}

func isArtifact(label string) bool {
	return label == LabelBlocking || label == LabelRinging ||
		label == LabelBandingComp || label == LabelBandingTone
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func (ft frameFeatures) width() int { return ft.widthPx }
