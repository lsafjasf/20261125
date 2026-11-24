// Package report renders repair results as machine-readable JSON and a
// human-readable console summary.
package report

import (
	"encoding/json"
	"fmt"
	"io"

	"adlabel/internal/repair"
)

// WriteJSON emits the full report with stable indentation.
func WriteJSON(w io.Writer, r *repair.Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(r); err != nil {
		return err
	}
	return nil
}

// WriteText emits a concise operator summary.
func WriteText(w io.Writer, r *repair.Report) {
	mode := "apply"
	if r.DryRun {
		mode = "dry-run"
	}
	fmt.Fprintf(w, "adlabel repair (%s) root=%s\n", mode, r.LibraryRoot)
	if r.AllOrNothing {
		fmt.Fprintln(w, "policy: all-or-nothing transaction")
	} else {
		fmt.Fprintln(w, "policy: per-group transaction")
	}
	fmt.Fprintf(w, "groups=%d applied=%d rolled_back=%d skipped=%d changed_fields=%d\n",
		r.Summary.TotalGroups, r.Summary.Applied, r.Summary.RolledBack,
		r.Summary.Skipped, r.Summary.ChangedFields)
	if r.Aborted {
		fmt.Fprintln(w, "BATCH ABORTED and rolled back to the pre-run state")
	}
	for _, g := range r.Groups {
		fmt.Fprintf(w, "- [%s] group=%s media=%s sidecar=%s\n",
			g.Status, g.GroupID, g.MediaID, g.SidecarPath)
		if g.HashAfter != "" && g.HashBefore != g.HashAfter {
			fmt.Fprintf(w, "    MEDIA HASH CHANGED %s -> %s\n", g.HashBefore, g.HashAfter)
		}
		if len(g.SameNamePeers) > 0 {
			fmt.Fprintf(w, "    same-name peers disambiguated by id: %v\n", g.SameNamePeers)
		}
		for _, f := range g.Fields {
			fmt.Fprintf(w, "    %s: %q -> %q (%s, authority=%s)\n",
				f.Field, f.Before, f.After, f.Decision, f.Authority)
		}
		if g.FailureReason != "" {
			fmt.Fprintf(w, "    reason: %s\n", g.FailureReason)
		}
	}
}

// ExitCode maps a report to a process exit code. Any rollback or an aborted
// batch is a failure so partial repairs can never look like success.
func ExitCode(r *repair.Report) int {
	if r.Aborted || r.Summary.RolledBack > 0 {
		return 1
	}
	return 0
}
