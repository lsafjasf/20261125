package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

const guidSuffix = "\x00\x00\x00\x00\x10\x00\x80\x00\x00\xaa\x00\x38\x9b\x71"

// File holds decoded, normalized interleaved PCM. Original bytes are not
// modified; playback previews are written to separate files.
type File struct {
	SampleRate   int
	Channels     int
	Bits         int
	ValidBits    int
	BlockAlign   int
	ChannelMask  uint32
	Layout       []string
	LayoutSource string
	Data         []float64
	Frames       int
	Duration     time.Duration
	PCMBytes     int
	Path         string
}

func Read(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF/WAVE file")
	}
	var (
		fmtData, data        []byte
		audioFormat          uint16
		channels, sampleRate int
		blockAlign, bits     int
		channelMask          uint32
		validBits            int
		cmapNames            []string
		haveFmt, haveData    bool
	)
	for pos := 12; pos+8 <= len(raw); {
		id := string(raw[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(raw[pos+4 : pos+8]))
		start, end := pos+8, pos+8+size
		if size < 0 || end < start || end > len(raw) {
			return nil, fmt.Errorf("chunk %q overruns file at offset %d", id, pos)
		}
		switch id {
		case "fmt ":
			fmtData, haveFmt = raw[start:end], true
		case "data":
			data, haveData = raw[start:end], true
		case "cmap":
			cmapNames = parseCmap(raw[start:end])
		}
		pos = end + size%2
	}
	if !haveFmt || !haveData {
		return nil, errors.New("missing fmt or data chunk")
	}
	if len(fmtData) < 16 {
		return nil, errors.New("fmt chunk is too short")
	}
	audioFormat = binary.LittleEndian.Uint16(fmtData[0:2])
	channels = int(binary.LittleEndian.Uint16(fmtData[2:4]))
	sampleRate = int(binary.LittleEndian.Uint32(fmtData[4:8]))
	blockAlign = int(binary.LittleEndian.Uint16(fmtData[12:14]))
	bits = int(binary.LittleEndian.Uint16(fmtData[14:16]))
	validBits = bits
	if audioFormat == 0xfffe {
		if len(fmtData) < 40 || binary.LittleEndian.Uint16(fmtData[16:18]) < 22 {
			return nil, errors.New("malformed WAVE_FORMAT_EXTENSIBLE fmt chunk")
		}
		validBits = int(binary.LittleEndian.Uint16(fmtData[18:20]))
		channelMask = binary.LittleEndian.Uint32(fmtData[20:24])
		if string(fmtData[26:40]) != guidSuffix {
			return nil, errors.New("unsupported extensible sub-format")
		}
		audioFormat = binary.LittleEndian.Uint16(fmtData[24:26])
	}
	if channels < 1 || channels > 32 {
		return nil, fmt.Errorf("unsupported channel count %d", channels)
	}
	if sampleRate < 8000 || sampleRate > 768000 {
		return nil, fmt.Errorf("unsupported sample rate %d", sampleRate)
	}
	bytesPerSample := (bits + 7) / 8
	if blockAlign != channels*bytesPerSample || blockAlign <= 0 {
		return nil, fmt.Errorf("invalid block align %d for %d x %d-bit", blockAlign, channels, bits)
	}
	if len(data)%blockAlign != 0 {
		return nil, errors.New("data chunk ends in a partial sample frame")
	}
	if audioFormat != 1 && audioFormat != 3 {
		return nil, fmt.Errorf("unsupported WAVE format 0x%04x", audioFormat)
	}
	if audioFormat == 1 && bits != 8 && bits != 16 && bits != 24 && bits != 32 {
		return nil, fmt.Errorf("unsupported PCM depth %d", bits)
	}
	if audioFormat == 3 && bits != 32 && bits != 64 {
		return nil, fmt.Errorf("unsupported float depth %d", bits)
	}
	samples, err := decode(data, audioFormat, bits, validBits)
	if err != nil {
		return nil, err
	}
	layout := cmapNames
	source := "cmap"
	if len(layout) != channels {
		layout = LayoutFromMask(channelMask, channels)
		source = "fmt channel mask"
	}
	if len(layout) != channels {
		layout = defaultLayout(channels)
		source = "default order (inferred, not verified)"
	}
	frames := len(samples) / channels
	return &File{
		SampleRate: sampleRate, Channels: channels, Bits: bits, ValidBits: validBits,
		BlockAlign: blockAlign, ChannelMask: channelMask, Layout: layout,
		LayoutSource: source, Data: samples, Frames: frames,
		Duration: time.Duration(float64(frames) / float64(sampleRate) * float64(time.Second)),
		PCMBytes: len(data), Path: filepath.Clean(path),
	}, nil
}

func parseCmap(payload []byte) []string {
	if len(payload) < 4 {
		return nil
	}
	count := int(binary.LittleEndian.Uint32(payload[0:4]))
	if count < 1 || count > 32 || len(payload) < 4+count*16 {
		return nil
	}
	out := make([]string, count)
	for i := 0; i < count; i++ {
		field := payload[4+i*16 : 4+(i+1)*16]
		field = bytes.TrimRight(field, "\x00")
		out[i] = normalizeName(string(field))
	}
	return out
}

func decode(data []byte, format uint16, bits, validBits int) ([]float64, error) {
	width := (bits + 7) / 8
	out := make([]float64, len(data)/width)
	scale := math.Ldexp(1, validBits-1)
	for i := range out {
		offset := i * width
		switch {
		case format == 3 && bits == 32:
			out[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[offset:])))
		case format == 3 && bits == 64:
			out[i] = math.Float64frombits(binary.LittleEndian.Uint64(data[offset:]))
		case bits == 8:
			out[i] = (float64(data[offset]) - 128) / 128
		case bits == 16:
			out[i] = float64(int16(binary.LittleEndian.Uint16(data[offset:]))) / scale
		case bits == 24:
			value := int32(data[offset]) | int32(data[offset+1])<<8 | int32(data[offset+2])<<16
			value = value << 8 >> 8
			out[i] = float64(value) / scale
		case bits == 32:
			out[i] = float64(int32(binary.LittleEndian.Uint32(data[offset:]))) / scale
		default:
			return nil, fmt.Errorf("cannot decode %d-bit format 0x%04x", bits, format)
		}
	}
	return out, nil
}

func (f *File) Channel(index int) []float64 {
	out := make([]float64, f.Frames)
	for frame := 0; frame < f.Frames; frame++ {
		out[frame] = f.Data[frame*f.Channels+index]
	}
	return out
}

func (f *File) ChannelIndex(name string) int {
	want := normalizeName(name)
	for i, current := range f.Layout {
		if normalizeName(current) == want {
			return i
		}
	}
	return -1
}
