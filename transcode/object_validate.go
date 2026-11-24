package transcode

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

func validateObject(ctx context.Context, dir, expectedJobKey string) (Manifest, error) {
	var manifest Manifest
	info, err := os.Lstat(dir)
	if err != nil {
		return manifest, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return manifest, fmt.Errorf("transcode: object %q is not a directory", dir)
	}
	data, err := readLimitedFile(filepath.Join(dir, "manifest.json"), maxStateFileSize)
	if err != nil {
		return manifest, err
	}
	if digestBytes(data) != filepath.Base(dir) {
		return manifest, fmt.Errorf("transcode: manifest digest does not match object path %q", dir)
	}
	if err := decodeJSON(bytes.NewReader(data), &manifest); err != nil {
		return manifest, err
	}
	if manifest.Format != formatVersion || !validDigest(manifest.JobKey) ||
		!validDigest(manifest.InputDigest) || !validDigest(manifest.PipelineHash) {
		return manifest, fmt.Errorf("transcode: invalid object manifest")
	}
	if expectedJobKey != "" && manifest.JobKey != expectedJobKey {
		return manifest, fmt.Errorf("transcode: object belongs to job %s, want %s",
			manifest.JobKey, expectedJobKey)
	}
	if manifest.Primary != "" {
		primary, err := cleanRelative(manifest.Primary)
		if err != nil || primary != manifest.Primary {
			return manifest, fmt.Errorf("transcode: unsafe primary path %q", manifest.Primary)
		}
	}
	seen := make(map[string]bool, len(manifest.Artifacts))
	expectedFiles := map[string]bool{"manifest.json": true}
	expectedDirs := make(map[string]bool)
	for _, artifact := range manifest.Artifacts {
		if err := ctx.Err(); err != nil {
			return manifest, err
		}
		cleaned, err := cleanRelative(artifact.Path)
		if err != nil || cleaned != artifact.Path || seen[artifact.Path] ||
			!validDigest(artifact.SHA256) {
			return manifest, fmt.Errorf("transcode: invalid artifact entry %q", artifact.Path)
		}
		seen[artifact.Path] = true
		expectedFiles[artifact.Path] = true
		for dir := path.Dir(artifact.Path); dir != "."; dir = path.Dir(dir) {
			expectedDirs[dir] = true
		}
	}
	if err := validateObjectTree(ctx, dir, expectedFiles, expectedDirs); err != nil {
		return manifest, err
	}
	for _, artifact := range manifest.Artifacts {
		if err := ctx.Err(); err != nil {
			return manifest, err
		}
		name := filepath.Join(dir, filepath.FromSlash(artifact.Path))
		fileInfo, err := os.Lstat(name)
		if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Size() != artifact.Size {
			return manifest, fmt.Errorf("transcode: artifact %q is missing or has wrong size", artifact.Path)
		}
		digest, size, err := hashFile(ctx, name)
		if err != nil {
			return manifest, err
		}
		if digest != artifact.SHA256 || size != artifact.Size {
			return manifest, fmt.Errorf("transcode: artifact %q failed checksum", artifact.Path)
		}
	}
	if manifest.Primary != "" && !seen[manifest.Primary] {
		return manifest, fmt.Errorf("transcode: primary artifact %q is absent", manifest.Primary)
	}
	return manifest, nil
}

func (job *preparedJob) validateObject(ctx context.Context, dir string) (Manifest, error) {
	manifest, err := validateObject(ctx, dir, job.key)
	if err != nil {
		return manifest, err
	}
	if manifest.InputDigest != job.inputDigest || manifest.PipelineHash != job.pipelineHash {
		return manifest, fmt.Errorf("transcode: object manifest does not match job %s", job.key)
	}
	return manifest, nil
}

func validateObjectTree(
	ctx context.Context,
	root string,
	expectedFiles map[string]bool,
	expectedDirs map[string]bool,
) error {
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == root {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("transcode: unexpected symlink %q in object", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			if !expectedDirs[relative] {
				return fmt.Errorf("transcode: unexpected directory %q in object", relative)
			}
			return nil
		}
		if !info.Mode().IsRegular() || !expectedFiles[relative] {
			return fmt.Errorf("transcode: unexpected file %q in object", relative)
		}
		return nil
	})
}

func readLimitedFile(name string, limit int64) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("transcode: %q exceeds size limit", name)
	}
	return data, nil
}

func (job *preparedJob) readCompletion() (string, bool) {
	var marker completion
	if err := readJSONFile(job.layout.jobMarker(job.key), &marker, maxStateFileSize); err != nil {
		return "", false
	}
	if marker.Format != formatVersion || marker.JobKey != job.key || !validDigest(marker.ObjectDigest) {
		return "", false
	}
	return marker.ObjectDigest, true
}

func (job *preparedJob) writeCompletion(ctx context.Context, digest string) error {
	marker := completion{
		Format:       formatVersion,
		JobKey:       job.key,
		ObjectDigest: digest,
	}
	data, err := marshalJSON(marker)
	if err != nil {
		return err
	}
	return writeFileAtomic(ctx, job.layout.jobMarker(job.key), data, 0o644)
}

func (job *preparedJob) result(manifest Manifest, digest string, reused bool) Result {
	outputDir := job.layout.objectDir(digest)
	artifacts := append([]Artifact(nil), manifest.Artifacts...)
	result := Result{
		Key:          job.key,
		ObjectDigest: digest,
		OutputDir:    outputDir,
		Artifacts:    artifacts,
		Reused:       reused,
	}
	if manifest.Primary != "" {
		result.PrimaryPath = filepath.Join(outputDir, filepath.FromSlash(manifest.Primary))
	}
	return result
}
