// Package repair executes batches of label corrections with strict
// transactional semantics.
//
// Corrections arrive in groups: one group always carries both the language
// and the duration fix for one media item. A group either fully succeeds
// (both sidecar labels updated) or leaves the library untouched. With the
// all-or-nothing policy the whole batch shares that fate.
package repair

import "time"

// Group statuses reported after a run.
const (
	StatusApplied    = "applied"
	StatusRolledBack = "rolled_back"
	StatusSkipped    = "skipped"
)

// FieldResult records the outcome of one field inside a group.
type FieldResult struct {
	Field     string `json:"field"`
	Before    string `json:"before"`
	After     string `json:"after"`
	Canonical string `json:"canonical"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason,omitempty"`
	Authority string `json:"authority_source,omitempty"`
}

// GroupResult is the full outcome of one correction group.
type GroupResult struct {
	GroupID       string        `json:"group_id"`
	MediaID       string        `json:"media_id"`
	SidecarPath   string        `json:"sidecar_path"`
	Status        string        `json:"status"`
	Fields        []FieldResult `json:"fields"`
	FailureReason string        `json:"failure_reason,omitempty"`
	// HashBefore/HashAfter are the media file digests; they must be identical.
	HashBefore string `json:"hash_before"`
	HashAfter  string `json:"hash_after"`
	HashStable bool   `json:"hash_stable"`
	// SameNamePeers lists other media_ids sharing the display name, proving
	// the multi-language variants were disambiguated by id, not name.
	SameNamePeers []string  `json:"same_name_peers,omitempty"`
	AppliedAt     time.Time `json:"applied_at,omitempty"`
}

// Summary aggregates the run for the machine-readable report.
type Summary struct {
	TotalGroups   int `json:"total_groups"`
	Applied       int `json:"applied"`
	RolledBack    int `json:"rolled_back"`
	Skipped       int `json:"skipped"`
	ChangedFields int `json:"changed_fields"`
}

// Report is the complete run artifact.
type Report struct {
	LibraryRoot  string        `json:"library_root"`
	StartedAt    time.Time     `json:"started_at"`
	FinishedAt   time.Time     `json:"finished_at"`
	AllOrNothing bool          `json:"all_or_nothing"`
	DryRun       bool          `json:"dry_run"`
	Summary      Summary       `json:"summary"`
	Groups       []GroupResult `json:"groups"`
	// Aborted is set when all-or-nothing preflight fails and nothing ran.
	Aborted bool `json:"aborted,omitempty"`
}
