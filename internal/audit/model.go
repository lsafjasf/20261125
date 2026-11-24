package audit

import (
	"time"

	"cutmux/internal/media"
)

type Options struct {
	LabelA      string
	LabelB      string
	FramesA     string
	FramesB     string
	Now         time.Time
	ToleranceMS float64
}

type Report struct {
	Tool          string             `json:"tool"`
	Version       string             `json:"version"`
	GeneratedUTC  string             `json:"generatedUtc"`
	LabelA        string             `json:"labelA"`
	LabelB        string             `json:"labelB"`
	ContainerA    ContainerSummary   `json:"containerA"`
	ContainerB    ContainerSummary   `json:"containerB"`
	Video         VideoComparison    `json:"video"`
	Audio         AudioComparison    `json:"audio"`
	Subtitle      SubtitleComparison `json:"subtitle"`
	BlackFrames   BlackComparison    `json:"blackFrames"`
	TotalDuration DurationPair       `json:"totalDuration"`
	Findings      []Finding          `json:"findings"`
	Verdict       Verdict            `json:"verdict"`
	NextCheck     string             `json:"nextCheck"`
}

type ContainerSummary struct {
	Path          string         `json:"path"`
	SHA256        string         `json:"sha256"`
	Format        string         `json:"format"`
	Brand         string         `json:"brand,omitempty"`
	DeclaredMs    float64        `json:"declaredDurationMs"`
	MeasuredEndMs float64        `json:"measuredEndMs"`
	TrackCount    int            `json:"trackCount"`
	Tracks        []TrackSummary `json:"tracks"`
	Warnings      []string       `json:"warnings,omitempty"`
}

type TrackSummary struct {
	ID               uint64        `json:"id"`
	Kind             string        `json:"kind"`
	Codec            string        `json:"codec"`
	Language         string        `json:"language,omitempty"`
	Timescale        uint64        `json:"timescale"`
	Samples          int           `json:"samples"`
	RawStartMs       float64       `json:"rawStartMs"`
	RawEndMs         float64       `json:"rawEndMs"`
	PresentedStartMs float64       `json:"presentedStartMs"`
	PresentedEndMs   float64       `json:"presentedEndMs"`
	PresentedMs      float64       `json:"presentedDurationMs"`
	HeadTrimMs       float64       `json:"headTrimMs"`
	TailTrimMs       float64       `json:"tailTrimMs"`
	CodecDelayMs     float64       `json:"codecDelayMs"`
	SeekPreRollMs    float64       `json:"seekPreRollMs"`
	FrameRate        string        `json:"frameRate,omitempty"`
	FrameRateFloat   float64       `json:"frameRateFloat,omitempty"`
	DefaultFrameMs   float64       `json:"defaultFrameMs,omitempty"`
	Edits            []EditSummary `json:"edits,omitempty"`
}

type EditSummary struct {
	SegmentDuration uint64  `json:"segmentDuration"`
	MediaTime       int64   `json:"mediaTime"`
	MediaRate       float64 `json:"mediaRate"`
	SegmentMs       float64 `json:"segmentMs,omitempty"`
	MediaStartMs    float64 `json:"mediaStartMs,omitempty"`
}

type DurationPair struct {
	A     DurationBreakdown `json:"a"`
	B     DurationBreakdown `json:"b"`
	Delta float64           `json:"deltaMs"`
}

type DurationBreakdown struct {
	DeclaredMs float64 `json:"declaredMs"`
	MeasuredMs float64 `json:"measuredMs"`
}

type CutPoint struct {
	StartMs   float64 `json:"startMs"`
	EndMs     float64 `json:"endMs"`
	Samples   int     `json:"samples"`
	FirstHash string  `json:"firstHash,omitempty"`
	LastHash  string  `json:"lastHash,omitempty"`
}

type TimelinePair struct {
	A     CutPoint `json:"a"`
	B     CutPoint `json:"b"`
	Delta float64  `json:"startDeltaMs"`
}

type VideoComparison struct {
	Timeline       TimelinePair `json:"timeline"`
	FPSA           string       `json:"fpsA"`
	FPSB           string       `json:"fpsB"`
	FPSMatch       bool         `json:"fpsMatch"`
	FrameDeltaMs   float64      `json:"frameDeltaMs"`
	RawFramesA     int          `json:"rawFramesA"`
	RawFramesB     int          `json:"rawFramesB"`
	PresentedA     int          `json:"presentedFramesA"`
	PresentedB     int          `json:"presentedFramesB"`
	Identity       Alignment    `json:"identity"`
	OnlyA          []FrameRun   `json:"onlyA,omitempty"`
	OnlyB          []FrameRun   `json:"onlyB,omitempty"`
	Classification string       `json:"classification"`
	Explanation    string       `json:"explanation"`
}

