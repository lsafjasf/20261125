package runner

import (
	"errors"
	"os"
	"time"

	"batchproc/internal/model"
	"batchproc/internal/store"
)

func newState(job model.Job, now time.Time) *model.JobState {
	return &model.JobState{
		SchemaVersion: model.SchemaVersion,
		JobID:         job.ID,
		Status:        model.StatusActive,
		Attempt:       0,
		Stages:        map[string]*model.StageState{},
		FinalName:     store.FinalName(job.ID),
		UpdatedAt:     now,
	}
}

// loadStateOrNew reads persisted stage state. A missing file yields a fresh
// state; a corrupt document is quarantined rather than trusted.
func loadStateOrNew(cfg Config, job model.Job) (*model.JobState, error) {
	path := cfg.Layout.StatePath(job.ID)
	var state model.JobState
	if err := store.ReadJSON(path, &state); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newState(job, cfg.Now()), nil
		}
		if dest, qerr := cfg.Layout.Quarantine(path, "badstate"); qerr == nil {
			cfg.Events.Log(model.Event{
				Kind: model.EventQuarantine, JobID: job.ID,
				Message: "corrupt state moved to " + dest,
			})
		}
		fresh := newState(job, cfg.Now())
		return fresh, nil
	}
	if state.Stages == nil {
		state.Stages = map[string]*model.StageState{}
	}
	if state.Attempt < 0 {
		state.Attempt = 0
	}
	return &state, nil
}

func saveState(cfg Config, state *model.JobState) error {
	state.SchemaVersion = model.SchemaVersion
	state.UpdatedAt = cfg.Now()
	return store.WriteJSONAtomic(cfg.Layout.StatePath(state.JobID), state)
}

// validateCheckpoints verifies every recorded stage artifact against its
// size and SHA-256. The first invalid checkpoint invalidates it and every
// later stage (linear pipeline), returning the index of the first stage to
// (re)run.
func validateCheckpoints(cfg Config, state *model.JobState) int {
	firstRerun := len(model.CanonicalStages)
	for i, name := range model.CanonicalStages {
		st := state.Stages[string(name)]
		if i >= firstRerun {
			delete(state.Stages, string(name))
			continue
		}
		if st == nil || !st.Done || st.SHA256 == "" {
			firstRerun = i
			continue
		}
		abs := cfg.Layout.ResolveOut(st.Artifact)
		size, digest, err := store.FileSHA256(abs)
		if err != nil || size != st.Size || digest != st.SHA256 {
			delete(state.Stages, string(name))
			firstRerun = i
		}
	}
	return firstRerun
}
