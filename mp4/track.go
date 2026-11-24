package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

// ColorInfo carries colour tagging from either the container (colr box)
// or the codec bitstream (SPS VUI / vpcC).
type ColorInfo struct {
	Primaries     uint16 `json:"primaries"`
	Transfer      uint16 `json:"transfer"`
	Matrix        uint16 `json:"matrix"`
	FullRange     bool   `json:"fullRange"`
	RangeKnown    bool   `json:"rangeKnown"`
	Source        string `json:"source"`
	PrimariesName string `json:"primariesName"`
	TransferName  string `json:"transferName"`
	MatrixName    string `json:"matrixName"`
}

// SampleDescription is one entry of a track's stsd box.
type SampleDescription struct {
	Format        string     `json:"format"` // avc1, hev1, hvc1, vp09, av01, ...
	Width         int        `json:"width"`
	Height        int        `json:"height"`
	ContainerColr *ColorInfo `json:"containerColor,omitempty"`
	ColrType      string     `json:"colrType,omitempty"` // nclx / nclc / ""
	CodecConfig   []byte     `json:"-"`                  // avcC / hvcC / vpcC payload
	ConfigType    string     `json:"configType,omitempty"`
}

// Track is a parsed moov/trak subtree.
type Track struct {
	Index     int                 `json:"index"`
	ID        uint32              `json:"id"`
	Handler   string              `json:"handler"` // vide / soun / ...
	Duration  uint64              `json:"duration"`
	Timescale uint32              `json:"timescale"`
	Samples   []SampleDescription `json:"sampleDescriptions"`
}

// ParseFile walks the top-level boxes and returns the parsed tracks.
func ParseFile(r io.ReadSeeker, size int64) ([]*Track, error) {
	var moov *Box
	for {
		b, err := ReadBox(r, size)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if b.Type == "moov" {
			moov = b
		}
	}
	if moov == nil {
		return nil, fmt.Errorf("no moov box found")
	}
	var tracks []*Track
	for i, trak := range FindChildren(moov, "trak") {
		t, err := parseTrak(trak, i)
		if err != nil {
			return nil, fmt.Errorf("trak %d: %w", i, err)
		}
		tracks = append(tracks, t)
	}
	return tracks, nil
}

func parseTrak(trak *Box, index int) (*Track, error) {
	t := &Track{Index: index}
	if tkhd := FindChild(trak, "tkhd"); tkhd != nil {
		t.ID = parseTrackID(tkhd.Data)
	}
	mdia := FindChild(trak, "mdia")
	if mdia == nil {
		return t, nil
	}
	if hdlr := FindChild(mdia, "hdlr"); hdlr != nil && len(hdlr.Data) >= 12 {
		t.Handler = string(hdlr.Data[8:12])
	}
	if mdhd := FindChild(mdia, "mdhd"); mdhd != nil {
		t.Timescale, t.Duration = parseMdhd(mdhd.Data)
	}
	minf := FindChild(mdia, "minf")
	if minf == nil {
		return t, nil
	}
	stbl := FindChild(minf, "stbl")
	if stbl == nil {
		return t, nil
	}
	if stsd := FindChild(stbl, "stsd"); stsd != nil {
		sds, err := parseStsd(stsd.Data)
		if err != nil {
			return nil, err
		}
		t.Samples = sds
	}
	return t, nil
}

func parseTrackID(d []byte) uint32 {
	if len(d) < 4 {
		return 0
	}
	version := d[0]
	if version == 1 && len(d) >= 32 {
		return binary.BigEndian.Uint32(d[20:24])
	}
	if len(d) >= 24 {
		return binary.BigEndian.Uint32(d[12:16])
	}
	return 0
}

func parseMdhd(d []byte) (uint32, uint64) {
	if len(d) < 4 {
		return 0, 0
	}
	if d[0] == 1 && len(d) >= 32 {
		return binary.BigEndian.Uint32(d[20:24]), uint64(binary.BigEndian.Uint32(d[24:28]))
	}
	if len(d) >= 24 {
		return binary.BigEndian.Uint32(d[12:16]), uint64(binary.BigEndian.Uint32(d[16:20]))
	}
	return 0, 0
}
