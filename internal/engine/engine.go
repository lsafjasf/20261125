// Package engine contains the simulated rendering stages. Every output is
// streamed into a temp file and atomically committed; a Kill hook models a
// hard process crash mid-write.
package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"batchproc/internal/model"
	"batchproc/internal/store"
)

// DefaultChunkSize is the streaming write granularity.
const DefaultChunkSize = 128 << 10

// ErrSimulatedCrash is returned by the in-test Kill hook. The production
// hook terminates the process directly, so this only surfaces in tests.
var ErrSimulatedCrash = errors.New("simulated hard crash")

// TaskError marks a deterministic per-job failure. The batch runner catches
// it, records the job as failed and continues with the next job.
type TaskError struct {
	JobID string
	Stage model.StageName
	Err   error
}

func (e *TaskError) Error() string {
	return fmt.Sprintf("job %s failed at stage %s: %v", e.JobID, e.Stage, e.Err)
}

func (e *TaskError) Unwrap() error { return e.Err }

// CrashRule says "crash while writing Stage for JobID".
type CrashRule struct {
	JobID      string
	Stage      model.StageName
	AfterBytes int64 // zero means "after the first written chunk"
}

// ParseCrashSpec parses "job-id:stage-name". Empty spec means no crash.
func ParseCrashSpec(spec string) (CrashRule, error) {
	if spec == "" {
		return CrashRule{}, nil
	}
	var rule CrashRule
	for i := 0; i < len(spec); i++ {
		if spec[i] == ':' {
			rule.JobID = spec[:i]
			stage := model.StageName(spec[i+1:])
			if rule.JobID == "" || !stage.Valid() {
				return CrashRule{}, fmt.Errorf("invalid --crash %q (want job-id:prepare|render|encode|publish)", spec)
			}
			rule.Stage = stage
			return rule, nil
		}
	}
	return CrashRule{}, fmt.Errorf("invalid --crash %q (want job-id:stage)", spec)
}

func (r CrashRule) matches(jobID string, stage model.StageName) bool {
	return r.JobID != "" && r.JobID == jobID && r.Stage == stage
}

// KillFunc simulates abrupt termination. Returning ErrSimulatedCrash keeps
// the in-process test alive; the CLI wires a hook that calls os.Exit(137).
type KillFunc func(jobID string, stage model.StageName, written int64) error

// Engine executes rendering stages.
type Engine struct {
	ChunkSize  int
	ChunkDelay time.Duration
	Kill       KillFunc
}

// New returns an Engine with sane defaults.
func New() *Engine {
	return &Engine{ChunkSize: DefaultChunkSize}
}

func (e *Engine) chunkSize() int {
	if e.ChunkSize > 0 {
		return e.ChunkSize
	}
	return DefaultChunkSize
}

const prepareSize = 64 << 10

func jobSeed(jobID string, prepareDigest string) int64 {
	h := sha256.New()
	_, _ = h.Write([]byte(jobID))
	_, _ = h.Write([]byte(prepareDigest))
	sum := h.Sum(nil)
	return int64(binary.BigEndian.Uint64(sum[:8]))
}

// ProduceStage generates the artifact for stage and atomically renames it to
// dest. renderPath is the committed render artifact for the encode stage.
func (e *Engine) ProduceStage(job model.Job, stage model.StageName, prepareDigest, renderPath, dest string, rule CrashRule) (int64, string, error) {
	sizeKB := job.SizeKB
	if sizeKB <= 0 {
		sizeKB = 2048
	}
	var src io.Reader
	switch stage {
	case model.StagePrepare:
		src = newPatternReader(job.ID, "PREPARE", prepareSize, 0x11)
	case model.StageRender:
		src = rand.New(rand.NewSource(jobSeed(job.ID, prepareDigest)))
		src = io.LimitReader(src, int64(sizeKB)*1024)
	case model.StageEncode:
		f, err := os.Open(renderPath)
		if err != nil {
			return 0, "", fmt.Errorf("open render artifact: %w", err)
		}
		defer f.Close()
		src = newEncodeReader(f, job.ID)
	default:
		return 0, "", fmt.Errorf("engine cannot produce stage %q", stage)
	}
	return e.atomicStream(dest, src, rule, job.ID, stage)
}

// atomicStream streams src into a temp file next to dest, then atomically
// renames it. On a simulated kill the temp file is deliberately left on disk,
// mirroring what a real crash leaves behind.
func (e *Engine) atomicStream(dest string, src io.Reader, rule CrashRule, jobID string, stage model.StageName) (int64, string, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".tmp-"+filepath.Base(dest)+"-*")
	if err != nil {
		return 0, "", err
	}
	tmpName := tmp.Name()

	buf := make([]byte, e.chunkSize())
	hasher := sha256.New()
	var written int64
	crashed := false

	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if e.ChunkDelay > 0 {
				time.Sleep(e.ChunkDelay)
			}
			if _, err := tmp.Write(buf[:n]); err != nil {
				_ = tmp.Close()
				_ = os.Remove(tmpName)
				return written, "", err
			}
			hasher.Write(buf[:n])
			written += int64(n)
			if !crashed && rule.matches(jobID, stage) {
				threshold := rule.AfterBytes
				if threshold <= 0 {
					threshold = 1
				}
				if written >= threshold {
					crashed = true
					if e.Kill != nil {
						if killErr := e.Kill(jobID, stage, written); killErr != nil {
							_ = tmp.Close() // FD closed by OS on real death; file stays
							return written, "", killErr
						}
					}
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
			return written, "", readErr
		}
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return 0, "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return 0, "", err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		_ = os.Remove(tmpName)
		return 0, "", err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		_ = os.Remove(tmpName)
		return 0, "", err
	}
	_ = store.DirFSync(filepath.Dir(dest))
	return written, hex.EncodeToString(hasher.Sum(nil)), nil
}
