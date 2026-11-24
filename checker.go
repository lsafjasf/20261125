package ffv1arc

import (
	"fmt"
	"os"
	"path/filepath"
)

// AlarmKind categorizes integrity findings.
type AlarmKind string

const (
	AlarmPayloadTampered AlarmKind = "payload_tampered"
	AlarmNameChanged     AlarmKind = "segment_name_changed"
	AlarmChapterChanged  AlarmKind = "chapter_changed"
	AlarmSegmentMissing  AlarmKind = "segment_missing"
	AlarmBlockIncomplete AlarmKind = "block_incomplete"
	AlarmUnregistered    AlarmKind = "unregistered_segment"
	AlarmIndexDrift      AlarmKind = "frame_index_drift"
	AlarmCorruptSegment  AlarmKind = "corrupt_segment"
)

// Alarm is a single integrity finding. Kinds are kept distinct so callers can
// react differently to payload edits, renames and chapter edits.
type Alarm struct {
	Kind    AlarmKind
	BlockID int
	Detail  string
}

func (al Alarm) Error() string {
	if al.BlockID == 0 {
		return fmt.Sprintf("ffv1arc alarm %s: %s", al.Kind, al.Detail)
	}
	return fmt.Sprintf("ffv1arc alarm %s (block %d): %s", al.Kind, al.BlockID, al.Detail)
}

// CheckReport aggregates a checker run.
type CheckReport struct {
	Alarms          []Alarm
	ReindexedBlocks []int
	ReindexedFrames int
	CheckedBlocks   int
}

// HasAlarm reports whether any alarm of the given kinds fired.
func (r *CheckReport) HasAlarm(kinds ...AlarmKind) bool {
	for _, al := range r.Alarms {
		for _, k := range kinds {
			if al.Kind == k {
				return true
			}
		}
	}
	return false
}

