package cbarcheck

// CandidateResult is the aggregate comparison of one region (or the whole
// frame) against one range/matrix/depth hypothesis.
type CandidateResult struct {
	Rank                int     `json:"rank"`
	Config              Config  `json:"config"`
	Score               float64 `json:"score"`
	MeanLumaError8      float64 `json:"meanLumaErrorCode8"`
	MeanCbError8        float64 `json:"meanCbErrorCode8"`
	MeanCrError8        float64 `json:"meanCrErrorCode8"`
	IllegalLumaPixels   int     `json:"illegalLumaPixels"`
	IllegalChromaPixels int     `json:"illegalChromaPixels"`
	EquivalentToBest    bool    `json:"equivalentToBest"`
	IsTarget            bool    `json:"isTarget"`
}

// PixelViolation is a single failing pixel under the target configuration.
// ErrorCode8 always uses 8-bit code-equivalent units; Observed, Expected and
// Delta use the numeric code scale of the frame or target as labelled.
type PixelViolation struct {
	X           int         `json:"x"`
	Y           int         `json:"y"`
	RegionIndex int         `json:"regionIndex"`
	Region      string      `json:"region"`
	Reasons     []string    `json:"reasons"`
	Observed    Sample      `json:"observed"`
	Expected    Sample      `json:"expected"`
	Delta       Sample      `json:"delta"`
	ErrorCode8  Sample      `json:"errorCode8"`
	Bounds      LegalBounds `json:"legalBounds"`
}

// RegionReport contains both the compliance result against target and the
// comparison against all 18 candidate configurations.
type RegionReport struct {
	Index        int        `json:"index"`
	Name         string     `json:"name"`
	Kind         RegionKind `json:"kind"`
	X0           int        `json:"x0"`
	Y0           int        `json:"y0"`
	X1           int        `json:"x1"`
	Y1           int        `json:"y1"`
	RGB          RGB        `json:"rgb"`
	Pixels       int        `json:"pixels"`
	Expected     Sample     `json:"expectedSample"`
	ObservedMean Sample     `json:"observedMean"`
	MeanError8   Sample     `json:"meanAbsErrorCode8"`

	LumaViolationPixels   int `json:"lumaViolationPixels"`
	ChromaViolationPixels int `json:"chromaViolationPixels"`
	IllegalLumaPixels     int `json:"illegalLumaPixels"`
	IllegalChromaPixels   int `json:"illegalChromaPixels"`

	TargetScore          float64           `json:"targetScore"`
	Best                 Config            `json:"bestConfig"`
	BestScore            float64           `json:"bestScore"`
	TargetEquivalentBest bool              `json:"targetEquivalentBest"`
	Candidates           []CandidateResult `json:"candidates"`
	FirstViolation       *PixelViolation   `json:"firstViolation,omitempty"`
}

type Summary struct {
	Pixels                int     `json:"pixels"`
	Regions               int     `json:"regions"`
	Compliant             bool    `json:"compliant"`
	LumaViolationPixels   int     `json:"lumaViolationPixels"`
	ChromaViolationPixels int     `json:"chromaViolationPixels"`
	IllegalLumaPixels     int     `json:"illegalLumaPixels"`
	IllegalChromaPixels   int     `json:"illegalChromaPixels"`
	RegionMismatches      int     `json:"regionConfigurationMismatches"`
	Target                Config  `json:"target"`
	Best                  Config  `json:"bestConfig"`
	BestScore             float64 `json:"bestScore"`
	TargetScore           float64 `json:"targetScore"`
	ConfigurationMismatch bool    `json:"configurationMismatch"`
}

// Report is deterministic for identical frame, layout, target, and thresholds.
type Report struct {
	Schema           string            `json:"schema"`
	Source           string            `json:"source,omitempty"`
	FrameWidth       int               `json:"frameWidth"`
	FrameHeight      int               `json:"frameHeight"`
	StorageDepth     BitDepth          `json:"storageDepth"`
	Target           Config            `json:"target"`
	Thresholds       Thresholds        `json:"thresholds"`
	Summary          Summary           `json:"summary"`
	GlobalCandidates []CandidateResult `json:"globalCandidates"`
	Regions          []RegionReport    `json:"regions"`
	FirstViolation   *PixelViolation   `json:"firstViolation,omitempty"`
	// FirstLumaViolation and FirstChromaViolation preserve the same row-major
	// scan order while keeping the two failure classes independently visible.
	FirstLumaViolation   *PixelViolation `json:"firstLumaViolation,omitempty"`
	FirstChromaViolation *PixelViolation `json:"firstChromaViolation,omitempty"`
}
