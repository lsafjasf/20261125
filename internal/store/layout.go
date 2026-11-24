// Package store owns the on-disk layout: state, temp/work directories,
// completion markers and atomic file helpers.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"batchproc/internal/model"
)

const (
	metaDirName       = ".batchproc"
	stateDirName      = "state"
	tmpDirName        = "tmp"
	workDirName       = "work"
	quarantineDirName = "quarantine"
	eventsFileName    = "events.jsonl"
)

// Layout centralizes every path used by the processor.
type Layout struct {
	OutDir        string
	MetaDir       string
	StateDir      string
	TempRoot      string
	WorkRoot      string
	QuarantineDir string
	EventsPath    string
}

// NewLayout resolves OutDir and ensures all metadata directories exist.
func NewLayout(outDir string) (*Layout, error) {
	abs, err := filepath.Abs(outDir)
	if err != nil {
		return nil, fmt.Errorf("resolve output dir: %w", err)
	}
	l := &Layout{
		OutDir:        abs,
		MetaDir:       filepath.Join(abs, metaDirName),
		StateDir:      filepath.Join(abs, metaDirName, stateDirName),
		TempRoot:      filepath.Join(abs, metaDirName, tmpDirName),
		WorkRoot:      filepath.Join(abs, metaDirName, workDirName),
		QuarantineDir: filepath.Join(abs, metaDirName, quarantineDirName),
		EventsPath:    filepath.Join(abs, metaDirName, eventsFileName),
	}
	for _, dir := range []string{
		l.OutDir, l.MetaDir, l.StateDir, l.TempRoot,
		l.WorkRoot, l.QuarantineDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return l, nil
}

// FinalPath returns the published artifact path for a job.
func (l *Layout) FinalPath(job model.Job) string {
	return filepath.Join(l.OutDir, FinalName(job.ID))
}

// FinalName is the artifact file name for a job id.
func FinalName(jobID string) string {
	return jobID + ".mp4"
}

// MarkerPath is the completion sidecar for an artifact.
func (l *Layout) MarkerPath(finalPath string) string {
	return finalPath + ".ok"
}

// StatePath is the per-job stage state document.
func (l *Layout) StatePath(jobID string) string {
	return filepath.Join(l.StateDir, jobID+".json")
}

// JobTempRoot holds all attempt temp directories for one job.
func (l *Layout) JobTempRoot(jobID string) string {
	return filepath.Join(l.TempRoot, jobID)
}

// AttemptTempDir is the private temp directory of one attempt.
func (l *Layout) AttemptTempDir(jobID string, attempt int) (string, error) {
	dir := filepath.Join(l.JobTempRoot(jobID), fmt.Sprintf("attempt-%d", attempt))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// StageArtifact is the committed intermediate output of a completed stage.
func (l *Layout) StageArtifact(jobID string, stage model.StageName) string {
	return filepath.Join(l.WorkRoot, jobID+"."+string(stage)+".bin")
}

// RelativeToOut converts a path below OutDir to a slash-separated relative
// name suitable for storing in state.
func (l *Layout) RelativeToOut(path string) string {
	rel, err := filepath.Rel(l.OutDir, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// ResolveOut reverses RelativeToOut.
func (l *Layout) ResolveOut(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(l.OutDir, filepath.FromSlash(rel))
}

// IsNotExist reports whether err is a "no such file" error.
func IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
