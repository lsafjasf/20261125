package runner

import (
	"errors"
	"os"
	"path/filepath"

	"batchproc/internal/model"
	"batchproc/internal/store"
)

// ArtifactStatus is the verified state of a job's published artifact.
type ArtifactStatus struct {
	FinalPath    string
	Exists       bool
	ActualSize   int64
	ActualDigest string
	Marker       *model.Marker
	// MarkerValid is true only when the sidecar parses AND its size and
	// SHA-256 match the bytes currently on disk.
	MarkerValid bool
}

// InspectArtifact verifies final file and completion marker. A truncated
// file, a missing marker or a digest mismatch all yield MarkerValid=false.
func InspectArtifact(l *store.Layout, job model.Job) ArtifactStatus {
	finalPath := l.FinalPath(job)
	st := ArtifactStatus{FinalPath: finalPath}

	info, err := os.Stat(finalPath)
	if err != nil {
		return st
	}
	if info.IsDir() {
		return st
	}
	st.Exists = true

	size, digest, err := store.FileSHA256(finalPath)
	if err != nil {
		return st
	}
	st.ActualSize = size
	st.ActualDigest = digest

	var marker model.Marker
	if err := store.ReadJSON(l.MarkerPath(finalPath), &marker); err != nil {
		return st
	}
	st.Marker = &marker
	st.MarkerValid = marker.SHA256 == digest && marker.Size == size &&
		marker.JobID == job.ID
	return st
}

// VerificationReport is the `verify` subcommand result for one job.
type VerificationReport struct {
	Job    model.Job
	Status ArtifactStatus
	OK     bool
	Reason string
}

// VerifyAll checks every job and also reports leftover temp files.
func VerifyAll(l *store.Layout, jobs []model.Job) ([]VerificationReport, []string, error) {
	var reports []VerificationReport
	for _, job := range jobs {
		st := InspectArtifact(l, job)
		rep := VerificationReport{Job: job, Status: st}
		switch {
		case !st.Exists:
			rep.Reason = "缺少最终产物"
		case st.Marker == nil:
			rep.Reason = "缺少完成标记（可能是截断文件）"
		case !st.MarkerValid:
			rep.Reason = "标记与产物不一致（截断或损坏）"
		default:
			rep.OK = true
			rep.Reason = "OK"
		}
		reports = append(reports, rep)
	}

	temps, err := findTempFiles(l)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return reports, nil, err
	}
	return reports, temps, nil
}

func findTempFiles(l *store.Layout) ([]string, error) {
	var temps []string
	err := walkTemp(l.OutDir, &temps)
	return temps, err
}

func walkTemp(root string, temps *[]string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			if entry.Name() == "quarantine" {
				continue
			}
			if err := walkTemp(path, temps); err != nil {
				return err
			}
			continue
		}
		name := entry.Name()
		if len(name) > 5 && name[:5] == ".tmp-" {
			*temps = append(*temps, path)
		}
	}
	return nil
}
