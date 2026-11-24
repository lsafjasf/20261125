// Package dpx parses SMPTE 268M DPX file headers.
package dpx

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

// Generic header layout offsets (SMPTE 268M-2003).
const (
	offMagic       = 0
	offFileName    = 36
	offOrientation = 768
	offWidth       = 772
	offHeight      = 776
	// Film industry header.
	offFramePos  = 1772
	offSeqLen    = 1776
	offFrameRate = 1784
	offFrameID   = 1792
	offSlateInfo = 1824
	// Television industry header.
	offTimeCode   = 1980
	offUserBits   = 1984
	offTemporalFR = 2000

	genericHeaderSize = 2048
)

// Header holds the fields relevant to sequence reconstruction.
type Header struct {
	Path        string
	Width       uint32
	Height      uint32
	FramePos    uint32 // frame position in sequence (film header)
	HasFramePos bool
	SeqLen      uint32 // sequence length (frames), 0 if undefined
	FrameRate   float32
	TimeCode    TimeCode
	HasTimeCode bool
	HeaderName  string // file name stored inside the header
	FrameID     string
	SlateInfo   string
}

// TimeCode is a SMPTE time code broken into fields.
type TimeCode struct {
	Hour, Min, Sec, Frame int
}

// AbsFrames converts the time code to an absolute frame count at fps.
func (t TimeCode) AbsFrames(fps int) int {
	return ((t.Hour*60+t.Min)*60+t.Sec)*fps + t.Frame
}

func (t TimeCode) String() string {
	return fmt.Sprintf("%02d:%02d:%02d:%02d", t.Hour, t.Min, t.Sec, t.Frame)
}

// ParseFile reads the generic header of a DPX file.
func ParseFile(path string) (Header, error) {
	f, err := os.Open(path)
	if err != nil {
		return Header{}, err
	}
	defer f.Close()
	return Parse(f, path)
}

// Parse reads a DPX header from r. Only the first 2048 bytes are consumed.
func Parse(r io.Reader, path string) (Header, error) {
	buf := make([]byte, genericHeaderSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return Header{}, fmt.Errorf("%s: reading header: %w", path, err)
	}
	var bo binary.ByteOrder
	switch string(buf[offMagic : offMagic+4]) {
	case "SDPX":
		bo = binary.BigEndian
	case "XPDS":
		bo = binary.LittleEndian
	default:
		return Header{}, fmt.Errorf("%s: not a DPX file (bad magic)", path)
	}
	h := Header{Path: path}
	h.Width = bo.Uint32(buf[offWidth:])
	h.Height = bo.Uint32(buf[offHeight:])
	h.HeaderName = cstr(buf[offFileName : offFileName+100])
	h.FrameID = cstr(buf[offFrameID : offFrameID+32])
	h.SlateInfo = cstr(buf[offSlateInfo : offSlateInfo+100])

	h.FramePos = bo.Uint32(buf[offFramePos:])
	h.HasFramePos = h.FramePos != 0xFFFFFFFF
	h.SeqLen = bo.Uint32(buf[offSeqLen:])
	if h.SeqLen == 0xFFFFFFFF {
		h.SeqLen = 0
	}
	h.FrameRate = readFloat32(bo, buf[offFrameRate:])
	if h.FrameRate <= 0 {
		h.FrameRate = readFloat32(bo, buf[offTemporalFR:])
	}

	rawTC := bo.Uint32(buf[offTimeCode:])
	if rawTC != 0xFFFFFFFF && rawTC != 0 {
		h.TimeCode = decodeBCDTimeCode(rawTC)
		h.HasTimeCode = true
	}
	return h, nil
}

// decodeBCDTimeCode interprets the DPX TV-header time code, stored as
// eight packed BCD digits 0xHHMMSSFF.
func decodeBCDTimeCode(v uint32) TimeCode {
	d := func(shift uint) int { return int((v >> shift) & 0xF) }
	return TimeCode{
		Hour:  d(28)*10 + d(24),
		Min:   d(20)*10 + d(16),
		Sec:   d(12)*10 + d(8),
		Frame: d(4)*10 + d(0),
	}
}

func readFloat32(bo binary.ByteOrder, b []byte) float32 {
	return float32FromBits(bo.Uint32(b))
}

func cstr(b []byte) string {
	if i := indexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// IsDPX reports whether the path looks like a DPX file by extension.
func IsDPX(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".dpx")
}
