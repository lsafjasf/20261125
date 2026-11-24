package audit

import "time"

type DeclaredRange string

const (
	DeclaredFull    DeclaredRange = "full"
	DeclaredLimited DeclaredRange = "limited"
	DeclaredUnknown DeclaredRange = "unknown"
)

type ClipSpec struct {
	ID          string        `json:"id"`
	Path        string        `json:"path"`
	Declared    DeclaredRange `json:"declared,omitempty"`
	Width       int           `json:"width,omitempty"`
	Height      int           `json:"height,omitempty"`
	PixelFormat string        `json:"pixelFormat,omitempty"`
	MaxSamples  uint64        `json:"maxSamples,omitempty"`
}

type Manifest struct {
	Defaults ClipSpec   `json:"defaults"`
	Clips    []ClipSpec `json:"clips"`
}

type LegalRange struct {
	Name  string `json:"name"`
	Black int    `json:"black"`
	White int    `json:"white"`
}

type Percentiles struct {
	P001  int `json:"p0_1"`
	P01   int `json:"p1"`
	P05   int `json:"p5"`
	P25   int `json:"p25"`
	P50   int `json:"p50"`
	P75   int `json:"p75"`
	P95   int `json:"p95"`
	P99   int `json:"p99"`
	P999  int `json:"p99_9"`
	P9999 int `json:"p99_99"`
}

type CodeCounts struct {
	LT16    uint64 `json:"below16"`
	EQ16    uint64 `json:"at16"`
	IN      uint64 `json:"inside16to235"`
	EQ235   uint64 `json:"at235"`
	GT235   uint64 `json:"above235"`
	EQ0     uint64 `json:"at0"`
	EQ255   uint64 `json:"at255"`
	Below64 uint64 `json:"below64"`
}

type Fractions struct {
	LT16    float64 `json:"below16"`
	EQ16    float64 `json:"at16"`
	IN      float64 `json:"inside16to235"`
	EQ235   float64 `json:"at235"`
	GT235   float64 `json:"above235"`
	Outside float64 `json:"outside16to235"`
	EQ0     float64 `json:"at0"`
	EQ255   float64 `json:"at255"`
	Below64 float64 `json:"below64"`
}

type SampleStats struct {
	Samples   uint64      `json:"samples"`
	Width     int         `json:"width,omitempty"`
	Height    int         `json:"height,omitempty"`
	Frames    int         `json:"frames"`
	Min       int         `json:"min"`
	Max       int         `json:"max"`
	Distinct  int         `json:"distinctCodes"`
	Percent   Percentiles `json:"percentiles"`
	Counts    CodeCounts  `json:"counts"`
	Fractions Fractions   `json:"fractions"`
}

type BlackLevel struct {
	Anchor          int      `json:"anchor"`
	ObservedFloor   int      `json:"observedFloor"`
	TypicalShadow   int      `json:"typicalShadowP5"`
	PedestalCodes   int      `json:"pedestalCodes"`
	PedestalPercent float64  `json:"pedestalPercentOfLegalRange"`
	Severity        string   `json:"severity"`
	Reading         string   `json:"reading"`
	StyleReading    string   `json:"styleReading"`
	Alternatives    []string `json:"alternatives,omitempty"`
}

type Hypothesis struct {
	Key            string     `json:"key"`
	Range          LegalRange `json:"range"`
	Score          int        `json:"score"`
	Summary        string     `json:"summary"`
	CorrectedLabel string     `json:"correctedLabel"`
	BlackLevel     BlackLevel `json:"blackLevel"`
	Evidence       []string   `json:"evidence"`
}

type Assessment struct {
	Declared       DeclaredRange `json:"declared"`
	Decision       string        `json:"decision"`
	Confidence     string        `json:"confidence"`
	LabelStatus    string        `json:"labelStatus"`
	CorrectedLabel string        `json:"correctedLabel"`
	Summary        string        `json:"summary"`
	Hypotheses     []Hypothesis  `json:"hypotheses"`
	Evidence       []string      `json:"evidence"`
	Caveats        []string      `json:"caveats"`
}

type LoadInfo struct {
	Path      string `json:"path"`
	Format    string `json:"format"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Frames    int    `json:"frames"`
	Sampled   uint64 `json:"sampled"`
	FileBytes int64  `json:"fileBytes,omitempty"`
}

type ClipResult struct {
	ID         string      `json:"id"`
	Input      LoadInfo    `json:"input"`
	Stats      SampleStats `json:"stats"`
	Assessment Assessment  `json:"assessment"`
	Error      string      `json:"error,omitempty"`
}

type BatchSummary struct {
	Clips            int            `json:"clips"`
	Analyzed         int            `json:"analyzed"`
	Decisions        map[string]int `json:"decisions"`
	Declared         map[string]int `json:"declared"`
	CorrectedLabels  map[string]int `json:"correctedLabels"`
	LabelMismatches  int            `json:"labelMismatches"`
	Indeterminate    int            `json:"indeterminate"`
	DominantDecision string         `json:"dominantDecision"`
	BatchConclusion  string         `json:"batchConclusion"`
}

type Report struct {
	SchemaVersion int          `json:"schemaVersion"`
	GeneratedAt   time.Time    `json:"generatedAt"`
	ToolVersion   string       `json:"toolVersion"`
	Policy        string       `json:"policy"`
	Batch         BatchSummary `json:"batch"`
	Clips         []ClipResult `json:"clips"`
}
