package runner

import (
	"errors"
	"fmt"
	"os"

	"batchproc/internal/engine"
	"batchproc/internal/model"
	"batchproc/internal/store"
)

// executePublish makes the encode candidate become the final artifact.
//
// Recovery matrix for an existing final path:
//   - bytes match the candidate digest -> already published, adopt it;
//   - bytes differ but a valid marker proves a prior SUCCESSFUL version ->
//     replace only when overwrite was explicitly authorized;
//   - bytes differ without a valid marker -> truncated/corrupt/legacy file,
//     quarantine it and publish fresh bytes.
func executePublish(cfg Config, job model.Job, state *model.JobState) (int64, string, error) {
	encodeStage := state.Stages[string(model.StageEncode)]
	if encodeStage == nil || !encodeStage.Done {
		return 0, "", errors.New("publish without completed encode checkpoint")
	}
	candidatePath := cfg.Layout.ResolveOut(encodeStage.Artifact)
	expectedDigest := encodeStage.SHA256
	finalPath := cfg.Layout.FinalPath(job)

	if info, err := os.Stat(finalPath); err == nil && !info.IsDir() {
		actualSize, actualDigest, hashErr := store.FileSHA256(finalPath)
		if hashErr != nil {
			return 0, "", hashErr
		}
		if actualDigest == expectedDigest {
			return actualSize, actualDigest, nil // adopt renamed-but-unmarked output
		}

		existing := InspectArtifact(cfg.Layout, job)
		switch {
		case existing.MarkerValid && state.OverwriteAuthorized:
			fmt.Fprintf(cfg.out(), "  ⚠ 发布将原子替换已确认覆盖的旧产物 %s\n", cfg.Layout.RelativeToOut(finalPath))
		case existing.MarkerValid:
			return 0, "", &engine.TaskError{
				JobID: job.ID, Stage: model.StagePublish,
				Err: errors.New("拒绝覆盖仍带有效完成标记的成功产物；如需重跑请加 --force 并显式确认"),
			}
		default:
			dest, qerr := cfg.Layout.Quarantine(finalPath, "truncated")
			if qerr != nil {
				return 0, "", qerr
			}
			cfg.Events.Log(model.Event{
				Kind: model.EventQuarantine, JobID: job.ID,
				Stage:   string(model.StagePublish),
				Message: "unmarked final moved to " + dest,
			})
			fmt.Fprintf(cfg.out(), "  ⚠ 发现无完成标记的可疑/截断文件，已隔离到 %s\n", dest)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, "", err
	}

	candidate, err := os.Open(candidatePath)
	if err != nil {
		return 0, "", err
	}
	size, digest, err := store.AtomicWriteReader(finalPath, candidate, 0o644)
	_ = candidate.Close()
	if err != nil {
		return 0, "", err
	}
	if digest != expectedDigest {
		return 0, "", errors.New("published artifact digest does not match encode checkpoint")
	}
	// Any stale sidecar now describes replaced bytes; the fresh marker is
	// written once the whole stage loop has committed.
	_ = os.Remove(cfg.Layout.MarkerPath(finalPath))
	return size, digest, nil
}

// writeCompletionMarker atomically writes the .ok sidecar with size and
// SHA-256. The marker is the only authoritative "finished product" signal.
func writeCompletionMarker(cfg Config, job model.Job, state *model.JobState) error {
	publishStage := state.Stages[string(model.StagePublish)]
	if publishStage == nil {
		return errors.New("cannot write marker without publish checkpoint")
	}
	finalPath := cfg.Layout.FinalPath(job)
	size, digest, err := store.FileSHA256(finalPath)
	if err != nil {
		return err
	}
	if digest != publishStage.SHA256 || digest != state.FinalDigest {
		return errors.New("final artifact digest disagrees with checkpoint")
	}

	stageNames := make([]string, 0, len(model.CanonicalStages))
	for _, name := range model.CanonicalStages {
		stageNames = append(stageNames, string(name))
	}
	marker := model.Marker{
		SchemaVersion: model.SchemaVersion,
		JobID:         job.ID,
		Artifact:      cfg.Layout.RelativeToOut(finalPath),
		Size:          size,
		SHA256:        digest,
		Stages:        stageNames,
		Attempts:      state.Attempt,
		CompletedAt:   cfg.Now(),
	}
	return store.WriteJSONAtomic(cfg.Layout.MarkerPath(finalPath), marker)
}
