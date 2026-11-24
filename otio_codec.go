package otioflat

import (
	"encoding/json"
	"fmt"
)

const flatTrackID = "otioflat-flattened-track"

// otioHeader is stored at Timeline.metadata[MetadataNamespace]. It preserves
// document-level fields without requiring an OTIO reader to understand them.
type otioHeader struct {
	OTIO_SCHEMA     string        `json:"OTIO_SCHEMA"`
	Version         int           `json:"version"`
	SourceSchema    string        `json:"source_OTIO_SCHEMA,omitempty"`
	Name            string        `json:"name,omitempty"`
	GlobalStartTime *RationalTime `json:"global_start_time,omitempty"`
	Root            *Stamp        `json:"root,omitempty"`
	Metadata        Metadata      `json:"metadata,omitempty"`
}

// FlattenOTIO flattens into an OTIO-like Timeline containing one transport
// track. Provenance is embedded in each node's metadata namespace.
func FlattenOTIO(timeline *Timeline) (*Timeline, error) {
	flat, err := Flatten(timeline)
	if err != nil {
		return nil, err
	}

	children := make([]*Node, 0, len(flat.Items))
	for _, item := range flat.Items {
		visible, err := nodeFromStamp(&item.Stamp)
		if err != nil {
			return nil, err
		}
		if visible.Metadata == nil {
			visible.Metadata = Metadata{}
		}
		visible.Metadata[MetadataNamespace] = item
		children = append(children, visible)
	}

	envelopeMetadata := cloneMetadata(flat.Metadata)
	if envelopeMetadata == nil {
		envelopeMetadata = Metadata{}
	}
	envelopeMetadata[MetadataNamespace] = otioHeader{
		OTIO_SCHEMA:     flat.OTIO_SCHEMA,
		Version:         flat.Version,
		SourceSchema:    flat.SourceSchema,
		Name:            flat.Name,
		GlobalStartTime: cloneTime(flat.GlobalStartTime),
		Root:            flat.Root,
		Metadata:        cloneMetadata(flat.Metadata),
	}

	return &Timeline{
		OTIO_SCHEMA: SchemaTimeline,
		Name:        flat.Name,
		Tracks: &Node{
			OTIO_SCHEMA: SchemaStack,
			ID:          "otioflat-transport-stack",
			Name:        "tracks",
			Children: []*Node{{
				OTIO_SCHEMA: SchemaTrack,
				ID:          flatTrackID,
				Name:        "Flattened OTIO",
				Children:    children,
			}},
		},
		Metadata: envelopeMetadata,
	}, nil
}

// UnflattenOTIO reads a flattened OTIO-like Timeline and rebuilds nesting.
func UnflattenOTIO(timeline *Timeline) (*Timeline, error) {
	if timeline == nil {
		return nil, fmt.Errorf("otioflat: cannot unflatten nil OTIO timeline")
	}
	if timeline.Metadata == nil {
		return nil, fmt.Errorf("otioflat: missing metadata namespace %q", MetadataNamespace)
	}
	rawHeader, ok := timeline.Metadata[MetadataNamespace]
	if !ok {
		return nil, fmt.Errorf("otioflat: missing flatten header in metadata[%q]", MetadataNamespace)
	}
	header, err := decodeMetadata[otioHeader](rawHeader)
	if err != nil {
		return nil, fmt.Errorf("otioflat: decode flatten header: %w", err)
	}
	if header.OTIO_SCHEMA != FlatSchema || header.Version != 1 {
		return nil, fmt.Errorf("otioflat: unsupported OTIO flatten header %q version %d", header.OTIO_SCHEMA, header.Version)
	}

	flat := &FlatDocument{
		OTIO_SCHEMA:     FlatSchema,
		Version:         1,
		SourceSchema:    header.SourceSchema,
		Name:            header.Name,
		GlobalStartTime: cloneTime(header.GlobalStartTime),
		Root:            header.Root,
		Metadata:        cloneMetadata(header.Metadata),
		Items:           []*FlatItem{},
	}

	var walk func(*Node) error
	walk = func(node *Node) error {
		if node == nil {
			return nil
		}
		if raw, ok := node.Metadata[MetadataNamespace]; ok {
			item, err := decodeMetadata[FlatItem](raw)
			if err != nil {
				return fmt.Errorf("decode item provenance for %q: %w", node.ID, err)
			}
			flat.Items = append(flat.Items, item)
			return nil
		}
		for _, child := range node.Children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(timeline.Tracks); err != nil {
		return nil, err
	}
	return Unflatten(flat)
}

// FlattenOTIOJSON returns a JSON-serialized OTIO transport timeline.
func FlattenOTIOJSON(timeline *Timeline) ([]byte, error) {
	otioTimeline, err := FlattenOTIO(timeline)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(otioTimeline, "", "  ")
}

// UnflattenOTIOJSON parses OTIO JSON and rebuilds the nested timeline.
func UnflattenOTIOJSON(data []byte) (*Timeline, error) {
	var timeline Timeline
	if err := json.Unmarshal(data, &timeline); err != nil {
		return nil, err
	}
	return UnflattenOTIO(&timeline)
}

func decodeMetadata[T any](raw any) (*T, error) {
	if value, ok := raw.(T); ok {
		return &value, nil
	}
	if value, ok := raw.(*T); ok {
		return value, nil
	}
	if text, ok := raw.(string); ok {
		var value T
		if err := json.Unmarshal([]byte(text), &value); err != nil {
			return nil, err
		}
		return &value, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return &value, nil
}
