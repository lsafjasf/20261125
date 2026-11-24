package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func main() {
	var (
		input     = flag.String("input", "", "video file / PNG directory / raw I420 .yuv")
		ref       = flag.String("ref", "", "optional reference (PNG dir or .yuv) for motion masking")
		outDir    = flag.String("out", "report_out", "output directory")
		width     = flag.Int("width", 0, "frame width (required for raw .yuv)")
		height    = flag.Int("height", 0, "frame height (required for raw .yuv)")
		fps       = flag.Float64("fps", 25, "frame rate for timestamps / raw input")
		maxFrames = flag.Int("max-frames", 0, "limit analyzed frames (0 = all)")
		topN      = flag.Int("top", 6, "number of hot frames to render")
		zoom      = flag.Int("zoom", 6, "nearest-neighbor zoom factor for crops")
		debug     = flag.Bool("debug", false, "print tile feature distribution and exit")
	)
	flag.Parse()
	if *input == "" {
		usage()
		os.Exit(2)
	}
	if err := run(*input, *ref, *outDir, *width, *height, *fps, *maxFrames, *topN, *zoom, *debug); err != nil {
		fmt.Fprintln(os.Stderr, "artifactdiag:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: artifactdiag -input <video|pngdir|file.yuv> [-ref ...] -out dir")
	flag.PrintDefaults()
}

func run(input, ref, outDir string, w, h int, fps float64, maxFrames, topN, zoomScale int, debug bool) error {
	cfg := DefaultConfig()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	var meta *VideoMeta
	dist, err := loadInput(input, outDir, w, h, fps, &meta)
	if err != nil {
		return err
	}
	refs, err := loadRef(ref, outDir, w, h, fps)
	if err != nil {
		return err
	}
	if debug {
		debugDump(dist, refs, cfg, 10)
		return nil
	}
	if maxFrames > 0 && maxFrames < len(dist) {
		dist = dist[:maxFrames]
		if refs != nil && len(refs) > maxFrames {
			refs = refs[:maxFrames]
		}
	}
	if len(dist) < 2 {
		return fmt.Errorf("need at least 2 frames, got %d", len(dist))
	}
	if dist[0].W%cfg.TileSize != 0 || dist[0].H%cfg.TileSize != 0 {
		return fmt.Errorf("frame size %dx%d must be divisible by tile size %d",
			dist[0].W, dist[0].H, cfg.TileSize)
	}

	keyframes := map[int]bool{}
	if meta != nil {
		if keys, err := probeKeyframes(input); err == nil {
			for _, k := range keys {
				keyframes[k] = true
			}
			meta.Keyframes = keys
		}
	}

	det := NewDetector(cfg, dist[0].W, dist[0].H)
	reports := make([]FrameReport, 0, len(dist))
	for i, fr := range dist {
		var rf, pf *Frame
		var rpf *Frame
		if refs != nil && i < len(refs) {
			rf = refs[i]
			if i > 0 {
				rpf = refs[i-1]
			}
		}
		if i > 0 {
			pf = dist[i-1]
		}
		reports = append(reports, det.Analyze(fr, rf, rpf, pf))
	}

	hot := pickHotFrames(reports, topN)
	hotOut := renderHotAssets(dist, reports, hot, keyframes, outDir, cfg.TileSize, zoomScale)

	report := buildReport(input, ref, meta, cfg, reports, keyframes, hotOut,
		time.Now().UTC().Format(time.RFC3339))
	if err := writeJSON(filepath.Join(outDir, "report.json"), report); err != nil {
		return err
	}
	if err := writeCSV(filepath.Join(outDir, "frame_scores.csv"), reports); err != nil {
		return err
	}
	if err := writeViewer(filepath.Join(outDir, "index.html"), report); err != nil {
		return err
	}
	printSummary(report)
	return nil
}

func loadInput(input, outDir string, w, h int, fps float64, meta **VideoMeta) ([]*Frame, error) {
	st, err := os.Stat(input)
	if err != nil {
		return nil, err
	}
	switch {
	case st.IsDir():
		return loadPNGSequence(input, fps)
	case filepath.Ext(input) == ".yuv":
		if w == 0 || h == 0 {
			return nil, fmt.Errorf("-width and -height required for raw YUV input")
		}
		return loadRawYUV(input, w, h, fps)
	default:
		m, err := probeMeta(input)
		if err != nil {
			return nil, err
		}
		*meta = m
		if fps == 25 && m.FrameRate > 0 {
			fps = m.FrameRate
		}
		cache := filepath.Join(outDir, "cache_png")
		return extractFrames(input, cache, fps)
	}
}

func loadRef(ref, outDir string, w, h int, fps float64) ([]*Frame, error) {
	if ref == "" {
		return nil, nil
	}
	st, err := os.Stat(ref)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return loadPNGSequence(ref, fps)
	}
	if filepath.Ext(ref) == ".yuv" {
		if w == 0 || h == 0 {
			return nil, fmt.Errorf("-width and -height required for raw YUV ref")
		}
		return loadRawYUV(ref, w, h, fps)
	}
	raw := filepath.Join(outDir, "cache_ref", "ref.yuv")
	if err := extractRawYUV(ref, raw); err != nil {
		return nil, err
	}
	m, err := probeMeta(ref)
	if err != nil || m.Width == 0 {
		return nil, fmt.Errorf("cannot determine reference dimensions")
	}
	return loadRawYUV(raw, m.Width, m.Height, fps)
}

