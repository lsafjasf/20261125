package repair

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"adlabel/internal/authority"
	"adlabel/internal/catalog"
)

// Engine runs a correction batch against one loaded library.
type Engine struct {
	lib          *catalog.Library
	opts         authority.Options
	allOrNothing bool
	dryRun       bool
}

// NewEngine builds an Engine. allOrNothing makes every group share one
// transaction: any failure rolls back every group already staged.
func NewEngine(lib *catalog.Library, opts authority.Options, allOrNothing, dryRun bool) *Engine {
	if opts.DurationEpsilon <= 0 {
		opts = authority.DefaultOptions()
	}
	return &Engine{lib: lib, opts: opts, allOrNothing: allOrNothing, dryRun: dryRun}
}

// stagedGroup holds everything needed to commit or undo one group.
type stagedGroup struct {
	result  *GroupResult
	entry   *catalog.Entry
	newData []byte
	mode    os.FileMode
	backup  []byte
}

// Run validates and executes the batch and returns the detailed report.
func (e *Engine) Run(corrections []catalog.Correction) (*Report, error) {
	started := time.Now().UTC()
	report := &Report{
		LibraryRoot:  e.lib.Root,
		StartedAt:    started,
		AllOrNothing: e.allOrNothing,
		DryRun:       e.dryRun,
	}

	groups, groupOrder, err := groupCorrections(corrections)
	if err != nil {
		report.FinishedAt = time.Now().UTC()
		report.Aborted = true
		return report, err
	}
	report.Summary.TotalGroups = len(groupOrder)

	var staged []*stagedGroup
	rollbackAll := func(failedAt int, reason string) {
		for i := len(staged) - 1; i >= 0; i-- {
			e.undo(staged[i])
		}
		for idx, sg := range staged {
			sg.result.Status = StatusRolledBack
			sg.result.FailureReason = fmt.Sprintf("batch abort after group %q: %s",
				groupOrder[failedAt], reason)
			report.Summary.RolledBack++
			report.Groups[idx] = *sg.result
		}
		// Nothing survives an all-or-nothing abort: reset applied counters.
		report.Summary.Applied = 0
		report.Summary.ChangedFields = 0
		staged = nil
	}

	for idx, groupID := range groupOrder {
		sg, perr := e.prepare(groupID, groups[groupID])
		report.Groups = append(report.Groups, *sg.result)
		if perr != nil {
			sg.result.Status = StatusRolledBack
			sg.result.FailureReason = perr.Error()
			report.Groups[len(report.Groups)-1] = *sg.result
			if e.allOrNothing {
				// rollbackAll marks and counts every group uniformly.
				rollbackAll(idx, perr.Error())
				report.Aborted = true
				break
			}
			report.Summary.RolledBack++
			continue
		}

		if e.dryRun {
			sg.result.Status = StatusSkipped
			sg.result.FailureReason = "dry_run: validated but not written"
			report.Groups[len(report.Groups)-1] = *sg.result
			report.Summary.Skipped++
			continue
		}

		if werr := e.commit(sg); werr != nil {
			e.undo(sg)
			sg.result.Status = StatusRolledBack
			sg.result.FailureReason = werr.Error()
			report.Groups[len(report.Groups)-1] = *sg.result
			if e.allOrNothing {
				// rollbackAll marks and counts every group uniformly.
				rollbackAll(idx, werr.Error())
				report.Aborted = true
				break
			}
			report.Summary.RolledBack++
			continue
		}

		sg.result.Status = StatusApplied
		sg.result.AppliedAt = time.Now().UTC()
		report.Groups[len(report.Groups)-1] = *sg.result
		report.Summary.Applied++
		report.Summary.ChangedFields += len(sg.result.Fields)
		staged = append(staged, sg)
	}

	report.FinishedAt = time.Now().UTC()
	return report, nil
}

// groupCorrections buckets corrections by group id and enforces group shape:
// each group targets one media id and contains both mutable fields exactly
// once, and contains no other field.
func groupCorrections(corrections []catalog.Correction) (map[string][]catalog.Correction, []string, error) {
	groups := map[string][]catalog.Correction{}
	var order []string
	seen := map[string]bool{}

	for _, corr := range corrections {
		if corr.GroupID == "" {
			return nil, nil, fmt.Errorf("correction for %q field %q has empty group_id", corr.MediaID, corr.Field)
		}
		if !seen[corr.GroupID] {
			seen[corr.GroupID] = true
			order = append(order, corr.GroupID)
		}
		groups[corr.GroupID] = append(groups[corr.GroupID], corr)
	}

	for _, groupID := range order {
		bucket := groups[groupID]
		mediaIDs := map[string]bool{}
		fields := map[string]int{}
		for _, corr := range bucket {
			mediaIDs[corr.MediaID] = true
			fields[corr.Field]++
		}
		if len(mediaIDs) != 1 {
			return nil, nil, fmt.Errorf("group %q spans multiple media_ids", groupID)
		}
		if len(bucket) != 2 {
			return nil, nil, fmt.Errorf("group %q must contain exactly 2 fields, got %d", groupID, len(bucket))
		}
		for _, field := range []string{catalog.FieldLanguage, catalog.FieldDuration} {
			if fields[field] != 1 {
				return nil, nil, fmt.Errorf("group %q must contain %q exactly once", groupID, field)
			}
		}
	}

	// One media item may appear in at most one group; otherwise the second
	// group would build on a stale in-memory record after the first commit.
	targeted := map[string]string{}
	for _, groupID := range order {
		mediaID := groups[groupID][0].MediaID
		if prior, exists := targeted[mediaID]; exists {
			return nil, nil, fmt.Errorf("media_id %q appears in groups %q and %q; one group per item",
				mediaID, prior, groupID)
		}
		targeted[mediaID] = groupID
	}
	sort.Strings(order)
	return groups, order, nil
}

