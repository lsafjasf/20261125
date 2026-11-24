package display

import "math"

// Reference white of the source material in nits. Source pixels are linear
// sRGB-relative values where 1.0 corresponds to this luminance.
const ReferenceWhiteNits = 100.0

// PQ constants from SMPTE ST.2084.
const (
	pqM1   = 2610.0 / 16384.0
	pqM2   = 2523.0 / 4096.0 * 128.0
	pqC1   = 3424.0 / 4096.0
	pqC2   = 2413.0 / 4096.0 * 32.0
	pqC3   = 2392.0 / 4096.0 * 32.0
	pqPeak = 10000.0
)

// PQEncode maps absolute linear luminance in nits to the ST.2084 signal.
func PQEncode(nits float64) float64 {
	if nits <= 0 {
		return 0
	}
	l := math.Min(nits/pqPeak, 1.0)
	lm := math.Pow(l, pqM1)
	num := pqC1 + pqC2*lm
	den := 1 + pqC3*lm
	return math.Pow(num/den, pqM2)
}

// PQDecode is the inverse of PQEncode and exists to make the tier verifiable
// via round-trip tests.
func PQDecode(signal float64) float64 {
	if signal <= 0 {
		return 0
	}
	sp := math.Pow(math.Min(signal, 1.0), 1.0/pqM2)
	lm := math.Max((sp-pqC1)/(pqC2-pqC3*sp), 0)
	return pqPeak * math.Pow(lm, 1.0/pqM1)
}

// SRGBEncode applies the IEC 61966-2-1 transfer function to a linear value
// normalized to [0,1].
func SRGBEncode(lin float64) float64 {
	lin = math.Max(0, math.Min(lin, 1.0))
	if lin <= 0.0031308 {
		return 12.92 * lin
	}
	return 1.055*math.Pow(lin, 1.0/2.4) - 0.055
}

// SRGBDecode is the inverse of SRGBEncode.
func SRGBDecode(signal float64) float64 {
	if signal <= 0.04045 {
		return signal / 12.92
	}
	return math.Pow((signal+0.055)/1.055, 2.4)
}

// ToneMap compresses one linear, sRGB-relative channel to the display's
// reachable range. Below the knee (reference white for HDR, panel peak for
// SDR) values pass through untouched; above it an exponential roll-off pulls
// HDR highlights toward the panel peak without hard clipping. The panel black
// floor is then lifted so true black renders at the measured black level.
func ToneMap(v float64, p Profile) float64 {
	peakRel := p.PeakNits / ReferenceWhiteNits
	kneeRel := p.KneeNits / ReferenceWhiteNits
	blackRel := p.BlackNits / ReferenceWhiteNits

	m := v
	switch {
	case v <= 0:
		m = 0
	case v <= kneeRel:
		m = v
	case peakRel > kneeRel:
		headroom := peakRel - kneeRel
		m = kneeRel + headroom*(1-math.Exp(-(v-kneeRel)/headroom))
	default:
		m = peakRel
	}
	// Lift black while keeping peak pinned to the panel peak.
	return blackRel + m*(1-blackRel/peakRel)
}

// ConvertPixel runs the full pipeline for one source pixel (linear sRGB
// relative RGB) and returns both normalized signal values and quantized code
// values in the resolved tier's bit depth.
func ConvertPixel(in [3]float64, p Profile) (signal [3]float64, codes [3]int) {
	mapped := p.GamutMatrix.mulVec(in)
	peakRel := p.PeakNits / ReferenceWhiteNits
	for i, v := range mapped {
		v = math.Max(0, v) // clip out-of-gamut negative excursions
		v = ToneMap(v, p)
		var enc float64
		switch p.Transfer {
		case TransferPQ:
			enc = PQEncode(v * ReferenceWhiteNits)
		default:
			enc = SRGBEncode(v / peakRel)
		}
		signal[i] = enc
		codes[i] = quantize(enc, p.CodeMax)
	}
	return signal, codes
}

func quantize(signal float64, codeMax int) int {
	c := int(math.Round(math.Max(0, math.Min(signal, 1.0)) * float64(codeMax)))
	if c > codeMax {
		c = codeMax
	}
	return c
}
