package icc

// Stats summarizes pointwise differences of two sampled curves.
type Stats struct {
	MaxAbs  float64 `json:"maxAbs"`
	RMS     float64 `json:"rms"`
	MeanAbs float64 `json:"meanAbs"`
	Samples int     `json:"samples"`
}

// CurveStats compares one channel's TRC between the two profiles.
type CurveStats struct {
	Channel    string      `json:"channel"`
	LeftForm   string      `json:"leftForm"`
	RightForm  string      `json:"rightForm"`
	Comparable bool        `json:"comparable"`
	Reason     string      `json:"reason,omitempty"`
	Stats      Stats       `json:"stats"`
	LeftQuant  *QuantStats `json:"leftQuant,omitempty"`
	RightQuant *QuantStats `json:"rightQuant,omitempty"`
}

// QuantStats measures the quantization of a curv LUT against an analytic
// reference curve evaluated at the same node positions.
type QuantStats struct {
	Present bool    `json:"present"`
	Entries int     `json:"entries"`
	MaxAbs  float64 `json:"maxAbs"`
	RMS     float64 `json:"rms"`
}

// WhitePointReport compares the wtpt tags (with header illuminant fallback).
type WhitePointReport struct {
	LeftSource    string  `json:"leftSource"`
	RightSource   string  `json:"rightSource"`
	LeftXYZ       XYZ     `json:"leftXYZ"`
	RightXYZ      XYZ     `json:"rightXYZ"`
	DeltaE76      float64 `json:"deltaE76"`
	LeftHeaderWP  XYZ     `json:"leftHeaderIlluminant"`
	RightHeaderWP XYZ     `json:"rightHeaderIlluminant"`
}

// MatrixCol compares one matrix column (r/g/b XYZ tag).
type MatrixCol struct {
	Channel    string  `json:"channel"`
	LeftXYZ    XYZ     `json:"leftXYZ"`
	RightXYZ   XYZ     `json:"rightXYZ"`
	MaxAbsDiff float64 `json:"maxAbsDiff"`
	LeftOK     bool    `json:"leftOK"`
	RightOK    bool    `json:"rightOK"`
}

// UnsupportedTag records a tag that lies outside the declared subset.
type UnsupportedTag struct {
	Signature string `json:"signature"`
	Offset    int    `json:"offset"`
	Size      int    `json:"size"`
	Type      string `json:"type"`
}

// Report is the full comparison result for two profiles.
type Report struct {
	LeftSummary  ProfileSummary   `json:"left"`
	RightSummary ProfileSummary   `json:"right"`
	WhitePoint   WhitePointReport `json:"whitePoint"`
	Matrix       []MatrixCol      `json:"matrix"`
	Curves       []CurveStats     `json:"curves"`
	Unsupported  struct {
		Left  []UnsupportedTag `json:"left"`
		Right []UnsupportedTag `json:"right"`
	} `json:"unsupported"`
}

// ProfileSummary exposes parse outcomes without leaking internal types.
type ProfileSummary struct {
	Path       string              `json:"path"`
	Size       int                 `json:"size"`
	Version    string              `json:"version"`
	Class      string              `json:"class"`
	ColorSpace string              `json:"colorSpace"`
	PCS        string              `json:"pcs"`
	TagCount   int                 `json:"tagCount"`
	Issues     []string            `json:"issues"`
	TagIssues  map[string][]string `json:"tagIssues"`
}

// SamplePoints is the number of evenly spaced points at which curves are
// compared, including both endpoints.
const SamplePoints = 256
