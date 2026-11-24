package transcode

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type builtObject struct {
	tempDir  string
	digest   string
	manifest Manifest
}

func (job *preparedJob) buildObject(
	ctx context.Context,
	stageDirs []string,
	states []checkpoint,
) (*builtObject, error) {
	tempDir, err := os.MkdirTemp(filepath.Join(job.layout.jobWork(job.key), "tmp"), "object-*")
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(tempDir)
		}
	}()

	manifest := Manifest{
		Format:       formatVersion,
		JobKey:       job.key,
		InputDigest:  job.inputDigest,
		PipelineHash: job.pipelineHash,
		Artifacts:    make([]Artifact, 0),
	}
	for index, stage := range job.stages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, artifact := range states[index].Artifacts {
			objectPath := path.Join("stages", stage.Name, artifact.Path)
			source := filepath.Join(stageDirs[index], filepath.FromSlash(artifact.Path))
			target := filepath.Join(tempDir, filepath.FromSlash(objectPath))
			if err := copyVerifiedFile(ctx, source, target, artifact); err != nil {
				return nil, err
			}
			manifest.Artifacts = append(manifest.Artifacts, Artifact{
				Path:   objectPath,
				Size:   artifact.Size,
				SHA256: artifact.SHA256,
			})
		}
	}
	if len(job.stages) > 0 {
		last := len(job.stages) - 1
		manifest.Primary = manifestPrimary(job.stages[last])
		if manifest.Primary != "" && !hasArtifact(manifest.Artifacts, manifest.Primary) {
			return nil, fmt.Errorf("transcode: missing primary artifact %q", manifest.Primary)
		}
	}
	data, err := marshalJSON(manifest)
	if err != nil {
		return nil, err
	}
	if err := syncTree(tempDir); err != nil {
		return nil, err
	}
	if err := writeFileAtomic(ctx, filepath.Join(tempDir, "manifest.json"), data, 0o644); err != nil {
		return nil, err
	}
	digest := digestBytes(data)
	keep = true
	return &builtObject{
		tempDir:  tempDir,
		digest:   digest,
		manifest: manifest,
	}, nil
}

func copyVerifiedFile(ctx context.Context, source, target string, artifact Artifact) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("transcode: artifact %q is not a regular file", source)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	digest, size, copyErr := copyWithHash(ctx, src, dst)
	if copyErr == nil {
		copyErr = dst.Sync()
	}
	closeErr := dst.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return copyErr
	}
	if digest != artifact.SHA256 || size != artifact.Size {
		return fmt.Errorf("transcode: artifact %q changed before publication", source)
	}
	return os.Chmod(target, 0o644)
}

func validDigest(digest string) bool {
	if len(digest) != 64 || digest != strings.ToLower(digest) {
		return false
	}
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == sha256Size
}

const sha256Size = 32

func (job *preparedJob) publishObject(
	ctx context.Context,
	built *builtObject,
	expectedDigest string,
) (bool, error) {
	if expectedDigest != "" && built.digest != expectedDigest {
		return false, fmt.Errorf("%w: job %s expected object %s, rebuilt %s",
			ErrNondeterministic, job.key, expectedDigest, built.digest)
	}
	lock, err := acquireFileLock(ctx, job.layout.objectLock(built.digest))
	if err != nil {
		return false, err
	}
	defer lock.release()

	destination := job.layout.objectDir(built.digest)
	if _, err := job.validateObject(ctx, destination); err == nil {
		_ = os.RemoveAll(built.tempDir)
		return true, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := os.RemoveAll(destination); err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return false, err
	}
	if err := os.Rename(built.tempDir, destination); err != nil {
		return false, err
	}
	if err := syncDir(filepath.Dir(destination)); err != nil {
		return false, err
	}
	if _, err := job.validateObject(ctx, destination); err != nil {
		return false, fmt.Errorf("transcode: published object failed validation: %w", err)
	}
	return false, nil
}
