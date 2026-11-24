// Package catalog defines the on-disk media library model used by the ad
// label repair tool.
//
// The library is a directory tree of read-only media files paired with JSON
// metadata sidecars. Media bytes are never rewritten: only the language and
// duration labels inside a sidecar may change, and only after the field
// authority chain (see package authority) confirms the correction.
package catalog

import "time"

// Field names that can appear in a label record. Only Language and Duration
// are mutable. Every other field is protected and must survive a repair run
// byte-for-byte apart from the two allowed labels.
const (
	FieldLanguage = "language"
	FieldDuration = "duration"
)

// MutableFields is the exact allow-list of fields a repair may touch.
var MutableFields = map[string]bool{
	FieldLanguage: true,
	FieldDuration: true,
}

// EvidenceSource names where a label value originally came from. The order
// of the constants mirrors the field authority chain: higher rank wins.
type EvidenceSource string

const (
	// SourceFilesystem is the weakest source: a file name or directory
	// convention. It can never author a correction on its own.
	SourceFilesystem EvidenceSource = "filesystem"
	// SourceContainer comes from the media container header (e.g. MP4 udta
	// tags, header duration).
	SourceContainer EvidenceSource = "container"
	// SourceBitstream is decoded from the elementary stream (real track
	// language, frame/timeline duration). Strongest technical source.
	SourceBitstream EvidenceSource = "bitstream"
	// SourceSidecar is the human-curated metadata companion. Curated
	// corrections ride on this source and outrank the raw technical read.
	SourceSidecar EvidenceSource = "sidecar"
)

// Evidence is one observed value for one field, kept for audit.
type Evidence struct {
	Source EvidenceSource `json:"source"`
	Field  string         `json:"field"`
	Raw    string         `json:"raw"`
	At     time.Time      `json:"at"`
}

// Label is the current value of one mutable field plus its provenance.
type Label struct {
	Value    string         `json:"value"`
	Source   EvidenceSource `json:"source"`
	Evidence []Evidence     `json:"evidence,omitempty"`
}

// MediaRecord is one sidecar document. The JSON layout is stable and the
// unknown/protected fields are preserved through a decode/encode round trip
// (see Decode and Encode), so adding new top-level fields never loses data.
type MediaRecord struct {
	MediaID   string           `json:"media_id"`
	Name      string           `json:"name"`
	MediaPath string           `json:"media_path"`
	MediaHash string           `json:"media_hash"`
	CreatedAt time.Time        `json:"created_at"`
	Labels    map[string]Label `json:"labels"`

	// sidecarPath is the file this record was loaded from; not serialised.
	sidecarPath string
}

// SidecarPath returns the absolute or library-relative path of the JSON
// sidecar backing this record.
func (m *MediaRecord) SidecarPath() string { return m.sidecarPath }

// SetSidecarPath records where the record was loaded from.
func (m *MediaRecord) SetSidecarPath(p string) { m.sidecarPath = p }

// Correction is one requested new value for one field of one media item.
// A group (same GroupID) always contains both the language and duration
// correction; the group is applied atomically or rolled back together.
type Correction struct {
	GroupID string `json:"group_id"`
	MediaID string `json:"media_id"`
	Field   string `json:"field"`
	Value   string `json:"value"`
	Reason  string `json:"reason,omitempty"`
}

// CorrectionFile is the batch document handed to the repair command.
type CorrectionFile struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Corrections []Correction `json:"corrections"`
}
