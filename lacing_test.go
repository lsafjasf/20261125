package matroska

import (
	"errors"
	"testing"
)

func blockPayload(flags byte, lacingHeader []byte, data []byte, relative int16) []byte {
	payload := []byte{0x81, byte(relative >> 8), byte(relative), flags}
	payload = append(payload, lacingHeader...)
	return append(payload, data...)
}

func TestFixedLacing(t *testing.T) {
	data := []byte("AAAABBBBCCCC")
	payload := blockPayload(0x04, []byte{2}, data, 7)
	opts := BlockOptions{
		PayloadOffset:        1000,
		ClusterTimecode:      2000,
		TimestampScale:       1_000_000,
		DefaultFrameDuration: 20_000_000,
	}
	block, err := ParseBlock(payload, opts)
	if err != nil {
		t.Fatal(err)
	}
	if block.Lacing != LacingFixed || block.FrameCount != 3 {
		t.Fatalf("lacing=%v frames=%d", block.Lacing, block.FrameCount)
	}
	wantStarts := []int64{1005, 1009, 1013}
	for i, frame := range block.Frames {
		if frame.Start != wantStarts[i] || frame.Size() != 4 {
			t.Errorf("frame %d range [%d,%d)", i, frame.Start, frame.End)
		}
		if want := int64(2007 + i*20); frame.Timecode != want {
			t.Errorf("frame %d timecode=%d want %d", i, frame.Timecode, want)
		}
		if want := int64(2_007_000_000 + i*20_000_000); frame.TimeNanos != want {
			t.Errorf("frame %d time_ns=%d want %d", i, frame.TimeNanos, want)
		}
	}
}

func TestNoLacing(t *testing.T) {
	payload := blockPayload(0x00, nil, []byte("single-frame"), 0)
	block, err := ParseBlock(payload, BlockOptions{PayloadOffset: 20})
	if err != nil {
		t.Fatal(err)
	}
	if block.Lacing != LacingNone || len(block.Frames) != 1 {
		t.Fatalf("lacing=%v frames=%d", block.Lacing, len(block.Frames))
	}
	if frame := block.Frames[0]; frame.Start != 24 || frame.End != 36 {
		t.Fatalf("frame range [%d,%d)", frame.Start, frame.End)
	}
}

func TestMaxVINTTrackNumber(t *testing.T) {
	payload := []byte{0xFF, 0, 0, 0, 'x'}
	block, err := ParseBlock(payload, BlockOptions{PayloadOffset: 20})
	if err != nil {
		t.Fatal(err)
	}
	if block.TrackNumber != 127 {
		t.Fatalf("track=%d want 127", block.TrackNumber)
	}
}

func TestXiphLacingWith255Chunks(t *testing.T) {
	data := make([]byte, 300+5+7)
	payload := blockPayload(0x02, []byte{2, 255, 45, 5}, data, 0)
	block, err := ParseBlock(payload, BlockOptions{
		PayloadOffset:  500,
		TimestampScale: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if block.Lacing != LacingXiph {
		t.Fatalf("lacing=%v", block.Lacing)
	}
	wantSizes := []int64{300, 5, 7}
	cursor := int64(508)
	for i, frame := range block.Frames {
		if frame.Start != cursor || frame.Size() != wantSizes[i] {
			t.Errorf("frame %d range [%d,%d), want size %d", i, frame.Start, frame.End, wantSizes[i])
		}
		cursor = frame.End
	}
}

func TestEBMLLacingAllowNegativeDelta(t *testing.T) {
	data := make([]byte, 800+500+1000)
	sizes := []byte{0x43, 0x20, 0x5E, 0xD3}
	payload := blockPayload(0x06, append([]byte{2}, sizes...), data, -3)
	block, err := ParseBlock(payload, BlockOptions{
		PayloadOffset:        2000,
		ClusterTimecode:      1000,
		TimestampScale:       1_000_000,
		DefaultFrameDuration: 10_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if block.Lacing != LacingEBML {
		t.Fatalf("lacing=%v", block.Lacing)
	}
	wantSizes := []int64{800, 500, 1000}
	cursor := int64(2009)
	for i, frame := range block.Frames {
		if frame.Start != cursor || frame.Size() != wantSizes[i] {
			t.Errorf("frame %d range [%d,%d), want size %d", i, frame.Start, frame.End, wantSizes[i])
		}
		cursor = frame.End
	}
}

func TestFirstFrameLengthError(t *testing.T) {
	payload := blockPayload(0x06, []byte{1, 0x8A}, nil, 0)
	_, err := ParseBlock(payload, BlockOptions{PayloadOffset: 10})
	if !errors.Is(err, ErrFirstFrameSize) {
		t.Fatalf("got %v, want ErrFirstFrameSize", err)
	}
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("got %v, want nested ErrTruncated", err)
	}
}

func TestNegativeDeltaError(t *testing.T) {
	payload := blockPayload(0x06, []byte{2, 0x8A, 0xAB}, []byte("0123456789"), 0)
	_, err := ParseBlock(payload, BlockOptions{PayloadOffset: 10})
	if !errors.Is(err, ErrNegativeDelta) {
		t.Fatalf("got %v, want ErrNegativeDelta", err)
	}
}

func TestFixedLacingSizeMismatch(t *testing.T) {
	payload := blockPayload(0x04, []byte{2}, []byte("12345"), 0)
	_, err := ParseBlock(payload, BlockOptions{PayloadOffset: 10})
	if !errors.Is(err, ErrFrameSize) {
		t.Fatalf("got %v, want ErrFrameSize", err)
	}
}
