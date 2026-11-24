package main

import "math"

// Artifact labels. Clean tiles and genuine content edges are reported too,
// so real edges are never silently counted as compression damage.
const (
	LabelClean       = "clean"
	LabelRealEdge    = "real_edge"
	LabelTextEdge    = "text_edge"
	LabelMotion      = "motion"
	LabelBlocking    = "blocking_artifact"
	LabelRinging     = "ringing_artifact"
	LabelBandingComp = "banding_compression"
	LabelBandingTone = "banding_tone_posterization"
)

// Config holds all tunable detection thresholds (luma is 0..255).
type Config struct {
	TileSize           int
	DarkLumaThr        int
	EdgeMagThr         int
	BlockRatioThr      float64
	BlockBoundaryThr   float64
	BlockBoundaryNoRef float64
	BlockInteriorMax   float64
	BlockRefSmooth     float64
	RingingThr         float64
	RingingEchoRatio   float64
	BandingMaxLevels   int
	BandingMinGap      int
	BandingMinRun      int
	MotionMeanThr      float64
	MotionPixThr       float64
	MotionFracThr      float64
	PersistFrames      int
}

func DefaultConfig() Config {
	return Config{
		TileSize:           16,
		DarkLumaThr:        80,
		EdgeMagThr:         42,
		BlockRatioThr:      1.6,
		BlockBoundaryThr:   2.5,
		BlockBoundaryNoRef: 4.5,
		BlockInteriorMax:   3.0,
		BlockRefSmooth:     1.2,
		RingingThr:         30,
		RingingEchoRatio:   0.08,
		BandingMaxLevels:   18,
		BandingMinGap:      2,
		BandingMinRun:      4,
		MotionMeanThr:      3.0,
		MotionPixThr:       12,
		MotionFracThr:      0.18,
		PersistFrames:      2,
	}
}

// TileReport is the diagnosis for one 16x16 (macroblock-sized) tile.
type TileReport struct {
	TX            int     `json:"tx"`
	TY            int     `json:"ty"`
	X             int     `json:"x"`
	Y             int     `json:"y"`
	Size          int     `json:"size"`
	Label         string  `json:"label"`
	MeanLuma      float64 `json:"mean_luma"`
	EdgeEnergy    float64 `json:"edge_energy"`
	BlockBoundary float64 `json:"block_boundary"`
	BlockInterior float64 `json:"block_interior"`
	BlockRatio    float64 `json:"block_ratio"`
	Ringing       float64 `json:"ringing"`
	BandingGap    int     `json:"banding_gap"`
	BandingLevels int     `json:"banding_levels"`
	Motion        float64 `json:"motion"`
	Score         float64 `json:"severity"`
	Confidence    float64 `json:"confidence"`
	Persistent    bool    `json:"persistent"`
}

// FrameReport collects tile diagnoses plus frame-level aggregates.
type FrameReport struct {
	Index    int            `json:"frame_index"`
	PTS      float64        `json:"pts_sec"`
	Score    float64        `json:"artifact_score"`
	Tiles    []TileReport   `json:"tiles"`
	Counts   map[string]int `json:"label_counts"`
	Hotspots []Region       `json:"hotspots"`
}

// Region is a connected cluster of same-label artifact tiles.
type Region struct {
	Label    string  `json:"label"`
	X        int     `json:"x"`
	Y        int     `json:"y"`
	W        int     `json:"w"`
	H        int     `json:"h"`
	TileIDs  []int   `json:"tile_ids"`
	Severity float64 `json:"severity"`
}

type frameFeatures struct {
	widthPx int
	sobel   []float64
	vDiff   []float64
	hDiff   []float64
	resid   []float64
	edgePix []bool
	// refEdge marks edges that already exist in the clean source; echoes around
	// them are codec ringing, while the edges themselves are genuine content.
	refEdge []bool
	// refBoundaryByTile is per-tile mean absolute difference on the 8px grid
	// in the source, to separate codec grid discontinuities from real edges.
	refBoundaryByTile []float64
	refInteriorByTile []float64
}

// Detector keeps temporal history for persistence and motion checks.
type Detector struct {
	cfg        Config
	cols, rows int
	history    [][]string // per tile: recent labels, newest last
}

func NewDetector(cfg Config, w, h int) *Detector {
	cols, rows := w/cfg.TileSize, h/cfg.TileSize
	return &Detector{cfg: cfg, cols: cols, rows: rows,
		history: make([][]string, cols*rows)}
}

