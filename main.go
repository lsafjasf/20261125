package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strconv"
)

const samplesN = 257

type sample struct {
	t            float64
	orig         Lab
	hard         Lab
	soft         Lab
	origInGamut  bool
	hardInGamut  bool
	softInGamut  bool
	hueOrig      float64
	hueHard      float64
	hueSoft      float64
	hardDE       float64
	softDE       float64
	hardHueStep  float64
	softHueStep  float64
	hardHueShift float64
	softHueShift float64
}

type trajectoryResult struct {
	name             string
	s                []sample
	maxHardDE        float64
	maxSoftDE        float64
	maxHardHu        float64
	maxSoftHu        float64
	maxHardShift     float64
	maxSoftShift     float64
	boundaryHardJump float64
	boundarySoftJump float64
	outCount         int
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func lerpLab(a, b Lab, t float64) Lab {
	return Lab{lerp(a.L, b.L, t), lerp(a.A, b.A, t), lerp(a.B, b.B, t)}
}

// runTrajectory traces a constant-hue Lab line from paper white (D65) to
// a highly saturated primary, mapping each sample both with naive device
// clipping and with hue-preserving soft compression.
func runTrajectory(name string, start, end Lab, dst Gamut) trajectoryResult {
	res := trajectoryResult{name: name, s: make([]sample, samplesN)}

	for i := 0; i < samplesN; i++ {
		t := float64(i) / (samplesN - 1)
		orig := lerpLab(start, end, t)
		hard := dst.clipLab(orig)
		soft := dst.softCompressLab(orig)

		smp := sample{
			t:           t,
			orig:        orig,
			hard:        hard,
			soft:        soft,
			origInGamut: dst.containsLCh(labToLCh(orig)),
			hardInGamut: dst.containsLCh(labToLCh(hard)),
			softInGamut: dst.containsLCh(labToLCh(soft)),
			hueOrig:     labToLCh(orig).H,
			hueHard:     labToLCh(hard).H,
			hueSoft:     labToLCh(soft).H,
			hardDE:      ciede2000(orig, hard),
			softDE:      ciede2000(orig, soft),
		}
		if !smp.origInGamut {
			res.outCount++
		}
		res.maxHardDE = math.Max(res.maxHardDE, smp.hardDE)
		res.maxSoftDE = math.Max(res.maxSoftDE, smp.softDE)
		res.s[i] = smp
	}

	for i := range res.s {
		// Absolute hue shift vs the original constant-hue trajectory.
		// Near the achromatic axis hue is undefined; skip those samples.
		if labToLCh(res.s[i].orig).C > 0.05 {
			res.s[i].hardHueShift = hueDistance(res.s[i].hueOrig, res.s[i].hueHard)
			res.s[i].softHueShift = hueDistance(res.s[i].hueOrig, res.s[i].hueSoft)
			res.maxHardShift = math.Max(res.maxHardShift, res.s[i].hardHueShift)
			res.maxSoftShift = math.Max(res.maxSoftShift, res.s[i].softHueShift)
		}
		if i == 0 {
			continue
		}
		// Continuity: hue jump between consecutive mapped samples,
		// counted only where both ends carry a defined hue.
		if labToLCh(res.s[i].hard).C > 0.05 && labToLCh(res.s[i-1].hard).C > 0.05 {
			res.s[i].hardHueStep = hueDistance(res.s[i-1].hueHard, res.s[i].hueHard)
			res.maxHardHu = math.Max(res.maxHardHu, res.s[i].hardHueStep)
		}
		if labToLCh(res.s[i].soft).C > 0.05 && labToLCh(res.s[i-1].soft).C > 0.05 {
			res.s[i].softHueStep = hueDistance(res.s[i-1].hueSoft, res.s[i].hueSoft)
			res.maxSoftHu = math.Max(res.maxSoftHu, res.s[i].softHueStep)
		}
	}
	// Discontinuity at the gamut boundary: compare the first out-of-gamut
	// sample whose hue is defined against the preceding defined-hue sample.
	for i := 1; i < len(res.s); i++ {
		if !res.s[i].origInGamut &&
			labToLCh(res.s[i].hard).C > 0.05 && labToLCh(res.s[i-1].hard).C > 0.05 {
			res.boundaryHardJump = hueDistance(res.s[i-1].hueHard, res.s[i].hueHard)
			break
		}
	}
	for i := 1; i < len(res.s); i++ {
		if !res.s[i].origInGamut &&
			labToLCh(res.s[i].soft).C > 0.05 && labToLCh(res.s[i-1].soft).C > 0.05 {
			res.boundarySoftJump = hueDistance(res.s[i-1].hueSoft, res.s[i].hueSoft)
			break
		}
	}
	return res
}

// hueDistance returns the shortest circular angular distance in degrees.
func hueDistance(a, b float64) float64 {
	d := math.Mod(b-a+540, 360) - 180
	return math.Abs(d)
}

func writeTrajectoryCSV(path string, r trajectoryResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	header := []string{
		"t", "orig_L", "orig_a", "orig_b", "orig_hue", "orig_in_gamut",
		"hard_hue", "hard_in_gamut", "hard_deltaE00", "hard_hue_step_deg", "hard_hue_shift_deg",
		"soft_hue", "soft_in_gamut", "soft_deltaE00", "soft_hue_step_deg", "soft_hue_shift_deg",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, s := range r.s {
		row := []string{
			strconv.FormatFloat(s.t, 'f', 5, 64),
			fv(s.orig.L), fv(s.orig.A), fv(s.orig.B), fv(s.hueOrig),
			strconv.FormatBool(s.origInGamut),
			fv(s.hueHard), strconv.FormatBool(s.hardInGamut),
			fv(s.hardDE), fv(s.hardHueStep), fv(s.hardHueShift),
			fv(s.hueSoft), strconv.FormatBool(s.softInGamut),
			fv(s.softDE), fv(s.softHueStep), fv(s.softHueShift),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func fv(v float64) string { return strconv.FormatFloat(v, 'f', 5, 64) }

func writeBoundaryCSV(path string, src, dst Gamut, lightness float64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"hue_deg", "src_boundary_C", "dst_boundary_C"}); err != nil {
		return err
	}
	for h := 0.0; h <= 360.0; h += 2 {
		row := []string{
			fv(h),
			fv(src.maxChroma(lightness, h)),
			fv(dst.maxChroma(lightness, h)),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func writeMetrics(path string, red, blue, cyan trajectoryResult, src, dst Gamut) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "# Gamut boundary tracking and soft compression report\n")
	fmt.Fprintf(f, "source_gamut=%s target_gamut=%s knee=%.2f samples_per_track=%d\n\n",
		src.name, dst.name, softKnee, samplesN)

	write := func(title string, r trajectoryResult) {
		fmt.Fprintf(f, "[%s] paper-white -> saturated %s\n", title, r.name)
		fmt.Fprintf(f, "  out_of_gamut_samples       : %d / %d\n", r.outCount, len(r.s))
		fmt.Fprintf(f, "  hard_clip_max_deltaE00      : %.4f\n", r.maxHardDE)
		fmt.Fprintf(f, "  soft_compress_max_deltaE00  : %.4f\n", r.maxSoftDE)
		fmt.Fprintf(f, "  hard_clip_max_hue_step_deg  : %.4f\n", r.maxHardHu)
		fmt.Fprintf(f, "  soft_compress_max_hue_step  : %.6f deg\n", r.maxSoftHu)
		fmt.Fprintf(f, "  hard_clip_max_hue_shift_deg : %.4f\n", r.maxHardShift)
		fmt.Fprintf(f, "  soft_compress_max_hue_shift : %.6f deg\n", r.maxSoftShift)
		fmt.Fprintf(f, "  hard_clip_hue_jump_at_edge  : %.4f deg\n", r.boundaryHardJump)
		fmt.Fprintf(f, "  soft_compress_hue_jump_edge : %.6f deg\n", r.boundarySoftJump)
		hardBad, softBad := 0, 0
		for _, s := range r.s {
			if !s.hardInGamut {
				hardBad++
			}
			if !s.softInGamut {
				softBad++
			}
		}
		fmt.Fprintf(f, "  hard_results_outside_gamut  : %d\n", hardBad)
		fmt.Fprintf(f, "  soft_results_outside_gamut  : %d\n\n", softBad)
	}
	write("RED TRACK", red)
	write("BLUE TRACK", blue)
	write("CYAN TRACK", cyan)
	return nil
}
