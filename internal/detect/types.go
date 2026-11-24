package detect

// Verdict labels the interpretation of a repeated-fragment hypothesis.
type Verdict string

const (
	VerdictWatermark Verdict = "watermark_repeat" // 真实重复：注入的短水印片段
	VerdictEcho      Verdict = "natural_echo"     // 自然回声：衰减的全局延迟链
	VerdictRinging   Verdict = "codec_ringing"    // 编码回响：块对齐短周期自相似
	VerdictRejected  Verdict = "rejected"         // 证据不足/反例占优
)

// Range is a half-open [Start,End) interval in raw sample indices.
type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// CI is a bootstrap percentile confidence interval.
type CI struct {
	Level float64 `json:"level"`
	Low   float64 `json:"low"`
	High  float64 `json:"high"`
}

// Occurrence is one located copy of a repeated fragment.
type Occurrence struct {
	Range        Range   `json:"range"`
	Band         string  `json:"band"`
	NCC          float64 `json:"ncc_to_anchor"`
	GainVsAnchor float64 `json:"gain_vs_anchor"`
	OffsetJitter int     `json:"aligned_offset_delta"`
}

// Features collects every measured discriminator so the rule decision is
// fully auditable from the report.
type Features struct {
	LagSamples      int     `json:"lag_samples"`
	LagSeconds      float64 `json:"lag_seconds"`
	NCCMean         float64 `json:"ncc_mean"`
	NCCMax          float64 `json:"ncc_max"`
	FracHighNCC     float64 `json:"frac_high_ncc"`
	GainMean        float64 `json:"gain_ratio_mean"`
	GainCV          float64 `json:"gain_ratio_cv"`
	DecayT123       float64 `json:"rms_decay_t1_t2_t3"`
	Rahmonic        float64 `json:"rahmonic_score"`
	GridFrac        float64 `json:"block_grid_fraction"`
	GridBlock       int     `json:"block_grid_size"`
	BlockDecay      float64 `json:"block_internal_decay"`
	OnsetLocalized  float64 `json:"onset_localized_fraction"`
	SupportFraction float64 `json:"support_fraction"`
	NullNCCMean     float64 `json:"shuffled_null_ncc_mean"`
}

// Finding is one fully-evidenced conclusion.
type Finding struct {
	Verdict        Verdict      `json:"verdict"`
	Label          string       `json:"label"`
	Band           string       `json:"band,omitempty"`
	FragmentLen    int          `json:"fragment_len_samples"`
	Range          Range        `json:"sample_range"`
	Occurrences    []Occurrence `json:"occurrences,omitempty"`
	Features       Features     `json:"features"`
	Confidence     CI           `json:"ncc_ci"`
	GainCI         CI           `json:"gain_ratio_ci"`
	Counterexample Counter      `json:"counterexample"`
	Reasons        []string     `json:"reasons"`
}

// RangeKey identifies a fragment family for cross-band deduplication.
func (f Finding) RangeKey() string {
	return f.Band + ":" +
		itostr(f.Range.Start) + "-" + itostr(f.Range.End) +
		":L" + itostr(f.FragmentLen)
}

func itostr(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Counter documents the competing explanation that was tested and defeated.
type Counter struct {
	Kind        string   `json:"kind"`
	Detail      string   `json:"detail"`
	NCC         float64  `json:"counter_ncc"`
	Threshold   *float64 `json:"decision_threshold"`
	SampleRange Range    `json:"counter_sample_range"`
}

// Report is the complete machine-readable result.
type Report struct {
	File          string          `json:"file"`
	SampleRate    int             `json:"sample_rate"`
	TotalSamples  int             `json:"total_samples"`
	DurationSec   float64         `json:"duration_sec"`
	Bands         []BandSpec      `json:"bands"`
	Findings      []Finding       `json:"findings"`
	Preprocessing string          `json:"preprocessing_note"`
	NullBaseline  float64         `json:"global_shuffled_null_ncc"`
	Summary       map[Verdict]int `json:"summary"`
}

type BandSpec struct {
	Name string  `json:"name"`
	Lo   float64 `json:"lo_hz"`
	Hi   float64 `json:"hi_hz"`
}

// Config holds analysis geometry; zero value is replaced by DefaultConfig.
type Config struct {
	FrameLen       int
	Hop            int
	AlignWindow    int
	AlignHop       int
	MinLag         int
	MaxLag         int
	WatermarkLens  []int
	CepWinLen      int
	Cephop         int
	BootstrapIters int
	HighNCCThresh  float64
	PairNCCThresh  float64
	MinClusterSize int
	GridBlocks     []int
}

func DefaultConfig(sampleRate int) Config {
	return Config{
		FrameLen:       1024,
		Hop:            512,
		AlignWindow:    1024,
		AlignHop:       256,
		MinLag:         256,
		MaxLag:         16000,
		WatermarkLens:  []int{256, 512, 1024, 2048},
		CepWinLen:      32768,
		Cephop:         16384,
		BootstrapIters: 1000,
		HighNCCThresh:  0.5,
		PairNCCThresh:  0.6,
		MinClusterSize: 3,
		GridBlocks:     []int{128, 256, 512, 1024},
	}
}
