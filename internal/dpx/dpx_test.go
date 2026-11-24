package dpx

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func buildHeader(t *testing.T, bo binary.ByteOrder, magic string) []byte {
	t.Helper()
	buf := make([]byte, genericHeaderSize)
	copy(buf[offMagic:], magic)
	bo.PutUint32(buf[offWidth:], 2048)
	bo.PutUint32(buf[offHeight:], 1556)
	bo.PutUint32(buf[offFramePos:], 42)
	bo.PutUint32(buf[offSeqLen:], 0xFFFFFFFF)
	bo.PutUint32(buf[offFrameRate:], math.Float32bits(24))
	bo.PutUint32(buf[offTimeCode:], 0x01123705) // 01:12:37:05
	copy(buf[offFileName:], "frame_0042.dpx")
	return buf
}

func TestParseBigEndian(t *testing.T) {
	h, err := Parse(bytes.NewReader(buildHeader(t, binary.BigEndian, "SDPX")), "f.dpx")
	if err != nil {
		t.Fatal(err)
	}
	if h.Width != 2048 || h.Height != 1556 {
		t.Errorf("bad raster %dx%d", h.Width, h.Height)
	}
	if !h.HasFramePos || h.FramePos != 42 {
		t.Errorf("bad frame pos %d", h.FramePos)
	}
	if h.SeqLen != 0 {
		t.Errorf("undefined seq len should map to 0, got %d", h.SeqLen)
	}
	if !h.HasTimeCode || h.TimeCode != (TimeCode{1, 12, 37, 5}) {
		t.Errorf("bad timecode %+v", h.TimeCode)
	}
	if got := h.TimeCode.AbsFrames(24); got != ((1*60+12)*60+37)*24+5 {
		t.Errorf("bad abs frames %d", got)
	}
	if h.HeaderName != "frame_0042.dpx" {
		t.Errorf("bad header name %q", h.HeaderName)
	}
}

func TestParseLittleEndian(t *testing.T) {
	h, err := Parse(bytes.NewReader(buildHeader(t, binary.LittleEndian, "XPDS")), "f.dpx")
	if err != nil {
		t.Fatal(err)
	}
	if h.Width != 2048 || h.FramePos != 42 || h.TimeCode.Hour != 1 {
		t.Errorf("bad little-endian parse: %+v", h)
	}
}

func TestParseRejectsNonDPX(t *testing.T) {
	if _, err := Parse(bytes.NewReader(make([]byte, genericHeaderSize)), "f.dpx"); err == nil {
		t.Error("expected magic error")
	}
	if _, err := Parse(bytes.NewReader([]byte("SDPX")), "f.dpx"); err == nil {
		t.Error("expected short-read error")
	}
}
