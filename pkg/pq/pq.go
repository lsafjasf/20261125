// Package pq implements the SMPTE ST 2084 Perceptual Quantizer (PQ)
// electro-optical transfer function and its inverse.
//
// All functions work on normalized linear-light signal N in [0, 1], where
// N = code / (2^bitDepth - 1). The linear-light luminance is returned in
// cd/m^2 (nits), with the PQ reference white peak of 10000 nits.
package pq

import "math"

// PQ constants from SMPTE ST 2084:2014, table 4.
const (
	M1 = 0.1593017578125 // 2610 / 16384
	M2 = 78.84375        // 2523 / 32
	C1 = 0.8359375       // 3424 / 4096
	C2 = 18.8515625      // 2413 / 4096
	C3 = 18.6875         // 2392 / 4096

	// PeakLuminance is the nominal peak luminance (nits) represented by N=1.
	PeakLuminance = 10000.0
)

// EOTF maps a normalized PQ signal N in [0,1] to linear luminance in nits.
// L = 10000 * ((max(N^(1/m2)-c1, 0)) / (c2 - c3*N^(1/m2)))^(1/m1)
func EOTF(n float64) float64 {
	if n <= 0 {
		return 0
	}
	if n >= 1 {
		return PeakLuminance
	}
	np := math.Pow(n, 1.0/M2)
	num := np - C1
	if num <= 0 {
		return 0
	}
	den := C2 - C3*np
	return PeakLuminance * math.Pow(num/den, 1.0/M1)
}

// OETF is the inverse EOTF: linear luminance in nits back to normalized
// PQ signal N in [0,1].
func OETF(lum float64) float64 {
	if lum <= 0 {
		return 0
	}
	if lum >= PeakLuminance {
		return 1
	}
	ym := math.Pow(lum/PeakLuminance, M1)
	n := math.Pow((C1+C2*ym)/(1.0+C3*ym), M2)
	if n < 0 {
		return 0
	}
	if n > 1 {
		return 1
	}
	return n
}

// CodeToNits converts an integer code value at the given bit depth
// (typically 10, 12 or 16) to luminance in nits.
func CodeToNits(code int, bitDepth int) float64 {
	maxCode := (1 << bitDepth) - 1
	if code <= 0 {
		return 0
	}
	if code >= maxCode {
		return PeakLuminance
	}
	return EOTF(float64(code) / float64(maxCode))
}

// NitsToCode converts luminance in nits to the nearest integer code value
// at the given bit depth.
func NitsToCode(lum float64, bitDepth int) int {
	maxCode := (1 << bitDepth) - 1
	n := OETF(lum)
	code := int(math.Round(n * float64(maxCode)))
	if code < 0 {
		return 0
	}
	if code > maxCode {
		return maxCode
	}
	return code
}

// MaxCode returns the maximum code value for a bit depth.
func MaxCode(bitDepth int) int { return (1 << bitDepth) - 1 }
