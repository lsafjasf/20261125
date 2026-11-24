package runner

import (
	"errors"
	"fmt"
	"time"

	"batchproc/internal/engine"
	"batchproc/internal/model"
)

func runJob(cfg Config, job model.Job, status ArtifactStatus) JobResult {
	result := JobResult{JobID: job.ID}
	out := cfg.out()

	state, err := loadStateOrNew(cfg, job)
	if err != nil {
		result.Outcome = OutcomeFailed
		result.Err = err
		return result
	}
	baseAttempt := state.Attempt

	// Fast path: a verified completion marker means a successful product.
	if status.MarkerValid {
		if !cfg.Force {
			state.Status = model.StatusCompleted
			state.LastError = ""
			if state.Attempt < 1 {
				state.Attempt = status.Marker.Attempts
			}
			_ = saveState(cfg, state)
			cfg.Events.Log(model.Event{
				Kind: model.EventJobSkip, JobID: job.ID,
				Message: "valid completion marker",
			})
			fmt.Fprintf(out, "✓ 任务 %-12s 已完成且 SHA-256 校验通过，跳过（重复执行幂等）\n", job.ID)
			result.Outcome = OutcomeSkipped
			result.Attempt = state.Attempt
			return result
		}
		// --force + explicit confirmation: start a fresh rebuild while the
		// old artifact stays untouched until publish atomically replaces it.
		state = newState(job, cfg.Now())
		state.Attempt = baseAttempt
		state.OverwriteAuthorized = true
		fmt.Fprintf(out, "↻ 任务 %-12s 已获显式确认，重新渲染全部阶段\n", job.ID)
	}

	state.Attempt++
	state.Status = model.StatusActive
	state.LastError = ""
	if err := saveState(cfg, state); err != nil {
		result.Outcome = OutcomeFailed
		result.Err = fmt.Errorf("persist state: %w", err)
		return result
	}
	result.Attempt = state.Attempt

	if removed, sweepErr := cfg.Layout.SweepTempFiles(); sweepErr == nil && len(removed) > 0 {
		fmt.Fprintf(out, "  🧹 清扫 %d 个上次中断遗留的临时文件\n", len(removed))
	}

	firstRerun := validateCheckpoints(cfg, state)
	if firstRerun > 0 {
		result.Resumed = true
	}

	for i, stage := range model.CanonicalStages {
		if i < firstRerun {
			result.StagesSkipped = append(result.StagesSkipped, string(stage))
			cfg.Events.Log(model.Event{
				Kind: model.EventStageSkip, JobID: job.ID,
				Stage: string(stage), Attempt: state.Attempt,
			})
			fmt.Fprintf(out, "  ↳ %-8s 命中检查点，跳过\n", stage)
			continue
		}

		if job.FailStage == string(stage) {
			taskErr := &engine.TaskError{
				JobID: job.ID, Stage: stage,
				Err: errors.New("注入的确定性任务失败（例如编码参数错误）"),
			}
			return failJob(cfg, state, result, stage, taskErr)
		}

		cfg.Events.Log(model.Event{
			Kind: model.EventStageStart, JobID: job.ID,
			Stage: string(stage), Attempt: state.Attempt,
		})
		started := time.Now()

		var size int64
		var digest string
		var stageErr error

		if stage == model.StagePublish {
			size, digest, stageErr = executePublish(cfg, job, state)
		} else {
			dest := cfg.Layout.StageArtifact(job.ID, stage)
			prepareDigest := digestOrEmpty(state.Stages[string(model.StagePrepare)])
			renderPath := cfg.Layout.StageArtifact(job.ID, model.StageRender)
			size, digest, stageErr = cfg.Engine.ProduceStage(
				job, stage, prepareDigest, renderPath, dest, cfg.Crash)
		}

		if stageErr != nil {
			if errors.Is(stageErr, engine.ErrSimulatedCrash) {
				cfg.Events.Log(model.Event{
					Kind: model.EventCrash, JobID: job.ID,
					Stage: string(stage), Attempt: state.Attempt,
					Message: stageErr.Error(),
				})
				result.Err = stageErr
				return result
			}
			return failJob(cfg, state, result, stage, stageErr)
		}

		abs := cfg.Layout.StageArtifact(job.ID, stage)
		if stage == model.StagePublish {
			abs = cfg.Layout.FinalPath(job)
			state.FinalDigest = digest
		}
		state.Stages[string(stage)] = &model.StageState{
			Name:       stage,
			Done:       true,
			Artifact:   cfg.Layout.RelativeToOut(abs),
			SHA256:     digest,
			Size:       size,
			Attempt:    state.Attempt,
			FinishedAt: cfg.Now(),
		}
		if stage == model.StageEncode {
			state.FinalDigest = digest
		}
		if err := saveState(cfg, state); err != nil {
			wrapped := fmt.Errorf("persist checkpoint: %w", err)
			return failJob(cfg, state, result, stage, wrapped)
		}

		elapsed := time.Since(started).Round(time.Millisecond)
		fmt.Fprintf(out, "\r  ✓ %-8s 完成 %10s  用时 %s\n", stage, formatBytes(size), elapsed)
		cfg.Events.Log(model.Event{
			Kind: model.EventStageDone, JobID: job.ID, Stage: string(stage),
			Attempt: state.Attempt, Bytes: size,
		})
		result.StagesRun = append(result.StagesRun, string(stage))
	}

	if err := writeCompletionMarker(cfg, job, state); err != nil {
		return failJob(cfg, state, result, model.StagePublish, fmt.Errorf("write marker: %w", err))
	}

	state.Status = model.StatusCompleted
	state.LastError = ""
	if err := saveState(cfg, state); err != nil {
		result.Outcome = OutcomeFailed
		result.Err = err
		return result
	}

	cfg.Events.Log(model.Event{
		Kind: model.EventJobDone, JobID: job.ID, Attempt: state.Attempt,
		Message: state.FinalDigest,
	})
	fmt.Fprintf(out, "🎉 任务 %-12s 完成（第 %d 次尝试，实际执行 %d 个阶段，跳过 %d 个）\n",
		job.ID, state.Attempt, len(result.StagesRun), len(result.StagesSkipped))
	result.Outcome = OutcomeCompleted
	result.Err = nil
	return result
}

func digestOrEmpty(st *model.StageState) string {
	if st == nil {
		return ""
	}
	return st.SHA256
}

func failJob(cfg Config, state *model.JobState, result JobResult, stage model.StageName, cause error) JobResult {
	state.Status = model.StatusFailed
	state.LastError = cause.Error()
	_ = saveState(cfg, state)
	cfg.Events.Log(model.Event{
		Kind: model.EventJobFailed, JobID: result.JobID,
		Stage: string(stage), Attempt: state.Attempt, Message: cause.Error(),
	})
	fmt.Fprintf(cfg.out(), "  ✗ %-8s 失败，已记录并隔离，后续任务继续: %v\n", stage, cause)
	result.Outcome = OutcomeFailed
	result.Err = cause
	return result
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
