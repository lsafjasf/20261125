package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const Schema = "tear-review/v1"

const (
	DecisionConfirmTear = "confirm_tear"
	DecisionClear       = "clear"
	DecisionUncertain   = "uncertain"
)

type File struct {
	Schema  string  `json:"schema"`
	Entries []Entry `json:"entries"`
}

type Entry struct {
	FrameID  string `json:"frame_id"`
	Decision string `json:"decision"`
	Note     string `json:"note,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`
}

func NewFile() *File {
	return &File{Schema: Schema, Entries: []Entry{}}
}

func Load(path string) (*File, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read review file: %w", err)
	}
	var file File
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, "", fmt.Errorf("parse review file: %w", err)
	}
	if file.Schema != "" && file.Schema != Schema {
		return nil, "", fmt.Errorf("unsupported review schema %q", file.Schema)
	}
	normalized, err := Normalize(&file)
	if err != nil {
		return nil, "", err
	}
	hash, err := Hash(normalized)
	if err != nil {
		return nil, "", err
	}
	return normalized, hash, nil
}

func Normalize(file *File) (*File, error) {
	if file == nil {
		return NewFile(), nil
	}
	seen := make(map[string]bool, len(file.Entries))
	entries := make([]Entry, 0, len(file.Entries))
	for index, entry := range file.Entries {
		frameID := strings.TrimSpace(entry.FrameID)
		if frameID == "" {
			return nil, fmt.Errorf("review entry %d has no frame_id", index)
		}
		if seen[frameID] {
			return nil, fmt.Errorf("duplicate review entry for frame %q", frameID)
		}
		seen[frameID] = true
		decision, err := CanonicalDecision(entry.Decision)
		if err != nil {
			return nil, fmt.Errorf("review entry for frame %q: %w", frameID, err)
		}
		entries = append(entries, Entry{
			FrameID:  frameID,
			Decision: decision,
			Note:     strings.TrimSpace(entry.Note),
			Reviewer: strings.TrimSpace(entry.Reviewer),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].FrameID < entries[j].FrameID
	})
	return &File{Schema: Schema, Entries: entries}, nil
}

func CanonicalDecision(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "confirm", "confirm_tear", "confirmed", "tear", "yes", "y":
		return DecisionConfirmTear, nil
	case "clear", "cleared", "not_tear", "no", "n":
		return DecisionClear, nil
	case "uncertain", "unsure", "maybe":
		return DecisionUncertain, nil
	default:
		return "", fmt.Errorf("unknown decision %q (use confirm_tear, clear, or uncertain)", value)
	}
}

func Hash(file *File) (string, error) {
	normalized, err := Normalize(file)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("canonicalize review file: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func Map(file *File) (map[string]Entry, error) {
	normalized, err := Normalize(file)
	if err != nil {
		return nil, err
	}
	result := make(map[string]Entry, len(normalized.Entries))
	for _, entry := range normalized.Entries {
		result[entry.FrameID] = entry
	}
	return result, nil
}

func Set(file *File, entry Entry) (*File, error) {
	normalized, err := Normalize(file)
	if err != nil {
		return nil, err
	}
	decision, err := CanonicalDecision(entry.Decision)
	if err != nil {
		return nil, err
	}
	entry.Decision = decision
	entry.FrameID = strings.TrimSpace(entry.FrameID)
	entry.Note = strings.TrimSpace(entry.Note)
	entry.Reviewer = strings.TrimSpace(entry.Reviewer)
	if entry.FrameID == "" {
		return nil, fmt.Errorf("frame_id is required")
	}
	for index := range normalized.Entries {
		if normalized.Entries[index].FrameID == entry.FrameID {
			normalized.Entries[index] = entry
			return Normalize(normalized)
		}
	}
	normalized.Entries = append(normalized.Entries, entry)
	return Normalize(normalized)
}

func Remove(file *File, frameID string) (*File, error) {
	normalized, err := Normalize(file)
	if err != nil {
		return nil, err
	}
	frameID = strings.TrimSpace(frameID)
	entries := normalized.Entries[:0]
	for _, entry := range normalized.Entries {
		if entry.FrameID != frameID {
			entries = append(entries, entry)
		}
	}
	normalized.Entries = entries
	return Normalize(normalized)
}

func Marshal(file *File) ([]byte, error) {
	normalized, err := Normalize(file)
	if err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func WriteAtomic(path string, file *File) error {
	raw, err := Marshal(file)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".tear-review-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary review file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(raw); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary review file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary review file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace review file: %w", err)
	}
	return nil
}
