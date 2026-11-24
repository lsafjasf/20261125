package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type backupInfo struct {
	path string
	hash string
	size int64
}

func readWAVFile(path string) (*wavFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	w, err := parseWAV(file)
	if err != nil {
		return nil, err
	}
	return w, nil
}

func createBackup(path string, now time.Time) (backupInfo, error) {
	original, err := os.ReadFile(path)
	if err != nil {
		return backupInfo{}, err
	}
	base := filepath.Base(path)
	backupBase := filepath.Join(filepath.Dir(path), base+".bak-"+now.UTC().Format("20060102T150405.000000000Z"))
	backupPath := backupBase
	for attempt := 1; ; attempt++ {
		if _, err := os.Stat(backupPath); errors.Is(err, os.ErrNotExist) {
			break
		} else if err != nil {
			return backupInfo{}, err
		}
		backupPath = fmt.Sprintf("%s.%d", backupBase, attempt)
	}
	if err := os.WriteFile(backupPath, original, 0o600); err != nil {
		return backupInfo{}, err
	}
	sum := sha256.Sum256(original)
	return backupInfo{
		path: backupPath,
		hash: hex.EncodeToString(sum[:]),
		size: int64(len(original)),
	}, nil
}

func atomicWrite(path string, content []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	rollbackTemp := true
	defer func() {
		if rollbackTemp {
			temp.Close()
			os.Remove(tempPath)
		}
	}()

	if _, err := temp.Write(content); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	rollbackTemp = false
	return syncDirectory(dir)
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func readback(path string, expected []byte) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	expectedSum := sha256.Sum256(expected)
	expectedHash := hex.EncodeToString(expectedSum[:])
	if actual != expectedHash {
		return actual, fmt.Errorf("readback hash mismatch: got %s want %s", actual, expectedHash)
	}
	return actual, nil
}

func rollback(target, backupPath string) (string, error) {
	if backupPath == "" {
		latest, err := latestBackup(target)
		if err != nil {
			return "", err
		}
		backupPath = latest
	}
	content, err := os.ReadFile(backupPath)
	if err != nil {
		return "", err
	}
	if _, err := parseWAV(strings.NewReader(string(content))); err != nil {
		return "", fmt.Errorf("backup is not a valid WAV: %w", err)
	}
	if err := atomicWrite(target, content); err != nil {
		return "", err
	}
	return hashBytes(content), nil
}

func latestBackup(target string) (string, error) {
	matches, err := filepath.Glob(target + ".bak-*")
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no backup found for %s", target)
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}