// Check verifies the whole archive. Frame indices are recomputed only for
// blocks whose on-disk layout drifted; payload tampering is reported, never
// silently absorbed into the index.
func (a *Archive) Check() (*CheckReport, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	rep := &CheckReport{}
	seen := make(map[string]bool)
	persist := false

	for _, b := range a.man.Blocks {
		rep.CheckedBlocks++
		seen[b.SegmentName] = true

		path := a.segPath(b.SegmentName)
		ps, err := parseSegment(path)
		if os.IsNotExist(err) {
			if alt := a.locateByIDLocked(b.ID); alt != "" {
				rep.Alarms = append(rep.Alarms, Alarm{
					Kind: AlarmNameChanged, BlockID: b.ID,
					Detail: fmt.Sprintf("expected %q, found %q", b.SegmentName, alt),
				})
				seen[alt] = true
				path = a.segPath(alt)
				ps, err = parseSegment(path)
			} else {
				rep.Alarms = append(rep.Alarms, Alarm{
					Kind: AlarmSegmentMissing, BlockID: b.ID,
					Detail: fmt.Sprintf("no segment for block %d", b.ID),
				})
				continue
			}
		}
		if err != nil {
			rep.Alarms = append(rep.Alarms, Alarm{Kind: AlarmCorruptSegment, BlockID: b.ID, Detail: err.Error()})
			continue
		}

		if ps.chapter != b.Chapter {
			rep.Alarms = append(rep.Alarms, Alarm{
				Kind: AlarmChapterChanged, BlockID: b.ID,
				Detail: fmt.Sprintf("manifest chapter %q, segment chapter %q", b.Chapter, ps.chapter),
			})
		}

		// Frame-by-frame payload verification: content edits raise alarms but
		// structural index drift (offset/length/frame count) is repairable.
		drift := len(b.Frames) != len(ps.frames)
		for i, fr := range b.Frames {
			if i >= len(ps.frames) {
				drift = true
				break
			}
			if fr.Number != ps.frames[i].Number || fr.Offset != ps.frames[i].Offset || fr.Length != ps.frames[i].Length {
				drift = true
			}
			if fr.Hash != ps.frames[i].Hash {
				rep.Alarms = append(rep.Alarms, Alarm{
					Kind: AlarmPayloadTampered, BlockID: b.ID,
					Detail: fmt.Sprintf("frame %d hash mismatch", fr.Number),
				})
			}
		}
		if drift {
			rep.Alarms = append(rep.Alarms, Alarm{
				Kind: AlarmIndexDrift, BlockID: b.ID,
				Detail: "recomputing frame index from segment",
			})
			rep.ReindexedBlocks = append(rep.ReindexedBlocks, b.ID)
			rep.ReindexedFrames += mergeIndexLocked(b, ps)
			persist = true
		}

		if !ps.sealed {
			rep.Alarms = append(rep.Alarms, Alarm{
				Kind: AlarmBlockIncomplete, BlockID: b.ID,
				Detail: "segment has no SEAL footer; run Recover/Resume",
			})
		} else if !b.Sealed {
			rep.Alarms = append(rep.Alarms, Alarm{
				Kind: AlarmBlockIncomplete, BlockID: b.ID,
				Detail: "segment is sealed on disk but manifest block is open",
			})
			b.Sealed = true
			persist = true
		}
	}

	entries, err := os.ReadDir(filepath.Join(a.root, segDir))
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || seen[e.Name()] {
				continue
			}
			rep.Alarms = append(rep.Alarms, Alarm{
				Kind:   AlarmUnregistered,
				Detail: fmt.Sprintf("segment file %q is not in the manifest", e.Name()),
			})
		}
	}

	if persist {
		if err := a.saveLocked(); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

// mergeIndexLocked updates only structural frame fields from the parsed
// segment, preserving payload hashes recorded in the manifest so tampered
// payloads keep alarming instead of being silently re-baselined.
func mergeIndexLocked(b *BlockState, ps *parsedSegment) int {
	merged := make([]FrameRec, len(ps.frames))
	for i, fr := range ps.frames {
		merged[i] = FrameRec{Number: fr.Number, Offset: fr.Offset, Length: fr.Length, Hash: fr.Hash}
		if i < len(b.Frames) {
			merged[i].Hash = b.Frames[i].Hash
		}
	}
	b.Frames = merged
	return len(merged)
}

// locateByIDLocked finds a segment on disk whose embedded block id matches,
// which is how an out-of-band rename is detected.
func (a *Archive) locateByIDLocked(id int) string {
	entries, err := os.ReadDir(filepath.Join(a.root, segDir))
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ps, err := parseSegment(a.segPath(e.Name()))
		if err != nil {
			continue
		}
		if ps.blockID == id {
			return e.Name()
		}
	}
	return ""
}

// Recover reconciles interrupted appends: it reparses every open block's
// segment and rebuilds only that block's frame index. It never seals a block,
// because sealing requires the caller to supply the rest of the fragment via
// Resume.
type RecoverReport struct {
	RecoveredBlocks []int
	ReindexedFrames int
	TruncatedFrames map[int]int
}

// Recover reconciles the manifest against disk after crashes.
func (a *Archive) Recover() (*RecoverReport, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	rep := &RecoverReport{TruncatedFrames: map[int]int{}}
	persist := false
	for _, b := range a.man.Blocks {
		if b.Sealed {
			continue
		}
		ps, err := parseSegment(a.segPath(b.SegmentName))
		if err != nil {
			return rep, fmt.Errorf("recover block %d: %w", b.ID, err)
		}
		n, changed := a.reindexBlockLocked(b, ps)
		rep.TruncatedFrames[b.ID] = n
		rep.RecoveredBlocks = append(rep.RecoveredBlocks, b.ID)
		rep.ReindexedFrames += n
		if changed {
			persist = true
		}
	}
	if persist {
		if err := a.saveLocked(); err != nil {
			return rep, err
		}
	}
	return rep, nil
}
