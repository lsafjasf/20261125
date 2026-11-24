package nightwb

import "math"

// Illuminant is the estimated scene illuminant expressed in linear sRGB
// (normalized so max channel == 1) and, when confidence permits, in XYZ/CCT.
type Illuminant struct {
	RGB         [3]float64 // linear sRGB, max-normalized
	XYZ         [3]float64
	CCT         float64   // correlated color temperature in K; 0 if unavailable
	Tint        float64   // green-magenta deviation along the Planckian locus
	Confidence  float64   // 0..1
	SampleCount int       // usable neutral samples
	MixedLight  bool      // bimodal neutral chromaticity detected
	Clusters    []Cluster // chromaticity clusters (mixed light)
	Method      string    // estimation path used
	Warnings    []string
}

// Cluster is a mode of the neutral-sample chromaticity distribution.
type Cluster struct {
	Weight     float64
	MeanRGB    [3]float64
	CCT        float64
	MeanChroma [2]float64
}

// Scheme configures a chromatic-adaptation pass.
type Scheme struct {
	Name       string
	Strength   float64 // 0 = no adaptation, 1 = full von Kries
	DesatLimit float64 // fraction of highlight headroom below which adaptation
	// is blended out per pixel (highlight desaturation protection); 0 disables
	HighlightKnee float64
}

// ConservativeScheme partially adapts and protects highlights; it trades some
// residual cast for lower risk of skin tone and highlight artifacts.
func ConservativeScheme() Scheme {
	return Scheme{Name: "conservative", Strength: 0.55, DesatLimit: 0.85, HighlightKnee: 0.1}
}

// AggressiveScheme performs full adaptation toward D65 with no per-pixel
// highlight protection.
func AggressiveScheme() Scheme {
	return Scheme{Name: "aggressive", Strength: 1.0, DesatLimit: 0, HighlightKnee: 0}
}

// Recoverability bit flags. A pixel may carry several.
const (
	FlagClippedRed = 1 << iota // channel saturated in the source
	FlagClippedGreen
	FlagClippedBlue
	FlagCrushed       // all channels near black: shadow detail gone
	FlagOutOfGamut    // adapted result exceeds [0,1] before encoding
	FlagMixedLight    // pixel sits in a region with competing illuminants
	FlagLowConfidence // global illuminant estimate is unreliable here
)

// ExposureReport summarizes exposure anomalies found in the frame.
type ExposureReport struct {
	HighlightClippedFrac float64 // pixels with any channel at sensor max
	ShadowCrushedFrac    float64 // pixels with all channels near zero
	UsableFrac           float64 // pixels with usable dynamic range
	ExposureState        string  // "normal", "overexposed", "underexposed"
}

// SchemeReport describes one adaptation output.
type SchemeReport struct {
	Name               string
	Strength           float64
	OutOfGamutFrac     float64
	MeanAdaptedNeutral [3]float64
}

// Report is the full machine-readable analysis result.
type Report struct {
	Illuminant        Illuminant
	Exposure          ExposureReport
	Recoverable       RecoverabilityStats
	Schemes           []SchemeReport
	UnrecoverableNote string
}

type RecoverabilityStats struct {
	ClippedFrac    float64
	CrushedFrac    float64
	OutOfGamutFrac float64
	MixedLightFrac float64
	LowConfFrac    float64
	AnyFlagFrac    float64
	FlagCounts     map[uint16]int
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
