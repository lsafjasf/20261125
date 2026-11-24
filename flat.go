package otioflat

import (
	"errors"
	"strconv"
)

// FlatSchema identifies the reversible flat JSON representation.
const FlatSchema = "ReversibleFlatTimeline/v1"

// ContainerMarker is stored as a zero-duration flat item so that empty parent
// compositions survive flatten/unflatten.
const ContainerMarker = "empty-composition"

// ErrCycle is returned when an in-memory composition graph cannot be expanded
// because traversal has returned to a node already on the active path.
var ErrCycle = errors.New("otioflat: cyclic composition cannot be flattened")

// CycleError describes the active traversal path that formed a cycle.
type CycleError struct {
	Path []string
}

func (e *CycleError) Error() string {
	return ErrCycle.Error() + ": " + joinPath(e.Path)
}

func (e *CycleError) Unwrap() error {
	return ErrCycle
}

// Stamp contains all reversible scalar fields of an OTIO subset node. Index
// and LocalStart are placement facts within the direct parent.
type Stamp struct {
	OTIO_SCHEMA     string          `json:"OTIO_SCHEMA,omitempty"`
	ID              string          `json:"id,omitempty"`
	Name            string          `json:"name,omitempty"`
	Layout          Layout          `json:"layout,omitempty"`
	Index           int             `json:"index"`
	LocalStart      RationalTime    `json:"local_start"`
	ChildrenPresent bool            `json:"children_present,omitempty"`
	SourceRange     *TimeRange      `json:"source_range,omitempty"`
	MediaReference  *MediaReference `json:"media_reference,omitempty"`
	Duration        *RationalTime   `json:"duration,omitempty"`
	Metadata        Metadata        `json:"metadata,omitempty"`
}

// FlatItem is a clip/gap leaf plus all provenance needed to put it back in its
// original parent sequence. Marker is non-empty only for bookkeeping items.
type FlatItem struct {
	Stamp
	GlobalStart RationalTime `json:"global_start"`
	Duration    RationalTime `json:"resolved_duration"`
	Path        []Stamp      `json:"path"`
	Marker      string       `json:"marker,omitempty"`
}

// FlatDocument is a list of leaves with occurrence-specific provenance.
type FlatDocument struct {
	OTIO_SCHEMA     string        `json:"OTIO_SCHEMA"`
	Version         int           `json:"version"`
	SourceSchema    string        `json:"source_OTIO_SCHEMA,omitempty"`
	Name            string        `json:"name,omitempty"`
	GlobalStartTime *RationalTime `json:"global_start_time,omitempty"`
	Root            *Stamp        `json:"root,omitempty"`
	Items           []*FlatItem   `json:"items"`
	Metadata        Metadata      `json:"metadata,omitempty"`
}

func joinPath(parts []string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += " -> "
		}
		out += part
	}
	return out
}

func nodeLabel(n *Node, index int) string {
	switch {
	case n == nil:
		return "<nil>"
	case n.ID != "":
		return n.ID
	case n.Name != "":
		return n.Name
	case n.OTIO_SCHEMA != "":
		return n.OTIO_SCHEMA + "#" + strconv.Itoa(index)
	default:
		return "node#" + strconv.Itoa(index)
	}
}
