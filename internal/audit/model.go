package audit

const SchemaVersion = "editlist-audit/v1"

// Report is the complete read-only audit result for one or more inputs.
type Report struct {
	SchemaVersion string       `json:"schema_version"`
	ReadOnly      bool         `json:"read_only"`
	Inputs        []FileReport `json:"inputs"`
	Summary       Summary      `json:"summary"`
}

type Summary struct {
	Files           int `json:"files"`
	Tracks          int `json:"tracks"`
	Edits           int `json:"edits"`
	Errors          int `json:"errors"`
	Warnings        int `json:"warnings"`
	EmptyEdits      int `json:"empty_edits"`
	DwellEdits      int `json:"dwell_edits"`
	NegativeStarts  int `json:"negative_starts"`
	CrossTimescales int `json:"cross_timescales"`
}

type FileReport struct {
	Path     string        `json:"path"`
	FileSize int64         `json:"file_size"`
	Boxes    []*Box        `json:"-"`
	Movie    *MovieInfo    `json:"movie,omitempty"`
	Tracks   []TrackReport `json:"tracks"`
	Findings []Finding     `json:"findings"`
	Error    string        `json:"error,omitempty"`
}

type MovieInfo struct {
	Timescale      uint32 `json:"timescale"`
	Duration       uint64 `json:"duration"`
	DurationSecond string `json:"duration_seconds_exact"`
}

type TrackReport struct {
	Index      int          `json:"index"`
	TrackID    uint32       `json:"track_id"`
	Handler    string       `json:"handler"`
	IsAudio    bool         `json:"is_audio"`
	TKHD       *TKHDInfo    `json:"tkhd,omitempty"`
	MDHD       *MDHDInfo    `json:"mdhd,omitempty"`
	ELST       *ELSTInfo    `json:"elst,omitempty"`
	Edits      []EditReport `json:"edits"`
	AudioStart *AudioStart  `json:"audio_start,omitempty"`
	Findings   []Finding    `json:"findings"`
}

type TKHDInfo struct {
	Version  uint8  `json:"version"`
	TrackID  uint32 `json:"track_id"`
	Duration uint64 `json:"duration_movie_units"`
}

type MDHDInfo struct {
	Version   uint8  `json:"version"`
	Timescale uint32 `json:"timescale"`
	Duration  uint64 `json:"duration_media_units"`
}

type ELSTInfo struct {
	Offset     int64  `json:"box_offset"`
	Version    uint8  `json:"version"`
	EntryCount uint32 `json:"entry_count"`
}

type Interval struct {
	Timescale        uint32  `json:"timescale"`
	StartUnits       string  `json:"start_units_exact"`
	EndUnits         string  `json:"end_units_exact"`
	DurationUnits    string  `json:"duration_units_exact"`
	StartSeconds     string  `json:"start_seconds_exact"`
	EndSeconds       string  `json:"end_seconds_exact"`
	DurationSeconds  string  `json:"duration_seconds_exact"`
	StartSecondsF    float64 `json:"start_seconds"`
	EndSecondsF      float64 `json:"end_seconds"`
	DurationSecondsF float64 `json:"duration_seconds"`
}

type RoundedUnits struct {
	Floor   string `json:"floor"`
	Nearest string `json:"nearest"`
	Ceil    string `json:"ceil"`
}

type EditReport struct {
	Index                    int           `json:"index"`
	Kind                     string        `json:"kind"`
	Empty                    bool          `json:"empty"`
	Dwell                    bool          `json:"dwell"`
	NegativeMediaTime        bool          `json:"negative_media_time"`
	SegmentDuration          uint64        `json:"segment_duration_movie_units"`
	Composition              Interval      `json:"composition_interval"`
	Media                    *Interval     `json:"media_interval,omitempty"`
	MediaDurationRound       *RoundedUnits `json:"media_duration_rounded,omitempty"`
	MediaOriginOnComposition *Point        `json:"media_zero_on_composition,omitempty"`
	RateRaw                  string        `json:"media_rate_raw_hex"`
	RateFixed                string        `json:"media_rate_fixed"`
	RateRational             string        `json:"media_rate_rational"`
	Warnings                 []string      `json:"warnings,omitempty"`
}

type Point struct {
	Timescale    uint32  `json:"timescale"`
	UnitsExact   string  `json:"units_exact"`
	SecondsExact string  `json:"seconds_exact"`
	Seconds      float64 `json:"seconds"`
}

type AudioStart struct {
	EditIndex               int    `json:"edit_index"`
	Kind                    string `json:"kind"`
	SourceMediaTimeUnits    int64  `json:"source_media_time_units"`
	SourceMediaTimeSeconds  string `json:"source_media_time_seconds_exact"`
	CompositionStartUnits   string `json:"composition_start_movie_units"`
	CompositionStartSeconds string `json:"composition_start_seconds_exact"`
	MediaZeroCompUnits      *Point `json:"media_zero_composition,omitempty"`
	UnitRateOffsetMedia     *Point `json:"unit_rate_offset_media,omitempty"`
	LeadingEmptySeconds     string `json:"leading_empty_seconds_exact,omitempty"`
	NegativeTimelineOrigin  bool   `json:"negative_timeline_origin"`
}

type Finding struct {
	Severity   string `json:"severity"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	BoxPath    string `json:"box_path,omitempty"`
	TrackIndex int    `json:"track_index,omitempty"`
	EditIndex  int    `json:"edit_index,omitempty"`
}
