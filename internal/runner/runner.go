// Package runner orchestrates batch execution: the overwrite gate, per-job
// isolation, stage checkpoint recovery and the final summary.
package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"batchproc/internal/confirm"
	"batchproc/internal/engine"
	"batchproc/internal/model"
	"batchproc/internal/store"
)

// ErrOverwriteConfirmation is returned when --force cannot obtain explicit
// confirmation (non-interactive stdin without --yes, or answer != yes).
var ErrOverwriteConfirmation = errors.New("overwrite not explicitly confirmed")

// Outcome values for JobResult.
const (
	OutcomeCompleted = "completed"
	OutcomeSkipped   = "skipped"
	OutcomeFailed    = "failed"
)

// JobResult describes the final disposition of one job.
type JobResult struct {
	JobID         string
	Outcome       string
	Attempt       int
	Resumed       bool
	StagesRun     []string
	StagesSkipped []string
	Err           error
}

// Summary is the batch result.
type Summary struct {
	Results   []JobResult
	Completed int
	Skipped   int
	Failed    int
}

// Config configures a batch run.
type Config struct {
	Jobs     []model.Job
	Layout   *store.Layout
	Events   *store.EventLogger
	Engine   *engine.Engine
	Prompter *confirm.Prompter
	Out      io.Writer
	Force    bool
	Crash    engine.CrashRule
	Now      func() time.Time
}

func (c *Config) out() io.Writer {
	if c.Out != nil {
		return c.Out
	}
	return io.Discard
}

// Run executes the batch. Ordinary per-job errors are captured in the
// Summary and execution continues; only confirmation failure and simulated
// hard crashes escape the loop.
func Run(cfg Config) (*Summary, error) {
	if cfg.Layout == nil {
		return nil, errors.New("runner: Layout is required")
	}
	if cfg.Engine == nil {
		cfg.Engine = engine.New()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Prompter == nil {
		cfg.Prompter = &confirm.Prompter{In: os.Stdin, Out: os.Stdout}
	}

	// Batch-wide explicit overwrite gate, resolved BEFORE any work starts.
	var overwriteTargets []confirm.Target
	statuses := make(map[string]ArtifactStatus, len(cfg.Jobs))
	for _, job := range cfg.Jobs {
		st := InspectArtifact(cfg.Layout, job)
		statuses[job.ID] = st
		if st.MarkerValid {
			overwriteTargets = append(overwriteTargets, confirm.Target{
				JobID:    job.ID,
				Artifact: cfg.Layout.RelativeToOut(st.FinalPath),
				SHA256:   st.ActualDigest,
			})
		}
	}
	if cfg.Force && len(overwriteTargets) > 0 {
		approved, err := cfg.Prompter.ConfirmOverwrite(overwriteTargets)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrOverwriteConfirmation, err)
		}
		if !approved {
			return nil, ErrOverwriteConfirmation
		}
		cfg.Events.Log(model.Event{
			Kind:    model.EventOverwrite,
			Message: fmt.Sprintf("explicit confirmation for %d artifact(s)", len(overwriteTargets)),
		})
	}

	summary := &Summary{}
	for _, job := range cfg.Jobs {
		result := runJobSafely(cfg, job, statuses[job.ID])
		summary.Results = append(summary.Results, result)
		switch result.Outcome {
		case OutcomeCompleted:
			summary.Completed++
		case OutcomeSkipped:
			summary.Skipped++
		case OutcomeFailed:
			summary.Failed++
		}
		// A simulated hard crash terminates the process-like execution;
		// nothing after this point in the batch runs in this "lifetime".
		if errors.Is(result.Err, engine.ErrSimulatedCrash) {
			return summary, result.Err
		}
	}
	return summary, nil
}

// runJobSafely isolates panics so one pathological job cannot kill the batch.
func runJobSafely(cfg Config, job model.Job, status ArtifactStatus) (result JobResult) {
	defer func() {
		if rec := recover(); rec != nil {
			result.Outcome = OutcomeFailed
			result.Err = fmt.Errorf("panic: %v", rec)
			fmt.Fprintf(cfg.out(), "  ✗ 任务 %s 发生 panic，已隔离: %v\n", job.ID, rec)
			cfg.Events.Log(model.Event{Kind: model.EventJobFailed, JobID: job.ID, Message: result.Err.Error()})
			persistFailure(cfg, job, result.Err)
		}
	}()
	return runJob(cfg, job, status)
}

func persistFailure(cfg Config, job model.Job, runErr error) {
	state, err := loadStateOrNew(cfg, job)
	if err != nil {
		return
	}
	state.Status = model.StatusFailed
	state.LastError = runErr.Error()
	state.UpdatedAt = cfg.Now()
	_ = store.WriteJSONAtomic(cfg.Layout.StatePath(job.ID), state)
}

// LoadJobs reads and validates a jobs.json document.
func LoadJobs(path string) ([]model.Job, error) {
	var doc model.JobsFile
	if err := store.ReadJSON(path, &doc); err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for i := range doc.Jobs {
		job := &doc.Jobs[i]
		if job.ID == "" {
			return nil, fmt.Errorf("jobs[%d]: id is required", i)
		}
		if seen[job.ID] {
			return nil, fmt.Errorf("duplicate job id %q", job.ID)
		}
		seen[job.ID] = true
		if job.FailStage != "" && !model.StageName(job.FailStage).Valid() {
			return nil, fmt.Errorf("job %s: unknown fail_stage %q", job.ID, job.FailStage)
		}
	}
	return doc.Jobs, nil
}
