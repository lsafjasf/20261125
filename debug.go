package main

import (
	"fmt"
	"sort"
)

type tileStat struct {
	id                                int
	motion, boundary, interior, ratio float64
	ringing, meanLuma, edgeEnergy     float64
	gap, levels, edges                int
	label                             string
}

// debugDump prints feature distributions and label counts so thresholds can
// be calibrated against real compressed material. Enabled with -debug.
func debugDump(frames, refs []*Frame, cfg Config, sampleTiles int) {
	d := NewDetector(cfg, frames[0].W, frames[0].H)
	var stats []tileStat
	labelCount := map[string]int{}
	limit := 3
	if len(frames) < limit {
		limit = len(frames)
	}
	for fi := 0; fi < limit; fi++ {
		f := frames[fi]
		feat := d.computeFeatures(f)
		if rf := refOrNil(refs, fi); rf != nil && rf.W == f.W {
			refFeat := d.computeFeatures(rf)
			feat.refEdge = refFeat.edgePix
			ts := cfg.TileSize
			for tile := 0; tile < d.cols*d.rows; tile++ {
				tx, ty := tile%d.cols, tile/d.cols
				b, in := d.blockEnergy(rf, refFeat, tx*ts, ty*ts)
				feat.refBoundaryByTile = append(feat.refBoundaryByTile, b)
				feat.refInteriorByTile = append(feat.refInteriorByTile, in)
			}
		}
		var rpf *Frame
		if fi > 0 && fi-1 < len(refs) {
			rpf = refs[fi-1]
		}
		motion := d.motionMap(f, refOrNil(refs, fi), prevOrNil(frames, fi), rpf)
		for ty := 0; ty < d.rows; ty++ {
			for tx := 0; tx < d.cols; tx++ {
				t := d.scoreTile(f, feat, motion, tx, ty)
				labelCount[t.Label]++
				stats = append(stats, tileStat{
					t.TY*d.cols + t.TX, t.Motion, t.BlockBoundary, t.BlockInterior,
					t.BlockRatio, t.Ringing, t.MeanLuma, t.EdgeEnergy,
					t.BandingGap, t.BandingLevels,
					edgeCount(feat, t.X, t.Y, t.Size), t.Label})
			}
		}
	}
	printTop := func(name string, less func(i, j int) bool) {
		sort.Slice(stats, less)
		fmt.Printf("== top %s ==\n", name)
		for i := 0; i < sampleTiles && i < len(stats); i++ {
			s := stats[i]
			fmt.Printf("tile=%3d %-22s motion=%6.2f bound=%6.2f inter=%5.2f ratio=%5.2f ring=%6.2f luma=%6.1f gap=%d lvl=%d\n",
				s.id, s.label, s.motion, s.boundary, s.interior, s.ratio,
				s.ringing, s.meanLuma, s.gap, s.levels)
		}
	}
	printTop("motion", func(i, j int) bool { return stats[i].motion > stats[j].motion })
	printTop("blockratio", func(i, j int) bool { return stats[i].ratio > stats[j].ratio })
	printTop("ringing", func(i, j int) bool { return stats[i].ringing > stats[j].ringing })
	printTop("banding-gap", func(i, j int) bool { return stats[i].gap > stats[j].gap })
	fmt.Printf("== labels (first %d frames) ==\n%v\n", limit, labelCount)
}

func edgeCount(feat frameFeatures, x0, y0, ts int) int {
	c := 0
	for yy := y0; yy < y0+ts; yy++ {
		for xx := x0; xx < x0+ts; xx++ {
			if feat.edgePix[yy*feat.widthPx+xx] {
				c++
			}
		}
	}
	return c
}

func refOrNil(refs []*Frame, i int) *Frame {
	if i < len(refs) {
		return refs[i]
	}
	return nil
}

func prevOrNil(frames []*Frame, i int) *Frame {
	if i > 0 {
		return frames[i-1]
	}
	return nil
}
