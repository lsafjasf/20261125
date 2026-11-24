// Package auditor reconciles claims about the same media item made by
// different evidence sources (file system, container, bitstream, sidecar).
//
// The reconciler never silently picks a value: every conclusion is either
// automatic (the admissible evidence is sufficient on its own) or routed to
// human review. When the evidence is insufficient the file system is never
// used as a fallback value.
package auditor

import "time"

// Source identifies where a piece of evidence came from.
type Source string

const (
	// SourceFilesystem is the least trustworthy source: file names, directory
	// layout and stat(2) metadata. It may be absent or wrong by convention.
	SourceFilesystem Source = "filesystem"
	// SourceContainer is metadata parsed from the media container header
	// (e.g. MP4/MKV/MP3 tags and header duration).
	SourceContainer Source = "container"
	// SourceBitstream is information decoded from the elementary stream
	// itself (e.g. frame counts, audio language tracks).
	SourceBitstream Source = "bitstream"
	// SourceSidecar is an external metadata companion (e.g. .nfo/.json).
	SourceSidecar Source = "sidecar"
)

// Field identifies the media attribute under review.
type Field string

const (
	FieldLanguage Field = "language"
	FieldDuration Field = "duration"
)

// Review statuses: exactly one of these is emitted per field.
const (
	// StatusAutomatic means the value was derived without human input.
	StatusAutomatic = "automatic"
	// StatusManual means the evidence conflicts and a human must decide.
	StatusManual = "manual"
	// StatusUndetermined means no admissible non-filesystem evidence exists;
	// no value is emitted and the file system must not be substituted.
	StatusUndetermined = "undetermined"
)

// Confidence levels for automatic conclusions.
const (
	ConfidenceHigh = "high"
	ConfidenceLow  = "low"
)

// Observation is one raw claim made by one source about one media item.
type Observation struct {
	// MediaID identifies the media item the claim belongs to.
	MediaID string
	// Source is the origin of the claim.
	Source Source
	// Field is the attribute the claim describes.
	Field Field
	// RawValue is the original, unnormalised value as emitted by the source
	// (e.g. "EN", "zh-CN", "01:42:03.500", "6123.5s").
	RawValue string
	// ObservedAt records when the evidence was collected.
	ObservedAt time.Time
	// Detail is optional free-form provenance (path, track number, atom...).
	Detail string
}

// SourceValue is one normalised observation as it appears in a report.
type SourceValue struct {
	Source    Source `json:"source"`
	RawValue  string `json:"rawValue"`
	Canonical string `json:"canonical"`
	Present   bool   `json:"present"`
	Admitted  bool   `json:"admitted"`
	Detail    string `json:"detail,omitempty"`
	// NormalizedSeconds is populated for duration observations; 0 otherwise.
	NormalizedSeconds float64 `json:"normalizedSeconds,omitempty"`
}

// Conflict records one divergent pair of observations on the same field.
type Conflict struct {
	Field   Field  `json:"field"`
	SourceA Source `json:"sourceA"`
	ValueA  string `json:"valueA"`
	SourceB Source `json:"sourceB"`
	ValueB  string `json:"valueB"`
	// InvolvesFilesystem is true when one side of the conflict is the
	// file system. Such conflicts are reported but never block an automatic
	// conclusion that is supported by the other sources.
	InvolvesFilesystem bool `json:"involvesFilesystem"`
	// IntraSource is true when the same source emitted competing values
	// (e.g. two sidecar files disagreeing). It always forces manual review.
	IntraSource bool `json:"intraSource"`
}

// FieldVerdict is the review result for one field of one media item.
type FieldVerdict struct {
	Field      Field  `json:"field"`
	Status     string `json:"status"`
	Confidence string `json:"confidence,omitempty"`
	// Value is the concluded canonical value. It is empty whenever Status is
	// manual or undetermined: this package never guesses, and in particular
	// it never copies a file-system value into an undetermined conclusion.
	Value string `json:"value,omitempty"`
	// ValueSources lists every source whose evidence supports Value.
	ValueSources []Source `json:"valueSources,omitempty"`
	// Reason is a machine-readable explanation, populated for manual and
	// undetermined verdicts.
	Reason     string        `json:"reason,omitempty"`
	Provenance []SourceValue `json:"provenance"`
	Conflicts  []Conflict    `json:"conflicts"`
}

// Report is the full audit output for one media item.
type Report struct {
	MediaID      string         `json:"mediaId"`
	GeneratedAt  time.Time      `json:"generatedAt"`
	Verdicts     []FieldVerdict `json:"verdicts"`
	SourceErrors []SourceError  `json:"sourceErrors,omitempty"`
}

// SourceError records a provider failure without aborting the whole audit.
type SourceError struct {
	Source Source `json:"source"`
	Err    string `json:"error"`
}

// Manual returns the verdicts that require a human decision.
func (r Report) Manual() []FieldVerdict {
	return r.filter(StatusManual)
}

// Automatic returns the verdicts concluded without human input.
func (r Report) Automatic() []FieldVerdict {
	return r.filter(StatusAutomatic)
}

// Undetermined returns the verdicts with no admissible non-filesystem value.
func (r Report) Undetermined() []FieldVerdict {
	return r.filter(StatusUndetermined)
}

func (r Report) filter(status string) []FieldVerdict {
	var out []FieldVerdict
	for _, verdict := range r.Verdicts {
		if verdict.Status == status {
			out = append(out, verdict)
		}
	}
	return out
}
