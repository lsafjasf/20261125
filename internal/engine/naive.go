package engine

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"batchproc/internal/model"
	"batchproc/internal/store"
)

// NaiveConfig runs the legacy processor behaviour used for the before/after
// comparison: it truncates the final artifact with os.Create, writes every
// stage directly into it and keeps no stage state or completion marker.
// Every restart therefore renders all stages from scratch, and a crash
// leaves a truncated <id>.mp4 behind.
type NaiveConfig struct {
	Layout *store.Layout
	Events *store.EventLogger
	Engine *Engine
	Job    model.Job
	Crash  CrashRule
}

// RunNaive executes one job with legacy semantics.
func RunNaive(cfg NaiveConfig) error {
	eng := cfg.Engine
	if eng == nil {
		eng = New()
	}
	job := cfg.Job
	finalPath := cfg.Layout.FinalPath(job)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return err
	}

	// THE BUG: os.Create truncates the published artifact immediately and
	// all stages append straight into the final file.
	f, err := os.Create(finalPath)
	if err != nil {
		return err
	}

	runStage := func(stage model.StageName, src io.Reader) error {
		cfg.Events.Log(model.Event{Kind: model.EventStageStart, JobID: job.ID, Stage: string(stage)})
		buf := make([]byte, eng.chunkSize())
		var written int64
		crashed := false
		for {
			n, rerr := src.Read(buf)
			if n > 0 {
				if _, werr := f.Write(buf[:n]); werr != nil {
					return werr
				}
				written += int64(n)
				if !crashed && cfg.Crash.matches(job.ID, stage) {
					crashed = true
					if eng.Kill != nil {
						if kerr := eng.Kill(job.ID, stage, written); kerr != nil {
							_ = f.Close() // leaves truncated final on disk
							return kerr
						}
					}
				}
			}
			if errors.Is(rerr, io.EOF) {
				break
			}
			if rerr != nil {
				return rerr
			}
		}
		_ = written
		cfg.Events.Log(model.Event{Kind: model.EventStageDone, JobID: job.ID, Stage: string(stage)})
		return nil
	}

	sizeKB := job.SizeKB
	if sizeKB <= 0 {
		sizeKB = 2048
	}

	if err := runStage(model.StagePrepare,
		newPatternReader(job.ID, "PREPARE", prepareSize, 0x11)); err != nil {
		return err
	}
	if err := runStage(model.StageRender,
		newPatternReader(job.ID, "RENDER", int64(sizeKB)*1024, 0x22)); err != nil {
		return err
	}
	// Legacy encode is another raw pass appended to the same file.
	if err := runStage(model.StageEncode,
		newPatternReader(job.ID, "ENCODE", int64(sizeKB)*512, 0x33)); err != nil {
		return err
	}

	_ = f.Sync()
	if err := f.Close(); err != nil {
		return err
	}
	// No marker is ever written by the legacy processor.
	cfg.Events.Log(model.Event{Kind: model.EventStageDone, JobID: job.ID, Stage: string(model.StagePublish)})
	return nil
}
