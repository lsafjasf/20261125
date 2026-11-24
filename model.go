package otioflat

import "strings"

// Metadata is the OTIO metadata extension object.
type Metadata map[string]any

// Timeline is a small nested OTIO-like timeline. Tracks is normally a Stack
// whose children are Tracks/Sequences; this package also accepts any Node as
// the root for focused tests.
type Timeline struct {
	OTIO_SCHEMA     string        `json:"OTIO_SCHEMA,omitempty"`
	Name            string        `json:"name,omitempty"`
	GlobalStartTime *RationalTime `json:"global_start_time,omitempty"`
	Tracks          *Node         `json:"tracks,omitempty"`
	Metadata        Metadata      `json:"metadata,omitempty"`
}

// MediaReference is the supported external media-reference subset.
type MediaReference struct {
	OTIO_SCHEMA    string     `json:"OTIO_SCHEMA,omitempty"`
	ID             string     `json:"id,omitempty"`
	Name           string     `json:"name,omitempty"`
	TargetURL      string     `json:"target_url,omitempty"`
	AvailableRange *TimeRange `json:"available_range,omitempty"`
	Metadata       Metadata   `json:"metadata,omitempty"`
}

// Node is either a composition (non-nil Children), a clip (non-nil
// MediaReference), or a gap. The union is intentional: it mirrors OTIO's
// polymorphic serialized objects while keeping the subset easy to traverse.
type Node struct {
	OTIO_SCHEMA    string          `json:"OTIO_SCHEMA,omitempty"`
	ID             string          `json:"id,omitempty"`
	Name           string          `json:"name,omitempty"`
	NodeLayout     Layout          `json:"layout,omitempty"`
	SourceRange    *TimeRange      `json:"source_range,omitempty"`
	MediaReference *MediaReference `json:"media_reference,omitempty"`
	Duration       *RationalTime   `json:"duration,omitempty"`
	Children       []*Node         `json:"children,omitempty"`
	Metadata       Metadata        `json:"metadata,omitempty"`
}

// Layout describes how children occupy local time.
type Layout string

const (
	// LayoutSequential places children end-to-end, like Track/Sequence.
	LayoutSequential Layout = "sequential"
	// LayoutParallel overlaps children, like Stack.
	LayoutParallel Layout = "parallel"
)

const (
	SchemaTimeline          = "Timeline"
	SchemaStack             = "Stack"
	SchemaTrack             = "Track"
	SchemaSequence          = "Sequence"
	SchemaClip              = "Clip"
	SchemaGap               = "Gap"
	SchemaExternalReference = "ExternalReference"

	// MetadataNamespace stores reversible provenance in ordinary OTIO metadata.
	MetadataNamespace = "dev.userfeedback.otioflat/v1"
)

// IsComposition reports whether n is a composition. A non-nil child slice is
// decisive even without a schema; known OTIO composition schemas are also
// accepted when children are omitted from JSON.
func (n *Node) IsComposition() bool {
	if n == nil {
		return false
	}
	return n.Children != nil || IsCompositionSchema(n.OTIO_SCHEMA)
}

// IsCompositionSchema reports whether schema is a supported parent type.
func IsCompositionSchema(schema string) bool {
	switch BaseSchema(schema) {
	case SchemaStack, SchemaTrack, SchemaSequence:
		return true
	default:
		return false
	}
}

// Layout returns the explicit layout when present, otherwise infers it from
// the OTIO schema name. Unknown compositions are sequential.
func (n *Node) Layout() Layout {
	if n == nil {
		return LayoutSequential
	}
	switch n.NodeLayout {
	case LayoutParallel, LayoutSequential:
		return n.NodeLayout
	}
	switch BaseSchema(n.OTIO_SCHEMA) {
	case SchemaStack:
		return LayoutParallel
	default:
		return LayoutSequential
	}
}

// BaseSchema strips an OTIO schema version suffix such as ".1".
func BaseSchema(schema string) string {
	if index := strings.LastIndex(schema, "."); index > 0 {
		version := schema[index+1:]
		if version != "" {
			allDigits := true
			for _, r := range version {
				if r < '0' || r > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				return schema[:index]
			}
		}
	}
	return schema
}
