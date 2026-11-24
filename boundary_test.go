package otioflat

import (
	"reflect"
	"testing"
)

func TestNilAndEmptyChildrenPresenceRoundTrip(t *testing.T) {
	original := &Timeline{
		OTIO_SCHEMA: SchemaTimeline,
		Tracks: &Node{
			OTIO_SCHEMA: SchemaStack,
			ID:          "root-with-nil-children",
		},
	}

	flat, err := Flatten(original)
	if err != nil {
		t.Fatal(err)
	}
	if flat.Root == nil || flat.Root.ChildrenPresent {
		t.Fatalf("root children_present should be false: %#v", flat.Root)
	}
	rebuilt, err := Unflatten(flat)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, rebuilt) {
		t.Fatalf("nil children mismatch\nwant: %s\ngot: %s",
			mustPrettyJSON(t, original), mustPrettyJSON(t, rebuilt))
	}

	original.Tracks.Children = []*Node{}
	flat, err = Flatten(original)
	if err != nil {
		t.Fatal(err)
	}
	if !flat.Root.ChildrenPresent {
		t.Fatal("empty non-nil children should set children_present")
	}
	rebuilt, err = Unflatten(flat)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, rebuilt) {
		t.Fatalf("empty children mismatch\nwant: %s\ngot: %s",
			mustPrettyJSON(t, original), mustPrettyJSON(t, rebuilt))
	}
}

func TestVersionedSchemaLayout(t *testing.T) {
	stack := &Node{OTIO_SCHEMA: "Stack.9"}
	if got := stack.Layout(); got != LayoutParallel {
		t.Fatalf("Stack.9 layout = %q, want %q", got, LayoutParallel)
	}
	track := &Node{OTIO_SCHEMA: "Track.1"}
	if got := track.Layout(); got != LayoutSequential {
		t.Fatalf("Track.1 layout = %q, want %q", got, LayoutSequential)
	}
	if got := BaseSchema("Clip.2"); got != SchemaClip {
		t.Fatalf("BaseSchema(Clip.2) = %q", got)
	}
}