// prepare fully validates one group and stages its new sidecar bytes without
// touching the library. Any error means the group is rejected.
func (e *Engine) prepare(groupID string, bucket []catalog.Correction) (*stagedGroup, error) {
	mediaID := bucket[0].MediaID
	entry, ok := e.lib.ByID[mediaID]
	result := &GroupResult{GroupID: groupID, MediaID: mediaID}
	sg := &stagedGroup{result: result, entry: entry}
	if !ok {
		return sg, fmt.Errorf("unknown media_id %q", mediaID)
	}
	result.SidecarPath = entry.Record.SidecarPath()

	// Same-name multi-language disambiguation evidence.
	for _, peer := range e.lib.SameNameVariants(entry) {
		if peer.Record.MediaID != mediaID {
			result.SameNamePeers = append(result.SameNamePeers, peer.Record.MediaID)
		}
	}

	// Media hash must be present and match the bytes before any label work.
	if !entry.MediaExists {
		return sg, fmt.Errorf("media file missing for %q", mediaID)
	}
	if !entry.HashOK {
		return sg, fmt.Errorf("media hash mismatch for %q; refusing to repair", mediaID)
	}
	result.HashBefore = entry.Record.MediaHash

	// Read-only sidecars are rejected up front so a group never partially
	// lands via a rename that bypasses the permission bit.
	if !entry.Writable && !e.dryRun {
		return sg, fmt.Errorf("sidecar %q is read-only", result.SidecarPath)
	}

	edited := map[string]catalog.Label{}
	for _, corr := range bucket {
		verdict := authority.Evaluate(corr, entry.Record, e.opts)
		fr := FieldResult{
			Field:     corr.Field,
			Before:    entry.Record.Labels[corr.Field].Value,
			After:     corr.Value,
			Canonical: verdict.Canonical,
			Decision:  verdict.Decision,
			Reason:    verdict.Reason,
			Authority: string(verdict.WinningSource),
		}
		result.Fields = append(result.Fields, fr)
		if verdict.Decision != authority.DecisionAccepted {
			return sg, fmt.Errorf("field %q not authorized: %s (%s)",
				corr.Field, verdict.Reason, verdict.Detail)
		}

		label := entry.Record.Labels[corr.Field]
		label.Value = verdict.Canonical
		label.Source = verdict.WinningSource
		label.Evidence = append(label.Evidence, catalog.Evidence{
			Source: catalog.SourceSidecar,
			Field:  corr.Field,
			Raw:    corr.Value,
			At:     time.Now().UTC(),
		})
		edited[corr.Field] = label
	}

	newData, err := catalog.Encode(entry.Record, entry.Protected, edited)
	if err != nil {
		return sg, err
	}

	// Round-trip guard: re-decode the staged bytes and ensure every protected
	// top-level field is preserved and the media hash is untouched.
	if err := verifyStable(entry, newData); err != nil {
		return sg, err
	}

	sg.newData = newData
	sg.mode = entry.OriginalMode
	sg.backup = mustReadFile(result.SidecarPath)
	result.HashAfter = result.HashBefore
	result.HashStable = true
	return sg, nil
}

// verifyStable decodes the staged sidecar and proves that no protected field
// (including media_hash) changed relative to the loaded record.
func verifyStable(entry *catalog.Entry, newData []byte) error {
	record, protected, err := catalog.Decode(newData)
	if err != nil {
		return fmt.Errorf("staged sidecar is invalid: %w", err)
	}
	if record.MediaHash != entry.Record.MediaHash {
		return fmt.Errorf("media_hash would change")
	}
	if record.MediaPath != entry.Record.MediaPath || record.Name != entry.Record.Name ||
		record.MediaID != entry.Record.MediaID {
		return fmt.Errorf("protected identity field would change")
	}
	for key, oldBlob := range entry.Protected {
		newBlob, ok := protected[key]
		if !ok {
			return fmt.Errorf("protected field %q would be dropped", key)
		}
		if !jsonEqual(oldBlob, newBlob) {
			return fmt.Errorf("protected field %q would change", key)
		}
	}
	return nil
}

// commit writes staged bytes atomically and re-hashes the media file to prove
// the media digest is unchanged.
func (e *Engine) commit(sg *stagedGroup) error {
	if err := catalog.WriteSidecarAtomic(sg.result.SidecarPath, sg.newData, sg.mode); err != nil {
		return err
	}
	mediaPath := sg.entry.Record.MediaPath
	if !isAbs(mediaPath) {
		mediaPath = joinPath(e.lib.Root, mediaPath)
	}
	actual, err := catalog.ReadMediaHash(mediaPath)
	if err != nil {
		return err
	}
	if actual != sg.result.HashBefore {
		return fmt.Errorf("media hash changed after write")
	}
	sg.result.HashAfter = actual
	sg.result.HashStable = actual == sg.result.HashBefore

	// Keep the in-memory library aligned with the bytes now on disk.
	reloaded, protected, err := catalog.LoadSidecar(sg.result.SidecarPath)
	if err != nil {
		return err
	}
	sg.entry.Record = reloaded
	sg.entry.Protected = protected
	return nil
}

// undo restores the original sidecar bytes after a failed commit or a batch
// abort. Media files are never written, so only the sidecar needs restoring.
func (e *Engine) undo(sg *stagedGroup) {
	if sg == nil || sg.backup == nil {
		return
	}
	_ = catalog.WriteSidecarAtomic(sg.result.SidecarPath, sg.backup, sg.mode)
	sg.result.HashAfter = sg.result.HashBefore
	sg.result.HashStable = true
}

func mustReadFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return fmt.Sprint(x) == fmt.Sprint(y)
}