// Analyze scores one distorted frame. ref/refPrev are optional clean-source
// frames; prev is the prior distorted frame and may be nil on frame zero.
func (d *Detector) Analyze(dist, ref, refPrev, prev *Frame) FrameReport {
	cfg := d.cfg
	ts := cfg.TileSize
	feat := d.computeFeatures(dist)
	if ref != nil && ref.W == dist.W {
		refFeat := d.computeFeatures(ref)
		feat.refEdge = refFeat.edgePix
		ts := cfg.TileSize
		for tile := 0; tile < d.cols*d.rows; tile++ {
			tx, ty := tile%d.cols, tile/d.cols
			b, i := d.blockEnergy(ref, refFeat, tx*ts, ty*ts)
			feat.refBoundaryByTile = append(feat.refBoundaryByTile, b)
			feat.refInteriorByTile = append(feat.refInteriorByTile, i)
		}
	}
	motion := d.motionMap(dist, ref, prev, refPrev)

	fr := FrameReport{Index: dist.Index, PTS: dist.PTS, Counts: map[string]int{}}
	for ty := 0; ty < d.rows; ty++ {
		for tx := 0; tx < d.cols; tx++ {
			t := d.scoreTile(dist, feat, motion, tx, ty)
			t = d.applyPersistence(t)
			fr.Tiles = append(fr.Tiles, t)
			fr.Counts[t.Label]++
		}
	}
	d.classifyBandingCause(fr.Tiles)
	fr.Score = d.frameScore(fr.Tiles)
	fr.Hotspots = clusterRegions(fr.Tiles, d.cols, d.rows, ts)
	return fr
}

func (d *Detector) computeFeatures(f *Frame) frameFeatures {
	w, h := f.W, f.H
	sobel := make([]float64, w*h)
	vDiff := make([]float64, w*h)
	hDiff := make([]float64, w*h)
	edge := make([]bool, w*h)
	blur := boxBlur(f.Y, w, h, 2)
	resid := make([]float64, w*h)
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			i := y*w + x
			gx := -int(f.Y[i-w-1]) - 2*int(f.Y[i-1]) - int(f.Y[i+w-1]) +
				int(f.Y[i-w+1]) + 2*int(f.Y[i+1]) + int(f.Y[i+w+1])
			gy := -int(f.Y[i-w-1]) - 2*int(f.Y[i-w]) - int(f.Y[i-w+1]) +
				int(f.Y[i+w-1]) + 2*int(f.Y[i+w]) + int(f.Y[i+w+1])
			mag := math.Abs(float64(gx)) + math.Abs(float64(gy))
			sobel[i] = mag
			edge[i] = mag >= float64(d.cfg.EdgeMagThr)*4
			vDiff[i] = math.Abs(float64(f.Y[i]) - float64(f.Y[i-1]))
			hDiff[i] = math.Abs(float64(f.Y[i]) - float64(f.Y[i-w]))
			resid[i] = float64(f.Y[i]) - blur[i]
		}
	}
	return frameFeatures{widthPx: w, sobel: sobel, vDiff: vDiff, hDiff: hDiff,
		resid: resid, edgePix: edge}
}

func boxBlur(y []byte, w, h, r int) []float64 {
	out := make([]float64, w*h)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			sum, n := 0, 0
			for dy := -r; dy <= r; dy++ {
				yy := row + dy
				if yy < 0 || yy >= h {
					continue
				}
				for dx := -r; dx <= r; dx++ {
					xx := col + dx
					if xx < 0 || xx >= w {
						continue
					}
					sum += int(y[yy*w+xx])
					n++
				}
			}
			out[row*w+col] = float64(sum) / float64(n)
		}
	}
	return out
}

// motionMap estimates per-tile motion from temporal gradients. When a clean
// reference exists, its frame-to-frame differences isolate genuine movement
// without range-offset or codec-noise contamination; otherwise the distorted
// sequence's own temporal gradient is used.
func (d *Detector) motionMap(dist, ref, prev, refPrev *Frame) []float64 {
	w := dist.W
	motion := make([]float64, d.cols*d.rows)
	var cur, prior *Frame
	switch {
	case ref != nil && refPrev != nil && refPrev.W == w:
		cur, prior = ref, refPrev
	case prev != nil && prev.W == w:
		cur, prior = dist, prev
	default:
		return motion
	}
	ts := d.cfg.TileSize
	for ty := 0; ty < d.rows; ty++ {
		for tx := 0; tx < d.cols; tx++ {
			strong, sum := 0, 0.0
			for yy := ty * ts; yy < (ty+1)*ts; yy++ {
				for xx := tx * ts; xx < (tx+1)*ts; xx++ {
					i := yy*w + xx
					dv := math.Abs(float64(cur.Y[i]) - float64(prior.Y[i]))
					sum += dv
					if dv >= d.cfg.MotionPixThr {
						strong++
					}
				}
			}
			mean := sum / float64(ts*ts)
			frac := float64(strong) / float64(ts*ts)
			// Require both elevated average change and a coherent moving area,
			// which sparse temporal codec flicker cannot satisfy.
			if mean >= d.cfg.MotionMeanThr && frac >= d.cfg.MotionFracThr {
				motion[ty*d.cols+tx] = mean
			}
		}
	}
	// Dilate one tile so the exposed/covered neighborhood of a moving object
	// is also excluded from artifact checks.
	dilated := make([]float64, len(motion))
	for ty := 0; ty < d.rows; ty++ {
		for tx := 0; tx < d.cols; tx++ {
			best := 0.0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					ny, nx := ty+dy, tx+dx
					if ny < 0 || ny >= d.rows || nx < 0 || nx >= d.cols {
						continue
					}
					if v := motion[ny*d.cols+nx]; v > best {
						best = v
					}
				}
			}
			dilated[ty*d.cols+tx] = best
		}
	}
	return dilated
}
