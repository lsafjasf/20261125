package ffv1

import (
	"encoding/binary"
	"fmt"
)

// Config is the decoded FFV1 Configuration Record carried in Matroska
// CodecPrivate (possibly behind a VFW BITMAPINFOHEADER wrapper).
type Config struct {
	Raw                []byte `json:"-"`
	StrippedVFW        bool
	Version            int
	MicroVersion       int
	CoderType          int
	Colorspace         int
	BitsPerRawSample   int
	ChromaPlanes       bool
	Log2HChromaSub     int
	Log2VChromaSub     int
	ExtraPlane         bool
	NumHSlices         int
	NumVSlices         int
	QuantTableSetCount int
	PixelFormat        string
	CRCStored          uint32
	CRCCalculated      uint32
	CRCOK              bool
	Parsed             bool
	Error              string
}

// ParseConfig decodes the FFV1 configuration record. codecID is the
// Matroska CodecID; for V_MS/VFW/FOURCC the 40-byte VFW wrapper is removed.
func ParseConfig(codecID string, data []byte) Config {
	cfg := Config{Raw: data}
	if stripped, ok := stripVFW(data); ok {
		cfg.Raw = stripped
		cfg.StrippedVFW = true
	}
	if len(cfg.Raw) < 8 {
		cfg.Error = "configuration record is too short"
		return cfg
	}
	cfg.CRCStored = binary.BigEndian.Uint32(cfg.Raw[len(cfg.Raw)-4:])
	cfg.CRCCalculated = CRC32Raw(cfg.Raw[:len(cfg.Raw)-4])
	cfg.CRCOK = cfg.CRCStored == cfg.CRCCalculated

	d := NewRangeDecoder(cfg.Raw)
	var state [32]byte
	for i := range state {
		state[i] = 128
	}
	read := func(signed bool) (int, bool) {
		v, err := d.symbol(&state, signed)
		if err != nil {
			cfg.Error = err.Error()
			return 0, false
		}
		return v, true
	}

	version, ok := read(false)
	if !ok {
		return cfg
	}
	cfg.Version = version
	if version < 2 {
		cfg.Error = fmt.Sprintf("configuration record version %d is not valid", version)
		return cfg
	}
	if version > 4 {
		cfg.Error = fmt.Sprintf("unsupported FFV1 version %d", version)
		return cfg
	}
	// The last four bytes are the configuration record CRC and are not
	// part of the range-coded stream.
	d.SetEnd(len(cfg.Raw) - 4)

	if version >= 3 {
		if cfg.MicroVersion, ok = read(false); !ok {
			return cfg
		}
	}
	if cfg.CoderType, ok = read(false); !ok {
		return cfg
	}
	if cfg.CoderType > 1 {
		for i := 1; i < 256; i++ {
			if _, ok := read(true); !ok {
				return cfg
			}
		}
	}
	if cfg.Colorspace, ok = read(false); !ok {
		return cfg
	}
	if version >= 1 {
		if cfg.BitsPerRawSample, ok = read(false); !ok {
			return cfg
		}
		if cfg.BitsPerRawSample == 0 {
			cfg.BitsPerRawSample = 8
		}
	} else {
		cfg.BitsPerRawSample = 8
	}
	cfg.ChromaPlanes = d.bit(&state[0]) != 0
	if cfg.Log2HChromaSub, ok = read(false); !ok {
		return cfg
	}
	if cfg.Log2VChromaSub, ok = read(false); !ok {
		return cfg
	}
	cfg.ExtraPlane = d.bit(&state[0]) != 0
	if version >= 3 {
		if cfg.NumHSlices, ok = read(false); !ok {
			return cfg
		}
		if cfg.NumVSlices, ok = read(false); !ok {
			return cfg
		}
		if cfg.QuantTableSetCount, ok = read(false); !ok {
			return cfg
		}
		cfg.NumHSlices++
		cfg.NumVSlices++
	} else {
		cfg.NumHSlices, cfg.NumVSlices = 1, 1
		cfg.QuantTableSetCount = 1
	}
	if cfg.NumHSlices < 1 || cfg.NumHSlices > 1024 ||
		cfg.NumVSlices < 1 || cfg.NumVSlices > 1024 {
		cfg.Error = fmt.Sprintf("invalid slice raster %dx%d", cfg.NumHSlices, cfg.NumVSlices)
		return cfg
	}
	cfg.PixelFormat = pixelFormat(cfg)
	cfg.Parsed = true
	return cfg
}

func stripVFW(data []byte) ([]byte, bool) {
	if len(data) < 44 {
		return nil, false
	}
	if string(data[16:20]) != "FFV1" {
		return nil, false
	}
	return data[40:], true
}

func pixelFormat(c Config) string {
	depth := ""
	if c.BitsPerRawSample > 8 {
		depth = fmt.Sprintf("%d", c.BitsPerRawSample)
	}
	if c.Colorspace == 1 {
		base := "gbr"
		if c.ExtraPlane {
			base = "gbra"
		}
		return base + "p" + depth
	}
	if c.Colorspace != 0 {
		return fmt.Sprintf("colorspace%d_%dbit", c.Colorspace, c.BitsPerRawSample)
	}
	if !c.ChromaPlanes {
		base := "gray"
		if c.ExtraPlane {
			base = "graya"
		}
		return base + depth
	}
	subs := "?"
	switch [2]int{c.Log2HChromaSub, c.Log2VChromaSub} {
	case [2]int{0, 0}:
		subs = "444"
	case [2]int{1, 0}:
		subs = "422"
	case [2]int{1, 1}:
		subs = "420"
	case [2]int{2, 0}:
		subs = "411"
	case [2]int{2, 1}:
		subs = "440"
	case [2]int{2, 2}:
		subs = "410"
	}
	base := "yuv"
	if c.ExtraPlane {
		base = "yuva"
	}
	return base + subs + "p" + depth
}

// Descriptor returns a compact human-readable summary.
func (c Config) Descriptor() string {
	if !c.Parsed {
		return "unparsed"
	}
	return fmt.Sprintf("v%d.%d coder=%d %s %dbit %dx%d slices",
		c.Version, c.MicroVersion, c.CoderType, c.PixelFormat,
		c.BitsPerRawSample, c.NumHSlices, c.NumVSlices)
}
