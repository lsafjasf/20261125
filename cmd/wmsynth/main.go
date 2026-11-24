// wmsynth generates a controlled test recording containing three labelled
// phenomena: a repeated short watermark fragment, a natural echo chain and
// codec-style block ringing. Ground-truth sample positions are written to a
// sidecar JSON so detector conclusions can be checked end to end.
package main

import (
	"encoding/json"
	"flag"
	"math"
	"math/rand/v2"
	"os"

	"watermarkaudit/internal/wavio"
)

const sr = 24000

type groundTruth struct {
	SampleRate int     `json:"sample_rate"`
	Watermark  []span  `json:"watermark_spans"`
	EchoLags   []int   `json:"echo_tap_lags"`
	RingBlock  int     `json:"ringing_block"`
	WMFreqLo   float64 `json:"wm_band_lo_hz"`
	WMFreqHi   float64 `json:"wm_band_hi_hz"`
}

type span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func main() {
	out := flag.String("out", "testdata/synth.wav", "output WAV path")
	truth := flag.String("truth", "testdata/synth.truth.json", "ground-truth JSON path")
	secs := flag.Float64("secs", 12, "duration in seconds")
	flag.Parse()

	rng := rand.New(rand.NewPCG(42, 99))
	n := int(*secs * sr)
	x := make([]float64, n)

	// 1) Speech-like base: three time-varying formant tones plus low noise,
	// gated by a slow syllable-rate amplitude envelope.
	formants := []float64{700, 1400, 2600}
	phase := make([]float64, 3)
	for i := range x {
		t := float64(i) / sr
		env := 0.55 + 0.45*math.Sin(2*math.Pi*3.1*t)*math.Sin(2*math.Pi*0.7*t+1.3)
		v := 0.0
		for k, fc := range formants {
			phase[k] += 2 * math.Pi * (fc + 25*math.Sin(2*math.Pi*0.9*t)) / sr
			v += 0.33 * math.Sin(phase[k]) / float64(k+1)
		}
		x[i] = env*v*0.5 + 0.02*(rng.Float64()*2-1)
	}

	// 2) Repeated watermark: one fixed 1024-sample, 1.2-3.2 kHz noise
	// fragment inserted at five irregular times with stable gain.
	wmLen := 1024
	wm := bandLimitedNoise(wmLen, 1200, 3200, rng)
	insertAt := []int{int(1.2 * sr), int(3.4 * sr), int(5.1 * sr), int(7.9 * sr), int(10.3 * sr)}
	gt := groundTruth{SampleRate: sr, EchoLags: []int{3000, 6000, 9000}, RingBlock: 256, WMFreqLo: 1200, WMFreqHi: 3200}
	for _, p := range insertAt {
		for k := 0; k < wmLen && p+k < n; k++ {
			x[p+k] += 0.55 * wm[k]
		}
		gt.Watermark = append(gt.Watermark, span{Start: p, End: p + wmLen})
	}

	// 3) Natural echo: global damped delay line (room-like tap train).
	echo := make([]float64, n)
	taps := []struct {
		lag int
		g   float64
	}{{3000, 0.5}, {6000, 0.25}, {9000, 0.12}}
	for _, tp := range taps {
		for i := tp.lag; i < n; i++ {
			echo[i] += tp.g * x[i-tp.lag]
		}
	}
	for i := range x {
		x[i] += echo[i]
	}

	// 4) Codec ringing: per-256-sample-block low-order spectral truncation
	// plus a damped block-boundary tone. No global denoising anywhere.
	x = codecRinging(x, 256, rng)

	// Peak normalise to stay inside 16-bit range (amplitude scaling only;
	// relative structure and gains are untouched).
	peak := 0.0
	for _, v := range x {
		if a := math.Abs(v); a > peak {
			peak = a
		}
	}
	for i := range x {
		x[i] = 0.92 * x[i] / peak
	}

	if err := os.MkdirAll(dirOf(*out), 0o755); err != nil {
		fail(err)
	}
	if err := wavio.WriteMono16(*out, x, sr); err != nil {
		fail(err)
	}
	if err := os.MkdirAll(dirOf(*truth), 0o755); err != nil {
		fail(err)
	}
	j, _ := json.MarshalIndent(gt, "", "  ")
	if err := os.WriteFile(*truth, j, 0o644); err != nil {
		fail(err)
	}
}

func bandLimitedNoise(n int, lo, hi float64, rng *rand.Rand) []float64 {
	// White noise then FFT mask (same primitive the auditor uses).
	v := make([]float64, n)
	for i := range v {
		v[i] = rng.Float64()*2 - 1
	}
	// Cheap time-domain band limit: modulate with carrier and window via a
	// 2-pole style difference filter chain approximated by FFT through dsp.
	return fftMask(v, lo, hi)
}
