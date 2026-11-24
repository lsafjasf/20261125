package otioflat

import "encoding/json"

// MarshalFlat serializes the reversible flat representation.
func MarshalFlat(flat *FlatDocument) ([]byte, error) {
	return json.MarshalIndent(flat, "", "  ")
}

// UnmarshalFlat parses a reversible flat representation.
func UnmarshalFlat(data []byte) (*FlatDocument, error) {
	var flat FlatDocument
	if err := json.Unmarshal(data, &flat); err != nil {
		return nil, err
	}
	return &flat, nil
}

// FlattenJSON flattens and encodes the result as JSON.
func FlattenJSON(timeline *Timeline) ([]byte, error) {
	flat, err := Flatten(timeline)
	if err != nil {
		return nil, err
	}
	return MarshalFlat(flat)
}

// UnflattenJSON parses flat JSON and rebuilds the nested timeline.
func UnflattenJSON(data []byte) (*Timeline, error) {
	flat, err := UnmarshalFlat(data)
	if err != nil {
		return nil, err
	}
	return Unflatten(flat)
}
