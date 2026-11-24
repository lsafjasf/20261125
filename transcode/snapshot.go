package transcode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (job *preparedJob) ensureSnapshot(ctx context.Context) (string, error) {
	inputDir := filepath.Join(job.layout.jobWork(job.key), "input")
	snapshot := filepath.Join(inputDir, "source")
	if digest, size, err := hashFile(ctx, snapshot); err == nil {
		if digest == job.inputDigest && size == job.inputSize {
			return snapshot, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		return "", err
	}
	tempDir := filepath.Join(job.layout.jobWork(job.key), "tmp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(tempDir, "input-*")
	if err != nil {
		return "", err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	source, err := os.Open(job.inputPath)
	if err != nil {
		temp.Close()
		return "", err
	}
	digest, size, copyErr := copyWithHash(ctx, source, temp)
	closeErr := source.Close()
	if copyErr != nil {
		temp.Close()
		return "", copyErr
	}
	if closeErr != nil {
		temp.Close()
		return "", closeErr
	}
	if digest != job.inputDigest || size != job.inputSize {
		temp.Close()
		return "", fmt.Errorf("%w: expected %s (%d bytes), got %s (%d bytes)",
			ErrInputChanged, job.inputDigest, job.inputSize, digest, size)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.Remove(snapshot); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(tempName, snapshot); err != nil {
		return "", err
	}
	if err := syncDir(inputDir); err != nil {
		return "", err
	}
	return snapshot, nil
}
