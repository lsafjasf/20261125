package mkv

import (
	"testing"

	"ffv1compare/internal/testutil"
)

func TestParseMachineA(t *testing.T) {
	doc, err := ParseFile(testutil.DataPath("machine-a.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.DocType != "matroska" {
		t.Fatalf("DocType=%q", doc.DocType)
	}
	if doc.TimestampScaleNS != 1_000_000 {
		t.Fatalf("TimestampScaleNS=%d", doc.TimestampScaleNS)
	}
	if len(doc.Tracks) != 1 {
		t.Fatalf("tracks=%d", len(doc.Tracks))
	}
	tr := doc.Tracks[0]
	if tr.CodecID != "V_MS/VFW/FOURCC" || tr.Width != 64 || tr.Height != 48 {
		t.Fatalf("track=%+v", tr)
	}
	if len(tr.CodecPrivate) != 82 {
		t.Fatalf("CodecPrivate len=%d", len(tr.CodecPrivate))
	}
	if len(doc.Frames) != 10 {
		t.Fatalf("frames=%d", len(doc.Frames))
	}
	if !doc.Frames[0].Keyframe || doc.Frames[1].Keyframe || !doc.Frames[5].Keyframe {
		t.Fatalf("unexpected keyframe flags: %v %v %v",
			doc.Frames[0].Keyframe, doc.Frames[1].Keyframe, doc.Frames[5].Keyframe)
	}
	for i, want := range []int64{0, 100_000_000, 200_000_000, 900_000_000} {
		idx := i
		if i == 3 {
			idx = 9
		}
		if got := doc.Frames[idx].PTSNS; got != want {
			t.Fatalf("frame %d PTS=%d want %d", idx, got, want)
		}
	}
	if len(doc.ContainerCRCs) == 0 {
		t.Fatal("expected container CRC elements")
	}
	for _, c := range doc.ContainerCRCs {
		if !c.OK {
			t.Fatalf("container CRC mismatch: %+v", c)
		}
	}
}
