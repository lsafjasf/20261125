// Package model defines the on-disk schemas shared by the batch processor.
package model

import "time"

const SchemaVersion = 1

// StageName is one stage of the render pipeline. Stages always run in
// CanonicalStages order and each committed stage is a resumable checkpoint.
type StageName string

const (
	StagePrepare StageName = "prepare"
	StageRender  StageName = "render"
	StageEncode  StageName = "encode"
	StagePublish StageName = "publish"
)

// CanonicalStages is the single source of truth for stage order.
var CanonicalStages = []StageName{
	StagePrepare,
	StageRender,
	StageEncode,
	StagePublish,
}

func (s StageName) Valid() bool {
	for _, candidate := range CanonicalStages {
		if candidate == s {
			return true
		}
	}
	return false
}

// Job is one batch entry loaded from jobs.json.
type Job struct {
	ID string `json:"id"`
	// SizeKB controls how many kilobytes the simulated render stage emits.
	SizeKB int `json:"size_kb,omitempty"`
	// FailStage, when non-empty, makes the job fail deterministically at
	// that stage. It models a bad input / encode error, not a process crash.
	FailStage string `json:"fail_stage,omitempty"`
}

// JobsFile is the top-level jobs.json document.
type JobsFile struct {
	Jobs []Job `json:"jobs"`
}

// Status values for JobState.
const (
	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// StageState is one committed checkpoint.
type StageState struct {
	Name       StageName `json:"name"`
	Done       bool      `json:"done"`
	Artifact   string    `json:"artifact,omitempty"`
	SHA256     string    `json:"sha256,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Attempt    int       `json:"attempt,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

// JobState is persisted atomically after every committed stage.
type JobState struct {
	SchemaVersion int                    `json:"schema_version"`
	JobID         string                 `json:"job_id"`
	Status        string                 `json:"status"`
	Attempt       int                    `json:"attempt"`
	Stages        map[string]*StageState `json:"stages"`
	// FinalName is the file name (inside OutDir) of the published artifact.
	FinalName string `json:"final_name"`
	// FinalDigest is the expected SHA-256 of the final artifact.
	FinalDigest string `json:"final_digest,omitempty"`
	// OverwriteAuthorized records explicit user confirmation for replacing
	// a previously successful artifact during this rebuild attempt.
	OverwriteAuthorized bool      `json:"overwrite_authorized,omitempty"`
	LastError           string    `json:"last_error,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Marker is the completion sidecar (<artifact>.ok). A final file without a
// matching Marker is never treated as a successful product.
type Marker struct {
	SchemaVersion int       `json:"schema_version"`
	JobID         string    `json:"job_id"`
	Artifact      string    `json:"artifact"`
	Size          int64     `json:"size"`
	SHA256        string    `json:"sha256"`
	Stages        []string  `json:"stages"`
	Attempts      int       `json:"attempts"`
	CompletedAt   time.Time `json:"completed_at"`
}

// Event kinds appended to the JSONL event log used for auditing and for the
// before/after comparison.
const (
	EventStageStart = "stage_start"
	EventStageDone  = "stage_done"
	EventStageSkip  = "stage_skip"
	EventJobDone    = "job_done"
	EventJobFailed  = "job_failed"
	EventJobSkip    = "job_skip"
	EventCrash      = "crash"
	EventQuarantine = "quarantine"
	EventOverwrite  = "overwrite_authorized"
)

// Event is one JSONL audit record.
type Event struct {
	TS      time.Time `json:"ts"`
	RunID   string    `json:"run_id"`
	PID     int       `json:"pid"`
	JobID   string    `json:"job_id,omitempty"`
	Stage   string    `json:"stage,omitempty"`
	Kind    string    `json:"kind"`
	Attempt int       `json:"attempt,omitempty"`
	Bytes   int64     `json:"bytes,omitempty"`
	Message string    `json:"message,omitempty"`
}