type AudioComparison struct {
	Timeline         TimelinePair `json:"timeline"`
	PacketsA         int          `json:"packetsA"`
	PacketsB         int          `json:"packetsB"`
	SampleFramesA    int64        `json:"sampleFramesA"`
	SampleFramesB    int64        `json:"sampleFramesB"`
	SampleFrameDelta int64        `json:"sampleFrameDelta"`
	DurationDeltaMs  float64      `json:"durationDeltaMs"`
	Identity         Alignment    `json:"identity"`
	OnlyA            []FrameRun   `json:"onlyA,omitempty"`
	OnlyB            []FrameRun   `json:"onlyB,omitempty"`
	HeadTrimA        float64      `json:"headTrimA_ms"`
	HeadTrimB        float64      `json:"headTrimB_ms"`
	TailTrimA        float64      `json:"tailTrimA_ms"`
	TailTrimB        float64      `json:"tailTrimB_ms"`
	CodecDelayA      float64      `json:"codecDelayA_ms"`
	CodecDelayB      float64      `json:"codecDelayB_ms"`
	Classification   string       `json:"classification"`
	Explanation      string       `json:"explanation"`
}

type SubtitleComparison struct {
	Timeline       TimelinePair `json:"timeline"`
	PresentA       bool         `json:"presentA"`
	PresentB       bool         `json:"presentB"`
	Classification string       `json:"classification"`
	Explanation    string       `json:"explanation"`
}

type BlackRun struct {
	StartMs float64 `json:"startMs"`
	EndMs   float64 `json:"endMs"`
	Frames  int     `json:"frames"`
}

type BlackEvidence struct {
	Source     string     `json:"source"`
	Definitive bool       `json:"definitive"`
	Matched    int        `json:"matchedFrames"`
	Total      int        `json:"totalFrames"`
	Runs       []BlackRun `json:"runs,omitempty"`
	Warning    string     `json:"warning,omitempty"`
}

type BlackFramePoint struct {
	AtMs       float64 `json:"atMs"`
	EndMs      float64 `json:"endMs"`
	Hash       string  `json:"hash"`
	Black      bool    `json:"black"`
	Definitive bool    `json:"definitive"`
}

type BlackComparison struct {
	A              BlackEvidence     `json:"a"`
	B              BlackEvidence     `json:"b"`
	OnlyA          []BlackFramePoint `json:"onlyA,omitempty"`
	OnlyB          []BlackFramePoint `json:"onlyB,omitempty"`
	Classification string            `json:"classification"`
	Explanation    string            `json:"explanation"`
}

type Finding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type Verdict struct {
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	LegalMux    bool   `json:"legalMuxDifferencesOnly"`
	RealLoss    bool   `json:"realFrameLossDetected"`
	NeedsReview bool   `json:"needsReview"`
}

type Alignment struct {
	Comparable bool       `json:"comparable"`
	Reason     string     `json:"reason,omitempty"`
	Common     int        `json:"common"`
	OnlyA      int        `json:"onlyA"`
	OnlyB      int        `json:"onlyB"`
	OnlyARuns  []IndexRun `json:"onlyARuns,omitempty"`
	OnlyBRuns  []IndexRun `json:"onlyBRuns,omitempty"`
}

type IndexRun struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type FrameRun struct {
	StartIndex int      `json:"startIndex"`
	EndIndex   int      `json:"endIndex"`
	StartMs    float64  `json:"startMs"`
	EndMs      float64  `json:"endMs"`
	Frames     int      `json:"frames"`
	Hashes     []string `json:"hashes,omitempty"`
}

func ms(ns int64) float64 {
	return float64(ns) / 1e6
}

func trackSummary(t *media.Track) TrackSummary {
	raw := t.RawSpan()
	presented := t.PresentedSpan()
	ts := TrackSummary{
		ID: t.ID, Kind: string(t.Kind), Codec: t.Codec, Language: t.Language,
		Timescale: t.Timescale, Samples: len(t.Samples),
		RawStartMs: ms(raw.Start), RawEndMs: ms(raw.End),
		PresentedStartMs: ms(presented.Start), PresentedEndMs: ms(presented.End),
		PresentedMs: ms(presented.End - presented.Start),
		HeadTrimMs:  ms(t.HeadTrim()), TailTrimMs: ms(t.TailTrim()),
		CodecDelayMs: ms(t.CodecDelay), SeekPreRollMs: ms(t.SeekPreRoll),
		DefaultFrameMs: ms(t.DefaultDuration),
	}
	if t.Kind == media.KindVideo {
		r := t.FrameRate()
		ts.FrameRate = r.String()
		ts.FrameRateFloat = r.Float()
	}
	for _, e := range t.Edits {
		es := EditSummary{
			SegmentDuration: e.SegmentDuration, MediaTime: e.MediaTime,
			MediaRate: e.MediaRate,
		}
		if t.MovieTimescale > 0 {
			es.SegmentMs = ms(scaleNS(e.SegmentDuration, t.MovieTimescale))
			if e.MediaTime >= 0 {
				es.MediaStartMs = ms(scaleNS(uint64(e.MediaTime), t.Timescale))
			}
		}
		ts.Edits = append(ts.Edits, es)
	}
	return ts
}

func scaleNS(v uint64, timescale uint64) int64 {
	if timescale == 0 {
		return 0
	}
	return int64(float64(v) * 1e9 / float64(timescale))
}
