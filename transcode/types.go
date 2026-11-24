package transcode

import (
	"context"
	"errors"
	"time"
)

const formatVersion = 1

var (
	// ErrNondeterministic means a retry produced different output for the
	// same content-addressed job. Stage versions must be bumped when their
	// output changes.
	ErrNondeterministic = errors.New("transcode: stage output is not deterministic")

	// ErrInputChanged means the input file changed while it was being
	// snapshotted. A retry can be used to read a stable version.
	ErrInputChanged = errors.New("transcode: input changed while being read")

	// ErrInvalidInput reports a request or pipeline that can never succeed.
	ErrInvalidInput = errors.New("transcode: invalid input")
)

// Stage is one resumable step in a transcode pipeline. Run must write all of
// its output below Input.OutputDir and must honor Input.Context.
//
// Version is part of the content address. Bump it whenever code or
// configuration can change the bytes produced by the stage.
type Stage struct {
	Name    string
	Version string
	Primary string
	Run     func(context.Context, StageInput) error
}

// StageInput contains a stable snapshot of the source and committed outputs
// from earlier stages.
type StageInput struct {
	InputPath   string
	InputDigest string
	InputSize   int64
	Parameters  map[string]string
	Previous    map[string]string
	OutputDir   string
}

// Artifact identifies one regular file in a committed stage or object.
// Path is always slash-separated and relative.
type Artifact struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest describes a completed, content-addressed output object.
type Manifest struct {
	Format       int        `json:"format"`
	JobKey       string     `json:"jobKey"`
	InputDigest  string     `json:"inputDigest"`
	PipelineHash string     `json:"pipelineHash"`
	Primary      string     `json:"primary,omitempty"`
	Artifacts    []Artifact `json:"artifacts"`
}

// Result is returned for both new and reused outputs.
type Result struct {
	Key          string     `json:"key"`
	ObjectDigest string     `json:"objectDigest"`
	OutputDir    string     `json:"outputDir"`
	PrimaryPath  string     `json:"primaryPath,omitempty"`
	Artifacts    []Artifact `json:"artifacts"`
	Reused       bool       `json:"reused"`
}

// Request identifies an input and deterministic transcode parameters.
type Request struct {
	InputPath  string
	Parameters map[string]string
}

// RetryPolicy controls idempotent retries. Run is safe to retry because a
// failed attempt never publishes a checkpoint or a final object.
type RetryPolicy struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
	Retryable   func(error) bool
}

type checkpoint struct {
	Format       int        `json:"format"`
	JobKey       string     `json:"jobKey"`
	InputDigest  string     `json:"inputDigest"`
	PipelineHash string     `json:"pipelineHash"`
	StageIndex   int        `json:"stageIndex"`
	StageName    string     `json:"stageName"`
	StageVersion string     `json:"stageVersion"`
	Artifacts    []Artifact `json:"artifacts"`
}

type completion struct {
	Format       int    `json:"format"`
	JobKey       string `json:"jobKey"`
	ObjectDigest string `json:"objectDigest"`
}
