package transcode

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
)

const maxStateFileSize = 8 << 20

func (job *preparedJob) stageDir(index int) string {
	return filepath.Join(job.layout.jobWork(job.key), "stages",
		fmt.Sprintf("%04d-%s", index, job.stages[index].Name))
}

func (job *preparedJob) checkpointPath(index int) string {
	return filepath.Join(job.layout.jobWork(job.key), "checkpoints",
		fmt.Sprintf("%04d-%s.json", index, job.stages[index].Name))
}

func scanArtifacts(ctx context.Context, dir string) ([]Artifact, error) {
	artifacts := make([]Artifact, 0)
	err := filepath.WalkDir(dir, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == dir {
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("transcode: symlink in stage output: %s", name)
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("transcode: symlink in stage output: %s", name)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("transcode: non-regular stage output: %s", name)
		}
		relative, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}
		relative, err = cleanRelative(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		digest, size, err := hashFile(ctx, name)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, Artifact{Path: relative, Size: size, SHA256: digest})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].Path < artifacts[j].Path
	})
	return artifacts, nil
}

func hasArtifact(artifacts []Artifact, name string) bool {
	for _, artifact := range artifacts {
		if artifact.Path == name {
			return true
		}
	}
	return false
}

func syncTree(root string) error {
	dirs := make([]string, 0)
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			dirs = append(dirs, name)
			return nil
		}
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		syncErr := file.Sync()
		closeErr := file.Close()
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	})
	if err != nil {
		return err
	}
	for index := len(dirs) - 1; index >= 0; index-- {
		if err := syncDir(dirs[index]); err != nil {
			return err
		}
	}
	return nil
}

func (job *preparedJob) commitStage(
	ctx context.Context,
	index int,
	tempDir string,
) (checkpoint, error) {
	stage := job.stages[index]
	artifacts, err := scanArtifacts(ctx, tempDir)
	if err != nil {
		return checkpoint{}, err
	}
	if stage.Primary != "" && !hasArtifact(artifacts, stage.Primary) {
		return checkpoint{}, fmt.Errorf("transcode: stage %q did not produce primary %q",
			stage.Name, stage.Primary)
	}
	if err := syncTree(tempDir); err != nil {
		return checkpoint{}, err
	}

	checkpointName := job.checkpointPath(index)
	if err := os.MkdirAll(filepath.Dir(checkpointName), 0o755); err != nil {
		return checkpoint{}, err
	}
	if err := os.Remove(checkpointName); err != nil && !os.IsNotExist(err) {
		return checkpoint{}, err
	}
	if err := syncDir(filepath.Dir(checkpointName)); err != nil && !os.IsNotExist(err) {
		return checkpoint{}, err
	}
	stageDir := job.stageDir(index)
	if err := os.MkdirAll(filepath.Dir(stageDir), 0o755); err != nil {
		return checkpoint{}, err
	}
	if err := os.RemoveAll(stageDir); err != nil {
		return checkpoint{}, err
	}
	if err := os.Rename(tempDir, stageDir); err != nil {
		return checkpoint{}, err
	}
	if err := syncDir(filepath.Dir(stageDir)); err != nil {
		return checkpoint{}, err
	}

	state := checkpoint{
		Format:       formatVersion,
		JobKey:       job.key,
		InputDigest:  job.inputDigest,
		PipelineHash: job.pipelineHash,
		StageIndex:   index,
		StageName:    stage.Name,
		StageVersion: stage.Version,
		Artifacts:    artifacts,
	}
	data, err := marshalJSON(state)
	if err != nil {
		return checkpoint{}, err
	}
	if err := writeFileAtomic(ctx, checkpointName, data, 0o644); err != nil {
		return checkpoint{}, err
	}
	return state, nil
}

func (job *preparedJob) loadCheckpoint(
	ctx context.Context,
	index int,
	stageDir string,
) (checkpoint, bool, error) {
	var state checkpoint
	if err := readJSONFile(job.checkpointPath(index), &state, maxStateFileSize); err != nil {
		return checkpoint{}, false, nil
	}
	stage := job.stages[index]
	if state.Format != formatVersion ||
		state.JobKey != job.key ||
		state.InputDigest != job.inputDigest ||
		state.PipelineHash != job.pipelineHash ||
		state.StageIndex != index ||
		state.StageName != stage.Name ||
		state.StageVersion != stage.Version {
		return checkpoint{}, false, nil
	}
	actual, err := scanArtifacts(ctx, stageDir)
	if err != nil {
		if ctx.Err() != nil {
			return checkpoint{}, false, ctx.Err()
		}
		return checkpoint{}, false, nil
	}
	if !slices.Equal(actual, state.Artifacts) {
		return checkpoint{}, false, nil
	}
	if stage.Primary != "" && !hasArtifact(actual, stage.Primary) {
		return checkpoint{}, false, nil
	}
	return state, true, nil
}

func (job *preparedJob) invalidateFrom(index int) error {
	for current := index; current < len(job.stages); current++ {
		if err := os.Remove(job.checkpointPath(current)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for current := index; current < len(job.stages); current++ {
		if err := os.RemoveAll(job.stageDir(current)); err != nil {
			return err
		}
	}
	return nil
}

func manifestPrimary(stage Stage) string {
	if stage.Primary == "" {
		return ""
	}
	return path.Join("stages", stage.Name, stage.Primary)
}
