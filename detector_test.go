package main

import "testing"

func constFrame(w, h int, fill byte) *Frame {
	y := make([]byte, w*h)
	for i := range y {
		y[i] = fill
	}
	return &Frame{Y: y, W: w, H: h}
}

// TestMotionTileNotArtifact ensures fast movement is labeled motion, never
// compression damage, even when pixel deltas are large.
func TestMotionTileNotArtifact(t *testing.T) {
	cfg := DefaultConfig()
	d := NewDetector(cfg, 32, 32)
	cur := constFrame(32, 32, 40)
	prev := constFrame(32, 32, 40)
	// Shift a bright 12x12 block by several pixels between frames.
	for yy := 4; yy < 16; yy++ {
		for xx := 4; xx < 16; xx++ {
			cur.Y[yy*32+xx] = 230
		}
	}
	for yy := 4; yy < 16; yy++ {
		for xx := 10; xx < 22; xx++ {
			prev.Y[yy*32+xx] = 230
		}
	}
	fr := d.Analyze(cur, nil, nil, prev)
	motionTiles := 0
	for _, tile := range fr.Tiles {
		if tile.Label == LabelBlocking || tile.Label == LabelRinging ||
			tile.Label == LabelBandingComp || tile.Label == LabelBandingTone {
			t.Fatalf("moving region mislabeled as %s at tile %d,%d",
				tile.Label, tile.TX, tile.TY)
		}
		if tile.Label == LabelMotion {
			motionTiles++
		}
	}
	if motionTiles == 0 {
		t.Fatal("expected at least one motion tile")
	}
}

// TestBlockingBoundaryEnergy checks that 8-pixel grid discontinuities produce
// higher boundary than interior energy on a flat dark frame.
func TestBlockingBoundaryEnergy(t *testing.T) {
	cfg := DefaultConfig()
	d := NewDetector(cfg, 32, 32)
	f := constFrame(32, 32, 30)
	// Inject a step exactly on the x=8 and x=16 codec grid lines.
	for yy := 0; yy < 32; yy++ {
		for xx := 8; xx < 16; xx++ {
			f.Y[yy*32+xx] = 38
		}
		for xx := 16; xx < 24; xx++ {
			f.Y[yy*32+xx] = 24
		}
	}
	feat := d.computeFeatures(f)
	tr := d.scoreTile(f, feat, make([]float64, 4), 1, 1)
	if tr.BlockRatio < cfg.BlockRatioThr {
		t.Fatalf("block ratio %.2f below threshold %.2f", tr.BlockRatio, cfg.BlockRatioThr)
	}
	if tr.Label != LabelBlocking {
		t.Fatalf("expected blocking_artifact, got %s", tr.Label)
	}
}

// TestSmoothFlatIsCleanAndRealEdgeClassified separates clean flat content,
// genuine edges and banding-prone quantized gradients.
func TestSmoothFlatIsCleanAndRealEdgeClassified(t *testing.T) {
	cfg := DefaultConfig()
	d := NewDetector(cfg, 32, 32)

	flat := constFrame(32, 32, 45)
	feat := d.computeFeatures(flat)
	tr := d.scoreTile(flat, feat, make([]float64, 4), 0, 0)
	if tr.Label != LabelClean {
		t.Fatalf("flat frame labeled %s, want clean", tr.Label)
	}

	// One crisp vertical edge in the middle: genuine content edge.
	edge := constFrame(32, 32, 45)
	for yy := 0; yy < 32; yy++ {
		for xx := 13; xx < 32; xx++ {
			edge.Y[yy*32+xx] = 210
		}
	}
	feat = d.computeFeatures(edge)
	tr = d.scoreTile(edge, feat, make([]float64, 4), 0, 0)
	if tr.Label != LabelRealEdge && tr.Label != LabelTextEdge {
		t.Fatalf("crisp edge labeled %s, want real/text edge", tr.Label)
	}
}

// TestBandingDarkQuantizedGradient verifies that a dark gradient collapsed
// into sparse luma levels with empty gaps is flagged as banding.
func TestBandingDarkQuantizedGradient(t *testing.T) {
	cfg := DefaultConfig()
	d := NewDetector(cfg, 32, 32)
	f := constFrame(32, 32, 10)
	// Only four dark levels, each held for 8 rows -> wide empty histogram gaps.
	for yy := 0; yy < 32; yy++ {
		level := 8 + (yy/8)*6
		for xx := 0; xx < 32; xx++ {
			f.Y[yy*32+xx] = byte(level)
		}
	}
	feat := d.computeFeatures(f)
	labels := map[string]bool{}
	for ty := 0; ty < 2; ty++ {
		for tx := 0; tx < 2; tx++ {
			tr := d.scoreTile(f, feat, make([]float64, 4), tx, ty)
			labels[tr.Label] = true
		}
	}
	if !labels[LabelBandingComp] && !labels[LabelBandingTone] {
		t.Fatalf("quantized dark gradient not flagged as banding: %v", labels)
	}
}

// TestRegionClustering merges adjacent same-label tiles into one hotspot.
func TestRegionClustering(t *testing.T) {
	tiles := []TileReport{
		{TX: 0, TY: 0, X: 0, Y: 0, Size: 16, Label: LabelBlocking, Score: 3},
		{TX: 1, TY: 0, X: 16, Y: 0, Size: 16, Label: LabelBlocking, Score: 3},
		{TX: 0, TY: 1, X: 0, Y: 16, Size: 16, Label: LabelRinging, Score: 5},
		{TX: 1, TY: 1, X: 16, Y: 16, Size: 16, Label: LabelClean, Score: 0},
	}
	regions := clusterRegions(tiles, 2, 2, 16)
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions, got %d", len(regions))
	}
	if regions[0].Label != LabelBlocking || len(regions[0].TileIDs) != 2 {
		t.Fatalf("blocking region not merged: %+v", regions[0])
	}
	if regions[0].W != 32 || regions[0].H != 16 {
		t.Fatalf("region bbox wrong: %+v", regions[0])
	}
}
