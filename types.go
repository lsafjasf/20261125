package mediaidx

import (
	"encoding/hex"
	"errors"
	"fmt"
)

var (
	ErrClosed            = errors.New("mediaidx: index is closed")
	ErrTransactionActive = errors.New("mediaidx: transaction already active")
	ErrNoTransaction     = errors.New("mediaidx: no active transaction")
	ErrEmptyTransaction  = errors.New("mediaidx: transaction has no leaves")
	ErrDamaged           = errors.New("mediaidx: committed media is shorter than the index")
	ErrCorruptIndex      = errors.New("mediaidx: invalid index journal")
	ErrLeafNotFound      = errors.New("mediaidx: leaf not found")
	ErrInvalidLocator    = errors.New("mediaidx: invalid locator")
	ErrInvalidData       = errors.New("mediaidx: invalid data")
)

// Hash is a SHA-256 digest.
type Hash [32]byte

func (hash Hash) String() string {
	return hex.EncodeToString(hash[:])
}

func (hash Hash) MarshalText() ([]byte, error) {
	return []byte(hash.String()), nil
}

func (hash *Hash) UnmarshalText(text []byte) error {
	if len(text) != hex.EncodedLen(len(hash)) {
		return fmt.Errorf("mediaidx: hash must be %d hex characters", hex.EncodedLen(len(hash)))
	}
	_, err := hex.Decode(hash[:], text)
	return err
}

// Kind identifies the media coordinate carried by a leaf.
type Kind uint8

const (
	// Frame means Locator.Start is a zero-based video frame number.
	Frame Kind = 1
	// Sample means Locator.Start is a zero-based audio sample position.
	Sample Kind = 2
)

func (kind Kind) valid() bool {
	return kind == Frame || kind == Sample
}

func (kind Kind) String() string {
	switch kind {
	case Frame:
		return "frame"
	case Sample:
		return "sample"
	default:
		return fmt.Sprintf("kind(%d)", kind)
	}
}

func (kind Kind) MarshalText() ([]byte, error) {
	if !kind.valid() {
		return nil, fmt.Errorf("mediaidx: invalid kind %d", kind)
	}
	return []byte(kind.String()), nil
}

func (kind *Kind) UnmarshalText(text []byte) error {
	parsed, err := parseKindText(string(text))
	if err != nil {
		return err
	}
	*kind = parsed
	return nil
}

func parseKindText(text string) (Kind, error) {
	switch text {
	case "frame":
		return Frame, nil
	case "sample":
		return Sample, nil
	default:
		return 0, fmt.Errorf("mediaidx: unknown kind %q", text)
	}
}

// Locator is the media coordinate covered by a leaf. Frame leaves always have
// Count == 1 so damage maps to one frame; Sample leaves use Count as the number
// of samples in the segment.
type Locator struct {
	Kind  Kind   `json:"kind"`
	Start uint64 `json:"start"`
	Count uint64 `json:"count"`
}

func (loc Locator) End() (uint64, bool) {
	end := loc.Start + loc.Count
	return end, end >= loc.Start
}

func (loc Locator) String() string {
	switch loc.Kind {
	case Sample:
		return fmt.Sprintf("sample[%d:%d]", loc.Start, loc.Start+loc.Count)
	case Frame:
		if loc.Count == 1 {
			return fmt.Sprintf("frame[%d]", loc.Start)
		}
		return fmt.Sprintf("frame[%d:%d]", loc.Start, loc.Start+loc.Count)
	default:
		return fmt.Sprintf("kind(%d)[%d:%d]", loc.Kind, loc.Start, loc.Start+loc.Count)
	}
}

// Leaf describes one contiguous byte range in the media file and the media
// coordinate that range covers.
type Leaf struct {
	Seq        uint64  `json:"seq"`
	Locator    Locator `json:"locator"`
	DataOffset int64   `json:"data_offset"`
	DataSize   int64   `json:"data_size"`
	Digest     Hash    `json:"digest"`
}

func (leaf Leaf) end() int64 {
	return leaf.DataOffset + leaf.DataSize
}

// CommitInfo is the durable frontier of the index.
type CommitInfo struct {
	Number    uint64 `json:"number"`
	LeafCount uint64 `json:"leaf_count"`
	DataSize  int64  `json:"data_size"`
	Root      Hash   `json:"root"`
	ID        Hash   `json:"id"`
}

// Options controls file placement and durability. DataPath defaults to
// "<index path>.data". By default both the data and index are fsynced before a
// commit becomes visible.
type Options struct {
	DataPath string
	NoSync   bool
}

// Stats is a cheap snapshot that does not read the media file.
type Stats struct {
	Commit      CommitInfo `json:"commit"`
	LeafCount   uint64     `json:"leaf_count"`
	DataSize    int64      `json:"data_size"`
	IndexPath   string     `json:"index_path"`
	DataPath    string     `json:"data_path"`
	HasGap      bool       `json:"has_gap"`
	Damaged     bool       `json:"damaged"`
	Uncommitted bool       `json:"uncommitted"`
}

// IssueCode classifies a completeness problem.
type IssueCode string

const (
	IssueTailTruncated     IssueCode = "tail_truncated"
	IssueCorruptLeaf       IssueCode = "corrupt_leaf"
	IssueUncommittedSuffix IssueCode = "uncommitted_suffix"
	IssueUnitGap           IssueCode = "unit_gap"
)

// Issue points at an affected media unit and its byte range.
type Issue struct {
	Code         IssueCode `json:"code"`
	LeafSeq      uint64    `json:"leaf_seq"`
	Locator      Locator   `json:"locator"`
	DataOffset   int64     `json:"data_offset"`
	DataSize     int64     `json:"data_size"`
	Available    int64     `json:"available"`
	MissingBytes int64     `json:"missing_bytes,omitempty"`
	MissingUnits uint64    `json:"missing_units,omitempty"`
	ExtraBytes   int64     `json:"extra_bytes,omitempty"`
	Expected     Hash      `json:"expected"`
	Actual       Hash      `json:"actual"`
}

// Report is the result of verifying all committed leaves.
type Report struct {
	Complete       bool       `json:"complete"`
	Commit         CommitInfo `json:"commit"`
	LeafCount      uint64     `json:"leaf_count"`
	VerifiedLeaves uint64     `json:"verified_leaves"`
	DataSize       int64      `json:"data_size"`
	CommittedData  int64      `json:"committed_data_size"`
	ActualData     int64      `json:"actual_data_size"`
	HasGap         bool       `json:"has_gap"`
	FirstProblemAt uint64     `json:"first_problem_at,omitempty"`
	Issues         []Issue    `json:"issues,omitempty"`
}
