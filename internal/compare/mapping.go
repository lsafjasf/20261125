// Package compare builds the cross-format event mapping between the
// canonical edit list and re-opened export files, and localizes timing
// discrepancies to a specific format and layer.
package compare

import (
	"cutcheck/internal/rational"
	"cutcheck/internal/timeline"
	"fmt"
	"sort"
)

// Reopened is a format-agnostic event re-opened from an exported file.
type Reopened struct {
	ID        string
	Clip      string
	Layer     string // "video" or "audio"
	Track     int
	RecordIn  rational.Rat // seconds
	RecordOut rational.Rat
	SourceIn  rational.Rat
	SourceOut rational.Rat
}

// FormatFile is one re-opened export under test.
type FormatFile struct {
	Format string // e.g. "cmx", "fcpxml"
	Events []Reopened
}

// NonEquivalent declares an item that a format cannot represent; it is
// excluded from the equivalence proof but listed in the report.
type NonEquivalent struct {
	EventID string // empty means a format-wide note
	Format  string
	Reason  string
}

// EventDelta is the signed timing error of one event in one format:
// re-opened value minus canonical value.
type EventDelta struct {
	EventID   string
	Layer     string
	Format    string
	RecordIn  rational.Rat // seconds
	RecordOut rational.Rat
}

// MappingRow links one canonical event to its re-opened counterparts.
type MappingRow struct {
	EventID string
	Layer   string
	Found   map[string]bool // format -> present
}

// canonicalEvents flattens the edit list into second-based tuples.
func canonicalEvents(el *timeline.EditList) []Reopened {
	var out []Reopened
	for _, ev := range el.Video {
		out = append(out, Reopened{
			ID:        ev.ID,
			Clip:      ev.Clip,
			Layer:     "video",
			Track:     ev.Track,
			RecordIn:  el.FrameSeconds(ev.RecordIn),
			RecordOut: el.FrameSeconds(ev.RecordOut),
			SourceIn:  el.FrameSeconds(ev.SourceIn),
			SourceOut: el.FrameSeconds(ev.SourceOut),
		})
	}
	for _, ev := range el.Audio {
		out = append(out, Reopened{
			ID:        ev.ID,
			Clip:      ev.Clip,
			Layer:     "audio",
			Track:     ev.Track,
			RecordIn:  el.SampleSeconds(ev.RecordIn),
			RecordOut: el.SampleSeconds(ev.RecordOut),
			SourceIn:  el.SampleSeconds(ev.SourceIn),
			SourceOut: el.SampleSeconds(ev.SourceOut),
		})
	}
	return out
}

// Map builds the cross-format event mapping table keyed by event ID.
func Map(el *timeline.EditList, files []FormatFile) []MappingRow {
	canon := canonicalEvents(el)
	rows := make([]MappingRow, 0, len(canon))
	for _, c := range canon {
		row := MappingRow{EventID: c.ID, Layer: c.Layer, Found: map[string]bool{}}
		for _, f := range files {
			for _, ev := range f.Events {
				if ev.ID == c.ID {
					row.Found[f.Format] = true
				}
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].EventID < rows[j].EventID })
	return rows
}

// Deltas computes per-event timing deltas for every format, skipping
// declared non-equivalent items.
func Deltas(el *timeline.EditList, files []FormatFile, declared []NonEquivalent) ([]EventDelta, []string) {
	skip := map[string]bool{}
	for _, d := range declared {
		if d.EventID != "" {
			skip[d.Format+"/"+d.EventID] = true
		}
	}
	var deltas []EventDelta
	var missing []string
	for _, c := range canonicalEvents(el) {
		for _, f := range files {
			if skip[f.Format+"/"+c.ID] {
				continue
			}
			var found *Reopened
			for i := range f.Events {
				if f.Events[i].ID == c.ID {
					found = &f.Events[i]
					break
				}
			}
			if found == nil {
				missing = append(missing, fmt.Sprintf("%s: event %s not found", f.Format, c.ID))
				continue
			}
			deltas = append(deltas, EventDelta{
				EventID:   c.ID,
				Layer:     c.Layer,
				Format:    f.Format,
				RecordIn:  found.RecordIn.Sub(c.RecordIn),
				RecordOut: found.RecordOut.Sub(c.RecordOut),
			})
		}
	}
	return deltas, missing
}
