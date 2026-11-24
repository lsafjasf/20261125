package otioflat

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

func TestRandomRoundTripTwoLevelsAndRepeatedInstances(t *testing.T) {
	for seed := int64(0); seed < 250; seed++ {
		rng := rand.New(rand.NewSource(seed))
		original := randomTimeline(rng, seed)

		flat, err := Flatten(original)
		if err != nil {
			t.Fatalf("seed %d: Flatten returned error: %v", seed, err)
		}
		if len(flat.Items) == 0 {
			t.Fatalf("seed %d: expected flattened items", seed)
		}
		foundTwoLevels := false
		for _, item := range flat.Items {
			if len(item.Path) >= 3 {
				foundTwoLevels = true
				break
			}
		}
		if !foundTwoLevels {
			t.Fatalf("seed %d: expected an item below outer and inner sequences", seed)
		}

		rebuilt, err := Unflatten(flat)
		if err != nil {
			t.Fatalf("seed %d: Unflatten returned error: %v", seed, err)
		}
		if !reflect.DeepEqual(original, rebuilt) {
			t.Fatalf("seed %d: direct round trip mismatch\nwant:\n%s\ngot:\n%s",
				seed, mustPrettyJSON(t, original), mustPrettyJSON(t, rebuilt))
		}

		data, err := MarshalFlat(flat)
		if err != nil {
			t.Fatalf("seed %d: marshal flat: %v", seed, err)
		}
		fromJSON, err := UnflattenJSON(data)
		if err != nil {
			t.Fatalf("seed %d: JSON round trip: %v\nflat JSON:\n%s", seed, err, data)
		}
		if !reflect.DeepEqual(original, fromJSON) {
			t.Fatalf("seed %d: JSON round trip mismatch\nwant:\n%s\ngot:\n%s",
				seed, mustPrettyJSON(t, original), mustPrettyJSON(t, fromJSON))
		}

		otioTimeline, err := FlattenOTIO(original)
		if err != nil {
			t.Fatalf("seed %d: FlattenOTIO: %v", seed, err)
		}
		fromOTIOMemory, err := UnflattenOTIO(otioTimeline)
		if err != nil {
			t.Fatalf("seed %d: UnflattenOTIO memory: %v", seed, err)
		}
		if !reflect.DeepEqual(original, fromOTIOMemory) {
			t.Fatalf("seed %d: in-memory OTIO round trip mismatch\nwant:\n%s\ngot:\n%s",
				seed, mustPrettyJSON(t, original), mustPrettyJSON(t, fromOTIOMemory))
		}

		otioData, err := FlattenOTIOJSON(original)
		if err != nil {
			t.Fatalf("seed %d: FlattenOTIOJSON: %v", seed, err)
		}
		fromOTIO, err := UnflattenOTIOJSON(otioData)
		if err != nil {
			t.Fatalf("seed %d: OTIO metadata round trip: %v\nOTIO JSON:\n%s", seed, err, otioData)
		}
		if !reflect.DeepEqual(original, fromOTIO) {
			t.Fatalf("seed %d: OTIO metadata round trip mismatch\nwant:\n%s\ngot:\n%s",
				seed, mustPrettyJSON(t, original), mustPrettyJSON(t, fromOTIO))
		}
	}
}

