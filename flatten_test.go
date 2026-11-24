package otioflat

import (
	"errors"
	"reflect"
	"testing"
)

func TestFlattenRejectsPointerCycle(t *testing.T) {
	root := &Node{OTIO_SCHEMA: SchemaStack, ID: "root", Children: []*Node{}}
	outer := &Node{OTIO_SCHEMA: SchemaSequence, ID: "outer", Children: []*Node{}}
	clip := testClip("clip", "media://a.mov", 2)

	root.Children = []*Node{outer}
	outer.Children = []*Node{clip, root}

	_, err := Flatten(&Timeline{OTIO_SCHEMA: SchemaTimeline, Tracks: root})
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("expected ErrCycle, got %v", err)
	}
	var cycleErr *CycleError
	if !errors.As(err, &cycleErr) {
		t.Fatalf("expected *CycleError, got %T", err)
	}
	if len(cycleErr.Path) < 3 {
		t.Fatalf("expected cycle path, got %#v", cycleErr.Path)
	}
}

func TestFlattenAcceptsRepeatedDAGInstance(t *testing.T) {
	clip := testClip("clip", "media://repeat.mov", 1)
	sequence := &Node{
		OTIO_SCHEMA: SchemaSequence,
		ID:          "shared-sequence",
		Children:    []*Node{clip},
	}
	root := &Node{
		OTIO_SCHEMA: SchemaStack,
		ID:          "root",
		Children:    []*Node{sequence, sequence},
	}
	timeline := &Timeline{OTIO_SCHEMA: SchemaTimeline, Tracks: root}

	flat, err := Flatten(timeline)
	if err != nil {
		t.Fatalf("repeated acyclic instance should be allowed: %v", err)
	}
	if got, want := len(flat.Items), 2; got != want {
		t.Fatalf("expected %d expanded occurrences, got %d", want, got)
	}
	if flat.Items[0].Path[len(flat.Items[0].Path)-1].Index != 0 {
		t.Fatalf("first occurrence has wrong path index: %+v", flat.Items[0].Path)
	}
	if flat.Items[1].Path[len(flat.Items[1].Path)-1].Index != 1 {
		t.Fatalf("second occurrence has wrong path index: %+v", flat.Items[1].Path)
	}

	rebuilt, err := Unflatten(flat)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(timeline, rebuilt) {
		t.Fatalf("repeated DAG instance did not round trip\nwant:\n%s\ngot:\n%s",
			mustPrettyJSON(t, timeline), mustPrettyJSON(t, rebuilt))
	}
}

func TestLocalTimingAndMediaReferenceProvenance(t *testing.T) {
	media := &MediaReference{
		OTIO_SCHEMA: SchemaExternalReference,
		ID:          "media-1",
		TargetURL:   "media://interview.mov",
		AvailableRange: &TimeRange{
			StartTime: RationalTime{Value: 0, Rate: 24},
			Duration:  RationalTime{Value: 240, Rate: 24},
		},
	}
	innerClip := &Node{
		OTIO_SCHEMA:    SchemaClip,
		ID:             "inner-clip",
		SourceRange:    &TimeRange{StartTime: RationalTime{Value: 24, Rate: 24}, Duration: RationalTime{Value: 12, Rate: 24}},
		MediaReference: media,
	}
	inner := &Node{
		OTIO_SCHEMA: SchemaSequence,
		ID:          "inner",
		// The instance trims the inner sequence to 3 seconds in parent time.
		SourceRange: &TimeRange{StartTime: RationalTime{Value: 1, Rate: 1}, Duration: RationalTime{Value: 3, Rate: 1}},
		Children:    []*Node{innerClip},
	}
	first := testClip("first", "media://first.mov", 2)
	last := testClip("last", "media://last.mov", 1)
	empty := &Node{OTIO_SCHEMA: SchemaSequence, ID: "empty-parent", Children: []*Node{}}
	root := &Node{
		OTIO_SCHEMA: SchemaSequence,
		ID:          "root",
		Children:    []*Node{first, inner, last, empty},
	}
	timeline := &Timeline{OTIO_SCHEMA: SchemaTimeline, Tracks: root}

	flat, err := Flatten(timeline)
	if err != nil {
		t.Fatal(err)
	}

	byID := make(map[string]*FlatItem, len(flat.Items))
	for _, item := range flat.Items {
		byID[item.ID] = item
	}

	firstItem := byID["first"]
	if firstItem == nil || firstItem.Index != 0 || firstItem.LocalStart.Value != 0 {
		t.Fatalf("bad first provenance: %#v", firstItem)
	}

	innerClipItem := byID["inner-clip"]
	if innerClipItem == nil {
		t.Fatal("missing inner clip")
	}
	if got, want := innerClipItem.Path[1].ID, "inner"; got != want {
		t.Fatalf("parent sequence = %q, want %q", got, want)
	}
	if got := innerClipItem.Path[1].Index; got != 1 {
		t.Fatalf("inner instance index = %d, want 1", got)
	}
	if got := innerClipItem.Path[1].LocalStart; got != (RationalTime{Value: 2, Rate: 1}) {
		t.Fatalf("inner parent local start = %d/%d, want 2/1", got.Value, got.Rate)
	}
	if innerClipItem.SourceRange.StartTime.Value != 24 {
		t.Fatalf("clip local source start not preserved: %#v", innerClipItem.SourceRange)
	}
	if innerClipItem.MediaReference == nil || innerClipItem.MediaReference.TargetURL != "media://interview.mov" {
		t.Fatalf("media reference not preserved: %#v", innerClipItem.MediaReference)
	}

	lastItem := byID["last"]
	if lastItem == nil {
		t.Fatal("missing last clip")
	}
	wantStart, err := addTime(RationalTime{Value: 2, Rate: 1}, RationalTime{Value: 3, Rate: 1})
	if err != nil {
		t.Fatal(err)
	}
	if lastItem.Index != 2 || lastItem.LocalStart != wantStart {
		t.Fatalf("last placement = index %d start %v, want index 2 start %v", lastItem.Index, lastItem.LocalStart, wantStart)
	}

	var marker *FlatItem
	for _, item := range flat.Items {
		if item.Marker == ContainerMarker && item.ID == "empty-parent" {
			marker = item
		}
	}
	if marker == nil {
		t.Fatal("empty parent was not marked")
	}

	rebuilt, err := Unflatten(flat)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(timeline, rebuilt) {
		t.Fatalf("timing/provenance round trip mismatch\nwant:\n%s\ngot:\n%s",
			mustPrettyJSON(t, timeline), mustPrettyJSON(t, rebuilt))
	}
}

func testClip(id, url string, durationSeconds int64) *Node {
	return &Node{
		OTIO_SCHEMA: SchemaClip,
		ID:          id,
		Name:        id,
		SourceRange: &TimeRange{
			StartTime: RationalTime{Value: 0, Rate: 1},
			Duration:  RationalTime{Value: durationSeconds, Rate: 1},
		},
		MediaReference: &MediaReference{
			OTIO_SCHEMA: SchemaExternalReference,
			TargetURL:   url,
		},
	}
}