func pickHotFrames(reports []FrameReport, n int) []FrameReport {
	sorted := make([]FrameReport, len(reports))
	copy(sorted, reports)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })
	if n > len(sorted) {
		n = len(sorted)
	}
	picked := sorted[:n]
	sort.Slice(picked, func(i, j int) bool { return picked[i].Index < picked[j].Index })
	return picked
}

func renderHotAssets(dist []*Frame, reports []FrameReport, hot []FrameReport,
	keyframes map[int]bool, outDir string, tileSize, zoomScale int) []HotFrame {
	reportByIndex := map[int]FrameReport{}
	for _, r := range reports {
		reportByIndex[r.Index] = r
	}
	var out []HotFrame
	for rank, hr := range hot {
		fr := dist[hr.Index]
		overlayName := fmt.Sprintf("frame_%04d_overlay.png", hr.Index)
		overlay := overlayImage(fr, hr.Tiles, hr.Hotspots, true)
		if err := writePNG(filepath.Join(outDir, overlayName), overlay); err != nil {
			continue
		}
		hf := HotFrame{
			Rank: rank + 1, FrameIndex: hr.Index, PTS: hr.PTS,
			Score: hr.Score, IsKeyframe: keyframes[hr.Index],
			OverlayPNG: overlayName, LabelCounts: hr.Counts,
		}
		zCount := 0
		for _, region := range hr.Hotspots {
			if zCount >= 4 || !isArtifact(region.Label) {
				continue
			}
			name := fmt.Sprintf("frame_%04d_zoom_%d_%s.png",
				hr.Index, zCount, region.Label)
			zi := zoomImage(fr, region, tileSize, zoomScale, true)
			if err := writePNG(filepath.Join(outDir, name), zi); err != nil {
				continue
			}
			hf.Zooms = append(hf.Zooms, Zoom{
				Label: region.Label, Region: region, ZoomPNG: name})
			zCount++
		}
		out = append(out, hf)
	}
	return out
}

func printSummary(r Report) {
	fmt.Printf("Analyzed %d frames (%d tiles/frame). Dominant artifact: %s\n",
		r.Summary.FramesAnalyzed, r.Summary.TilesPerFrame, r.Summary.WorstLabel)
	for _, label := range []string{LabelBlocking, LabelRinging,
		LabelBandingComp, LabelBandingTone} {
		fmt.Printf("  %-24s tiles=%d frames=%d\n", label,
			r.Summary.LabelTiles[label], r.Summary.LabelFrames[label])
	}
	fmt.Printf("Hot frames rendered: %d. Output: %s\n", len(r.HotFrames), r.Input)
}
