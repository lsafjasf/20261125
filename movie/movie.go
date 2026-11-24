// Package movie parses the small subset of MP4/MOV metadata needed for index
// rebuilding: top level mdat sections plus the video track sample tables.
package movie

import (
	"fmt"
	"io"
	"os"

	"mp4index/bmff"
)

// MediaSection describes one mdat box payload inside the source file.
type MediaSection struct {
	BoxStart       int64 // absolute offset of mdat size field
	PayloadOffset  int64 // absolute offset of first media byte
	PayloadLength  int64 // bytes available on disk (clamped to file size)
	DeclaredLength int64 // payload length from the box header (may exceed disk)
}

// SttsEntry is one time-to-sample run.
type SttsEntry struct {
	Count    uint32
	Duration uint32
}

// OldIndex keeps the pre-existing sample tables for diagnostics.
type OldIndex struct {
	SampleCount uint32   // stsz sample_count (0 means uniform size)
	UniformSize uint32   // stsz sample_size when non-zero
	Sizes       []uint32 // stsz entries (truncated to what fits)
	Offsets     []uint64 // stco / co64 entries
	Offsets64   bool
	SyncSamples []uint32 // stss entries
	Stts        []SttsEntry
	Stsc        []StscEntry
}

// StscEntry is one sample-to-chunk run.
type StscEntry struct {
	FirstChunk      uint32
	SamplesPerChunk uint32
	DescIndex       uint32
}

// Track is a parsed track.
type Track struct {
	TrackID    uint32
	Handler    string // "vide", "soun", ...
	TimeScale  uint32
	Duration   uint64
	Width      uint32
	Height     uint32
	StsdBox    []byte // raw stsd full box, reused verbatim in repaired output
	Codec      string // best effort sample entry fourcc (avc1/hvc1/...)
	LengthSize int    // NAL length prefix size derived from avcC/hvcC; 4 default
	Index      *OldIndex
	Ctts       []int32 // expanded per-sample composition offsets
	RawTrak    []byte  // complete original trak box (for preserving other tracks)
}

// Info is the parsed movie.
type Info struct {
	FileSize       int64
	FtypBox        []byte // raw ftyp box, may be nil
	HasMoov        bool
	MovieTimeScale uint32
	Tracks         []*Track
	Video          *Track
	Mdats          []MediaSection
}

// OpenAndParse opens path and parses its metadata.
func OpenAndParse(path string) (*os.File, *Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	info, err := Parse(f, st.Size())
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, info, nil
}

// Parse reads top level boxes through ra.
func Parse(ra io.ReaderAt, size int64) (*Info, error) {
	info := &Info{FileSize: size}
	var moov bmff.Box
	haveMoov := false

	// Custom top level loop: a truncated trailing box (very common for
	// incomplete recordings) must still be recorded with clamped payload.
	pos := int64(0)
	for pos+8 <= size {
		b, herr := bmff.ReadBoxHeader(ra, pos, size)
		if herr != nil {
			break
		}
		boxEnd := b.End()
		truncated := boxEnd > size

		func(b bmff.Box) {
			switch b.Type {
			case "ftyp":
				raw, err := bmff.ReadBoxBytes(ra, b)
				if err == nil {
					info.FtypBox = raw
				}
			case "moov":
				moov = b
				haveMoov = true
			case "mdat":
				declared := b.PayloadLength()
				available := declared
				pstart := b.PayloadStart()
				if pstart > size {
					available = 0
				} else if pstart+available > size || b.Size < 0 {
					available = size - pstart
				}
				if available < 0 {
					available = 0
				}
				info.Mdats = append(info.Mdats, MediaSection{
					BoxStart:       b.Start,
					PayloadOffset:  pstart,
					PayloadLength:  available,
					DeclaredLength: declared,
				})
			}
		}(b)

		if b.Size < 0 {
			break
		}
		if truncated {
			break
		}
		pos = boxEnd
	}

	if haveMoov {
		info.HasMoov = true
		if err := parseMoov(ra, moov, info); err != nil {
			return info, fmt.Errorf("parse moov: %w", err)
		}
	}
	return info, nil
}
