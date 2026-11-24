package main

import "capmatrix/display"

// materialPixel is one source swatch: linear sRGB-relative RGB, where 1.0 is
// the 100-nit reference white and values above 1.0 are HDR highlights.
type materialPixel struct {
	Name string
	RGB  [3]float64
}

func sourceMaterial() []materialPixel {
	return []materialPixel{
		{"deep-shadow", [3]float64{0.02, 0.02, 0.02}},
		{"mid-gray", [3]float64{0.18, 0.18, 0.18}},
		{"sdr-white", [3]float64{1.0, 1.0, 1.0}},
		{"hdr-1000n", [3]float64{10.0, 10.0, 10.0}},
		{"saturated-red", [3]float64{0.8, 0.05, 0.02}},
		{"cyan-sky", [3]float64{0.1, 0.5, 0.7}},
	}
}

// simulatedDevices returns the three target panels plus deliberately illegal
// or incomplete rows used to exercise the documented fallback path.
func simulatedDevices() []display.Display {
	srgb, _ := display.StandardGamut("sRGB")
	p3, _ := display.StandardGamut("P3-D65")
	rec2020, _ := display.StandardGamut("Rec2020")
	return []display.Display{
		{ID: "A-hdr-oled", PeakNits: 1000, BlackNits: 0.05, Gamut: p3, Bits: 10},
		{ID: "B-sdr-lcd", PeakNits: 300, BlackNits: 0.3, Gamut: srgb, Bits: 8},
		{ID: "C-cheap-laptop", PeakNits: 180, BlackNits: 1.2, Gamut: srgb, Bits: 6},
		{ID: "X-negative-peak", PeakNits: -50, BlackNits: 0.1, Gamut: p3, Bits: 10},
		{ID: "Y-incomplete-gamut", PeakNits: 800, BlackNits: 0.05, Gamut: display.Primaries{}, Bits: 10},
		{ID: "Z-implausible-2020", PeakNits: 600, BlackNits: 900, Gamut: rec2020, Bits: 12},
	}
}
