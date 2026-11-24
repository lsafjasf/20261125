package vdetect

import (
	"math"
	"testing"
)

func gameEffectFrame(frame int) *Frame {
	const w, h = 640, 360
	f := &Frame{W: w, H: h, Y: make([]byte, w*h), R: make([]byte, w*h), G: make([]byte, w*h), B: make([]byte, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			v := byte(18 + 12*y/h)
			f.Y[i], f.R[i], f.G[i], f.B[i] = v, v, v+6, v+18
		}
	}
	for k := 0; k < 90; k++ {
		angle := float64(k) * 2.399963
		radius := float64(18 + frame*9 + (k*7)%280)
		x := int(320 + math.Cos(angle)*radius)
		y := int(180 + math.Sin(angle)*radius*0.62)
		pr := 2 + (k % 5)
		rv := byte(255)
		gv := byte(90 + (k*17)%140)
		bv := byte(30 + (k*29)%80)
		if k%7 == 0 {
			gv, bv = 230, 220
		}
		for yy := y - pr; yy <= y+pr; yy++ {
			for xx := x - pr; xx <= x+pr; xx++ {
				if xx < 0 || yy < 0 || xx >= w || yy >= h || (xx-x)*(xx-x)+(yy-y)*(yy-y) > pr*pr {
					continue
				}
				i := yy*w + xx
				f.R[i], f.G[i], f.B[i] = rv, gv, bv
				f.Y[i] = byte((19595*int(rv) + 38470*int(gv) + 7471*int(bv)) >> 16)
			}
		}
	}
	if frame%4 < 2 {
		r := 40 + frame*18
		for y := 180 - r; y < 180+r; y++ {
			for x := 320 - r; x < 320+r; x++ {
				if x < 0 || y < 0 || x >= w || y >= h {
					continue
				}
				d2 := (x-320)*(x-320) + (y-180)*(y-180)
				if d2 > r*r {
					continue
				}
				i := y*w + x
				f.R[i], f.G[i], f.B[i] = 255, 235, 170
				f.Y[i] = 235
			}
		}
	}
	return f
}

func TestGameParticleEffectsNoStructuralFalsePositives(t *testing.T) {
	frames := make([]*Frame, 0, 14)
	for i := 0; i < 14; i++ {
		frames = append(frames, gameEffectFrame(i))
	}
	rep := analyzeFrames(frames)
	if rep.Summary.TearFrames != 0 || rep.Summary.BlockFrames != 0 || rep.Summary.FieldOrderFrames != 0 {
		t.Fatalf("particle effects produced structural findings: %+v", rep.Summary)
	}
	if rep.Summary.EffectFrames == 0 {
		t.Fatalf("expected particle/game-effect regions to be retained")
	}
}
