package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	pcmFormat        uint16 = 1
	extensibleFormat uint16 = 65534

	cmapID = "cmap"
	fmtID  = "fmt "
	dataID = "data"
)

var pcmSubtype = [16]byte{
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00,
	0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71,
}

type chunk struct {
	id     string
	offset int
	size   int
	data   []byte
}

type formatInfo struct {
	audioFormat   uint16
	channels      uint16
	sampleRate    uint32
	bitsPerSample uint16
	mask          uint32
	hasMask       bool
}

type channelMap struct {
	offset int
	size   int
	names  []string
}

type wavFile struct {
	raw      []byte
	riffSize int
	chunks   []chunk
	format   formatInfo
	data     chunk
	cmap     *channelMap
}

func parseWAV(input io.Reader) (*wavFile, error) {
	raw, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}
	if len(raw) < 44 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF/WAVE file")
	}

	w := &wavFile{
		raw:      raw,
		riffSize: int(binary.LittleEndian.Uint32(raw[4:8])),
	}
	if w.riffSize != len(raw)-8 {
		return nil, fmt.Errorf("RIFF size declares %d bytes but container has %d", w.riffSize, len(raw)-8)
	}
	pos := 12
	var fmtChunk *chunk
	var dataChunk *chunk
	for pos < len(raw) {
		if pos+8 > len(raw) {
			return nil, errors.New("incomplete RIFF chunk header")
		}
		id := string(raw[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(raw[pos+4 : pos+8]))
		start := pos + 8
		if size > len(raw)-start {
			return nil, fmt.Errorf("chunk %q extends past end of file", id)
		}
		end := start + size
		current := chunk{id: id, offset: pos, size: size, data: raw[start:end]}
		w.chunks = append(w.chunks, current)
		switch id {
		case fmtID:
			if fmtChunk != nil {
				return nil, errors.New("duplicate fmt chunk")
			}
			f := current
			fmtChunk = &f
		case dataID:
			if dataChunk != nil {
				return nil, errors.New("duplicate data chunk")
			}
			d := current
			dataChunk = &d
			w.data = d
		case cmapID:
			if w.cmap != nil {
				return nil, errors.New("duplicate cmap chunk")
			}
			mapInfo, err := parseChannelMap(pos, size, current.data)
			if err != nil {
				return nil, err
			}
			w.cmap = mapInfo
		}
		padding := size % 2
		if padding != 0 {
			if end >= len(raw) || raw[end] != 0 {
				return nil, fmt.Errorf("chunk %q has invalid padding byte", id)
			}
		}
		pos = end + padding
	}
	if pos != len(raw) {
		return nil, errors.New("trailing bytes after RIFF chunks")
	}

	if fmtChunk == nil {
		return nil, errors.New("missing fmt chunk")
	}
	if w.data.id == "" {
		return nil, errors.New("missing data chunk")
	}
	if err := w.parseFormat(*fmtChunk); err != nil {
		return nil, err
	}
	if int(w.format.channels) == 0 {
		return nil, errors.New("zero channels in fmt chunk")
	}
	if w.cmap != nil && len(w.cmap.names) != int(w.format.channels) {
		return nil, fmt.Errorf("cmap declares %d channels but fmt declares %d", len(w.cmap.names), w.format.channels)
	}
	if w.format.bitsPerSample%8 != 0 {
		return nil, fmt.Errorf("unsupported sample width: %d bits", w.format.bitsPerSample)
	}
	if err := w.validateFrameLayout(); err != nil {
		return nil, err
	}
	return w, nil
}

func parseChannelMap(offset, size int, data []byte) (*channelMap, error) {
	if size < 4 {
		return nil, errors.New("cmap chunk is too small")
	}
	count := int(binary.LittleEndian.Uint32(data[0:4]))
	if count == 0 || count > (size-4)/16 {
		return nil, errors.New("invalid cmap channel count")
	}
	expectedSize := 4 + 16*count
	if size != expectedSize {
		return nil, errors.New("cmap chunk contains unused bytes")
	}
	names := make([]string, count)
	for i := range names {
		start := 4 + i*16
		label := trimZero(data[start : start+16])
		if label == "" {
			return nil, fmt.Errorf("cmap channel %d has an empty name", i+1)
		}
		names[i] = label
	}
	return &channelMap{offset: offset, size: size, names: names}, nil
}

func trimZero(value []byte) string {
	for i, b := range value {
		if b == 0 {
			return string(value[:i])
		}
	}
	return string(value)
}

func (w *wavFile) parseFormat(c chunk) error {
	if c.size < 16 {
		return errors.New("fmt chunk is too small")
	}
	d := c.data
	w.format = formatInfo{
		audioFormat:   binary.LittleEndian.Uint16(d[0:2]),
		channels:      binary.LittleEndian.Uint16(d[2:4]),
		sampleRate:    binary.LittleEndian.Uint32(d[4:8]),
		bitsPerSample: binary.LittleEndian.Uint16(d[14:16]),
	}
	switch w.format.audioFormat {
	case pcmFormat:
		return nil
	case extensibleFormat:
		if c.size < 40 {
			return errors.New("WAVEFORMATEXTENSIBLE fmt chunk is too small")
		}
		var subtype [16]byte
		copy(subtype[:], d[24:40])
		if subtype != pcmSubtype {
			return errors.New("only uncompressed PCM is supported")
		}
		w.format.mask = binary.LittleEndian.Uint32(d[20:24])
		w.format.hasMask = true
		return nil
	default:
		return fmt.Errorf("unsupported audio format: %d", w.format.audioFormat)
	}
}

func (w *wavFile) validateFrameLayout() error {
	channels := int(w.format.channels)
	bytesPerSample := int(w.format.bitsPerSample) / 8
	blockAlign := int(binary.LittleEndian.Uint16(w.fmtChunk().data[12:14]))
	if blockAlign != channels*bytesPerSample {
		return fmt.Errorf("block align %d does not equal channels %d × sample bytes %d", blockAlign, channels, bytesPerSample)
	}
	if w.data.size%(channels*bytesPerSample) != 0 {
		return errors.New("data payload does not contain whole interleaved frames")
	}
	return nil
}

func (w *wavFile) fmtChunk() chunk {
	for _, c := range w.chunks {
		if c.id == fmtID {
			return c
		}
	}
	panic("missing fmt chunk")
}

func (w *wavFile) frameCount() int {
	return w.data.size / w.blockAlign()
}

func (w *wavFile) blockAlign() int {
	return int(w.format.channels) * (int(w.format.bitsPerSample) / 8)
}

func (w *wavFile) declaredNames() []string {
	if w.cmap != nil {
		return append([]string(nil), w.cmap.names...)
	}
	if w.format.hasMask {
		return maskNames(w.format.mask)
	}
	return numberedNames(int(w.format.channels))
}