func randomTimeline(rng *rand.Rand, seed int64) *Timeline {
	rate := []int64{24, 25, 30}[rng.Intn(3)]
	media := make([]*MediaReference, 5)
	for i := range media {
		media[i] = &MediaReference{
			OTIO_SCHEMA: SchemaExternalReference,
			ID:          fmt.Sprintf("media-%d-%d", seed, i),
			Name:        fmt.Sprintf("Camera %d", i+1),
			TargetURL:   fmt.Sprintf("media://project-%d/cam-%02d.mov", seed, i),
			AvailableRange: &TimeRange{
				StartTime: RationalTime{Value: 0, Rate: rate},
				Duration:  RationalTime{Value: 120 + rng.Int63n(120), Rate: rate},
			},
			Metadata: Metadata{"kind": "external_media", "pool": "random"},
		}
	}

	root := &Node{
		OTIO_SCHEMA: SchemaStack,
		ID:          fmt.Sprintf("tracks-%d", seed),
		Name:        "tracks",
		Children:    []*Node{},
		Metadata:    Metadata{"role": "timeline_tracks"},
	}

	outerCount := 1 + rng.Intn(3)
	for outerIndex := 0; outerIndex < outerCount; outerIndex++ {
		outer := &Node{
			OTIO_SCHEMA: []string{SchemaTrack, SchemaSequence}[rng.Intn(2)],
			ID:          fmt.Sprintf("outer-%d-%d", seed, outerIndex),
			Name:        fmt.Sprintf("Outer Sequence %d", outerIndex),
			Children:    []*Node{},
			Metadata:    Metadata{"role": "outer", "seed": fmt.Sprint(seed)},
		}
		if rng.Intn(2) == 0 {
			outer.SourceRange = randomRange(rng, rate)
		}

		innerA := randomInnerSequence(rng, seed, outerIndex, 0, rate, media)
		outer.Children = append(outer.Children, innerA)

		innerCount := 1 + rng.Intn(2)
		for innerIndex := 1; innerIndex <= innerCount; innerIndex++ {
			outer.Children = append(outer.Children,
				randomInnerSequence(rng, seed, outerIndex, innerIndex, rate, media))
		}

		// Always repeat the exact same instance pointer in a later sibling
		// position. The occurrence path/index, not the node ID, distinguishes it.
		outer.Children = append(outer.Children, innerA)

		if rng.Intn(3) == 0 {
			outer.Children = append(outer.Children, randomGap(rng, seed, outerIndex, 99, rate))
		}
		root.Children = append(root.Children, outer)
	}

	return &Timeline{
		OTIO_SCHEMA: SchemaTimeline,
		Name:        fmt.Sprintf("random-roundtrip-%d", seed),
		Tracks:      root,
		Metadata:    Metadata{"generator": "otioflat.random", "seed": fmt.Sprint(seed)},
	}
}

func randomInnerSequence(rng *rand.Rand, seed int64, outerIndex, innerIndex int, rate int64, media []*MediaReference) *Node {
	inner := &Node{
		OTIO_SCHEMA: []string{SchemaSequence, SchemaTrack, SchemaStack}[rng.Intn(3)],
		ID:          fmt.Sprintf("inner-%d-%d-%d", seed, outerIndex, innerIndex),
		Name:        fmt.Sprintf("Inner %d-%d", outerIndex, innerIndex),
		Children:    []*Node{},
		Metadata: Metadata{
			"role":       "inner",
			"definition": fmt.Sprintf("inner-template-%d", innerIndex%2),
			"repeated":   "true",
		},
	}
	if inner.OTIO_SCHEMA == SchemaStack {
		inner.NodeLayout = LayoutParallel
	}
	if rng.Intn(2) == 0 {
		inner.SourceRange = randomRange(rng, rate)
	}

	clipCount := 1 + rng.Intn(5)
	for clipIndex := 0; clipIndex < clipCount; clipIndex++ {
		mediaReference := media[rng.Intn(len(media))]
		if clipIndex == 0 {
			mediaReference = media[0]
		}
		clip := &Node{
			OTIO_SCHEMA:    SchemaClip,
			ID:             fmt.Sprintf("clip-%d-%d-%d-%d", seed, outerIndex, innerIndex, clipIndex),
			Name:           fmt.Sprintf("Shot %d", clipIndex),
			SourceRange:    randomRange(rng, rate),
			MediaReference: mediaReference,
			Metadata:       Metadata{"role": "clip", "take": fmt.Sprint(1 + rng.Intn(4))},
		}
		inner.Children = append(inner.Children, clip)
		if rng.Intn(5) == 0 {
			inner.Children = append(inner.Children, randomGap(rng, seed, outerIndex, clipIndex, rate))
		}
	}
	return inner
}

func randomRange(rng *rand.Rand, rate int64) *TimeRange {
	return &TimeRange{
		StartTime: RationalTime{Value: rng.Int63n(5), Rate: rate},
		Duration:  RationalTime{Value: 1 + rng.Int63n(12), Rate: rate},
	}
}

func randomGap(rng *rand.Rand, seed int64, outerIndex, suffix int, rate int64) *Node {
	gap := &Node{
		OTIO_SCHEMA: SchemaGap,
		ID:          fmt.Sprintf("gap-%d-%d-%d", seed, outerIndex, suffix),
		Name:        "Gap",
	}
	if rng.Intn(2) == 0 {
		gap.SourceRange = randomRange(rng, rate)
	} else {
		gap.Duration = &RationalTime{Value: 1 + rng.Int63n(4), Rate: rate}
	}
	return gap
}

func mustPrettyJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal debug JSON: %v", err)
	}
	return string(data)
}
