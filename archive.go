// Package ffv1arc implements an append-only FFV1-style weekly segment archive
// with an incremental integrity checker.
//
// The archive stores one sealed segment per weekly append inside seg/, while a
// JSON manifest in index/manifest.json records block state and frame indices.
// Frame index entries are only recomputed for the blocks that actually changed,
// and sealed blocks can never be rewritten through the archive API.
package ffv1arc

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Errors returned by archive operations. They are wrapped, so callers should
// use errors.Is rather than direct equality.
var (
	ErrBlockSealed      = errors.New("ffv1arc: block is already sealed and cannot be rewritten")
	ErrBlockIncomplete  = errors.New("ffv1arc: block is incomplete, run Recover before appending again")
	ErrBlockNotFound    = errors.New("ffv1arc: block not found")
	ErrDuplicateAppend  = errors.New("ffv1arc: identical fragment was already appended")
	ErrBlockIDCollision = errors.New("ffv1arc: block id already exists")
	ErrBlockStillOpen   = errors.New("ffv1arc: another block is still open, recover it first")
	ErrInvalidSegment   = errors.New("ffv1arc: malformed segment file")
	ErrEmptyFragment    = errors.New("ffv1arc: fragment contains no frames")
	ErrRenameInProgress = errors.New("ffv1arc: cannot rename an incomplete block")
	ErrInvalidName      = errors.New("ffv1arc: segment name must be a plain file name")
)

// FrameRec is the per-frame index entry stored in the manifest.
type FrameRec struct {
	Number int    `json:"number"`
	Offset int64  `json:"offset"`
	Length int    `json:"length"`
	Hash   string `json:"hash"`
}

// BlockState describes one weekly segment in the manifest.
type BlockState struct {
	ID           int        `json:"id"`
	Chapter      string     `json:"chapter"`
	SegmentName  string     `json:"segment_name"`
	Frames       []FrameRec `json:"frames"`
	PayloadHash  string     `json:"payload_hash"`
	PayloadBytes int64      `json:"payload_bytes"`
	Sealed       bool       `json:"sealed"`
}

// Manifest is the durable archive index.
type Manifest struct {
	Version int           `json:"version"`
	Blocks  []*BlockState `json:"blocks"`
}

// Archive is an append-only weekly segment archive.
type Archive struct {
	root string
	mu   sync.Mutex
	man  *Manifest
}

const (
	manifestVersion = 1
	manifestRelPath = "index" + string(filepath.Separator) + "manifest.json"
	segDir          = "seg"
)

func hashB64(b []byte) string {
	sum := sha256.Sum256(b)
	return base64Std(sum[:])
}

func base64Std(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func (a *Archive) segPath(name string) string {
	return filepath.Join(a.root, segDir, name)
}

func (a *Archive) manifestPath() string {
	return filepath.Join(a.root, manifestRelPath)
}

// Open opens (or initializes) an archive at root.
func Open(root string) (*Archive, error) {
	if err := os.MkdirAll(filepath.Join(root, segDir), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "index"), 0o755); err != nil {
		return nil, err
	}
	a := &Archive{root: root, man: &Manifest{Version: manifestVersion}}
	data, err := os.ReadFile(filepath.Join(root, manifestRelPath))
	switch {
	case err == nil:
		if err := json.Unmarshal(data, a.man); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSegment, err)
		}
		if a.man.Version != manifestVersion {
			return nil, fmt.Errorf("%w: unsupported manifest version %d", ErrInvalidSegment, a.man.Version)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := a.saveLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	return a, nil
}

// saveLocked atomically persists the manifest. Callers must hold a.mu.
func (a *Archive) saveLocked() error {
	data, err := json.MarshalIndent(a.man, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(a.root, "index")
	tmp, err := os.CreateTemp(dir, "manifest-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, a.manifestPath())
}

func (a *Archive) findLocked(id int) *BlockState {
	for _, b := range a.man.Blocks {
		if b.ID == id {
			return b
		}
	}
	return nil
}

func (a *Archive) nextIDLocked() int {
	max := 0
	for _, b := range a.man.Blocks {
		if b.ID > max {
			max = b.ID
		}
	}
	return max + 1
}

func putU16(b []byte, v uint16) { binary.BigEndian.PutUint16(b, v) }
func putU32(b []byte, v uint32) { binary.BigEndian.PutUint32(b, v) }
