package transcode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Run executes a job and returns after a complete object has been published.
// Calling Run again for the same input and parameters reuses that object.
func (e *Engine) Run(ctx context.Context, request Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	job, err := e.prepare(ctx, request)
	if err != nil {
		return Result{}, err
	}
	lock, err := acquireFileLock(ctx, job.layout.jobLock(job.key))
	if err != nil {
		return Result{}, err
	}
	defer lock.release()

	expectedDigest, hasCompletion := job.readCompletion()
	if hasCompletion {
		if manifest, err := job.validateObject(ctx, job.layout.objectDir(expectedDigest)); err == nil {
			return job.result(manifest, expectedDigest, true), nil
		} else if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
	}

	if err := job.prepareWork(); err != nil {
		return Result{}, err
	}
	snapshot, err := job.ensureSnapshot(ctx)
	if err != nil {
		return Result{}, err
	}

	stageDirs := make([]string, len(job.stages))
	states := make([]checkpoint, len(job.stages))
	resumeAt := 0
	for index := range job.stages {
		dir := job.stageDir(index)
		state, valid, err := job.loadCheckpoint(ctx, index, dir)
		if err != nil {
			return Result{}, err
		}
		if !valid {
			resumeAt = index
			break
		}
		stageDirs[index] = dir
		states[index] = state
		resumeAt = index + 1
	}
	if resumeAt < len(job.stages) {
		if err := job.invalidateFrom(resumeAt); err != nil {
			return Result{}, err
		}
		for index := resumeAt; index < len(job.stages); index++ {
			state, dir, err := job.runStage(ctx, index, snapshot, stageDirs)
			if err != nil {
				return Result{}, err
			}
			stageDirs[index] = dir
			states[index] = state
		}
	}

	built, err := job.buildObject(ctx, stageDirs, states)
	if err != nil {
		return Result{}, err
	}
	reused, err := job.publishObject(ctx, built, expectedDigest)
	if err != nil {
		_ = os.RemoveAll(built.tempDir)
		return Result{}, err
	}
	if err := job.writeCompletion(ctx, built.digest); err != nil {
		return Result{}, err
	}
	return job.result(built.manifest, built.digest, reused), nil
}

func (job *preparedJob) prepareWork() error {
	work := job.layout.jobWork(job.key)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	temp := filepath.Join(work, "tmp")
	if err := os.RemoveAll(temp); err != nil {
		return err
	}
	return os.MkdirAll(temp, 0o755)
}

func (job *preparedJob) runStage(
	ctx context.Context,
	index int,
	snapshot string,
	stageDirs []string,
) (checkpoint, string, error) {
	stage := job.stages[index]
	tempDir, err := os.MkdirTemp(filepath.Join(job.layout.jobWork(job.key), "tmp"), "stage-*")
	if err != nil {
		return checkpoint{}, "", err
	}
	defer os.RemoveAll(tempDir)

	previous := make(map[string]string, index)
	for prior := 0; prior < index; prior++ {
		previous[job.stages[prior].Name] = stageDirs[prior]
	}
	input := StageInput{
		InputPath:   snapshot,
		InputDigest: job.inputDigest,
		InputSize:   job.inputSize,
		Parameters:  cloneParameters(job.parameters),
		Previous:    previous,
		OutputDir:   tempDir,
	}
	if err := stage.Run(ctx, input); err != nil {
		return checkpoint{}, "", err
	}
	if err := ctx.Err(); err != nil {
		return checkpoint{}, "", err
	}
	state, err := job.commitStage(ctx, index, tempDir)
	if err != nil {
		return checkpoint{}, "", err
	}
	return state, job.stageDir(index), nil
}

// RunWithRetry retries Run with bounded exponential backoff. Cancellation of
// ctx stops both the current attempt and any pending backoff.
func (e *Engine) RunWithRetry(ctx context.Context, request Request, policy RetryPolicy) (Result, error) {
	attempts := policy.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}
	wait := policy.InitialWait
	if wait <= 0 {
		wait = 100 * time.Millisecond
	}
	maxWait := policy.MaxWait
	if maxWait <= 0 {
		maxWait = 2 * time.Second
	}
	if wait > maxWait {
		wait = maxWait
	}
	retryable := policy.Retryable
	if retryable == nil {
		retryable = defaultRetryable
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		result, err := e.Run(ctx, request)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt == attempts || !retryable(err) {
			return Result{}, err
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Result{}, ctx.Err()
		case <-timer.C:
		}
		wait *= 2
		if wait > maxWait {
			wait = maxWait
		}
	}
	return Result{}, lastErr
}

func defaultRetryable(err error) bool {
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, ErrInvalidInput) ||
		errors.Is(err, ErrNondeterministic) {
		return false
	}
	return true
}
