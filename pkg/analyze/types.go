// Package analyze derives reviewable HDR static metadata candidates
// (MaxCLL / MaxFALL) from actual decoded frame pixels using the SMPTE
// ST 2084 PQ transfer function. It only depends on the Go standard library.
package analyze

// Frame is a decoded image in PQ code values. R, G, B each hold one
// working-range code value per pixel, row-major, length Width*Height.
// BitDepth is the signal bit depth (10, 12 or 16); code values are stored
// in that range even when the container (PNG) physically carries 16 bits.
type Frame struct {
	Name     string
	Width    int
	Height   int
	BitDepth int
	R        []uint32
	G        []uint32
	B        []uint32
}

// PixelCount returns the number of pixels in the frame.
func (f *Frame) PixelCount() int { return f.Width * f.Height }

// Region is a 4-connected component of pixels above a highlight threshold.
type Region struct {
	ID        int
	Size      int     // pixel count
	AreaRatio float64 // Size / frame pixel count
	Qualified bool    // meets the minimum area ratio, i.e. not a speckle
	MinX      int
	MinY      int
	MaxX      int
	MaxY      int
	PeakNits  float64 // maximum pixel luminance inside the region
	PeakCode  int     // code value producing PeakNits
	MeanNits  float64 // mean luminance of pixels inside the region
}

// FrameStats holds per-frame luminance statistics in nits.
type FrameStats struct {
	Name       string
	MaxNits    float64 // single brightest pixel (CTA-861.3 frame MaxCLL input)
	MaxCode    int
	MeanNits   float64 // frame average light level (CTA-861.3 frame FALL input)
	P99_99Nits float64 // mean of the brightest 0.01% of pixels
	P99_9Nits  float64 // mean of the brightest 0.1% of pixels
	Over1000   int     // pixels above 1000 nits
	Over2000   int
	Over4000   int
	ClipPixels int // pixels at or above the display clip point
	ClipRatio  float64
	Regions    []Region
}

// SequenceStats aggregates per-frame results into static metadata candidates.
type SequenceStats struct {
	FrameCount    int
	BitDepth      int
	Frames        []FrameStats
	MaxCLLNits    float64 // max of per-frame maxima (strict CTA-861.3 definition)
	MaxFALLNits   float64 // max of per-frame averages (strict CTA-861.3 definition)
	RobustCLL     float64 // max across frames of per-frame p99.9 (speckle resistant)
	RegionCLL     float64 // brightest pixel inside an "area-qualified" region
	QualifiedArea int     // minimum pixel count used for RegionCLL
	ClipPointNits float64
	ClipPointCode int
	TotalClip     int
}
