package audio

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
)

const (
	formatPCM         = 1
	formatIEEEFloat   = 3
	formatExtensible  = 0xfffe
	maxDecodedSamples = 100_000_000
)

// WAV keeps the uploaded bytes untouched. Interleaved is a decoded, normalized
// analysis copy; nothing in this package writes back to Original.
type WAV struct {
	Original      []byte
	SHA256        string
	Format        uint16
	Channels      int
	SampleRate    int
	BitsPerSample int
	FrameCount    int
	Duration      float64
	Interleaved   []float64
}

type chunk struct {
	id   string
	data []byte
}

// Parse accepts PCM and IEEE-float RIFF/WAVE files, including WAVE_FORMAT_EXTENSIBLE
// wrappers. The original byte slice is retained verbatim.
func Parse(raw []byte) (*WAV, error) {
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF/WAVE file")
	}
	if int64(len(raw)) > math.MaxUint32 {
		return nil, errors.New("wav file is too large")
	}
	chunks, err := readChunks(raw)
	if err != nil {
		return nil, err
	}
	fmtData, ok := findChunk(chunks, "fmt ")
	if !ok || len(fmtData) < 16 {
		return nil, errors.New("missing or truncated fmt chunk")
	}
	format, channels, sampleRate, blockAlign, bits, err := parseFormat(fmtData)
	if err != nil {
		return nil, err
	}
	data, ok := findChunk(chunks, "data")
	if !ok {
		return nil, errors.New("missing data chunk")
	}
	if channels <= 0 || channels > 64 {
		return nil, fmt.Errorf("unsupported channel count %d", channels)
	}
	if sampleRate <= 0 || sampleRate > 768000 {
		return nil, fmt.Errorf("unsupported sample rate %d", sampleRate)
	}
	if blockAlign <= 0 {
		blockAlign = channels * bits / 8
	}
	if blockAlign <= 0 || len(data) < blockAlign {
		return nil, errors.New("invalid or empty audio data")
	}
	frames := len(data) / blockAlign
	if int64(frames)*int64(channels) > maxDecodedSamples {
		return nil, fmt.Errorf("audio is too large to analyze safely (%d decoded samples)", int64(frames)*int64(channels))
	}
	samples, err := decodeSamples(data[:frames*blockAlign], format, channels, bits, blockAlign)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	return &WAV{
		Original:      raw,
		SHA256:        hex.EncodeToString(sum[:]),
		Format:        format,
		Channels:      channels,
		SampleRate:    sampleRate,
		BitsPerSample: bits,
		FrameCount:    frames,
		Duration:      float64(frames) / float64(sampleRate),
		Interleaved:   samples,
	}, nil
}

func readChunks(raw []byte) ([]chunk, error) {
	var out []chunk
	for pos := 12; pos+8 <= len(raw); {
		id := string(raw[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(raw[pos+4 : pos+8]))
		pos += 8
		if size < 0 || pos+size < pos || pos+size > len(raw) {
			if id == "data" {
				size = len(raw) - pos
			} else {
				return nil, fmt.Errorf("truncated %q chunk", id)
			}
		}
		out = append(out, chunk{id: id, data: raw[pos : pos+size]})
		pos += size
		if size&1 == 1 && pos < len(raw) {
			pos++
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no RIFF chunks found")
	}
	return out, nil
}

func findChunk(chunks []chunk, id string) ([]byte, bool) {
	for _, c := range chunks {
		if c.id == id {
			return c.data, true
		}
	}
	return nil, false
}

func parseFormat(fmtData []byte) (format uint16, channels int, sampleRate int, blockAlign int, bits int, err error) {
	format = binary.LittleEndian.Uint16(fmtData[0:2])
	channels = int(binary.LittleEndian.Uint16(fmtData[2:4]))
	sampleRate = int(binary.LittleEndian.Uint32(fmtData[4:8]))
	blockAlign = int(binary.LittleEndian.Uint16(fmtData[12:14]))
	bits = int(binary.LittleEndian.Uint16(fmtData[14:16]))
	if format == formatExtensible {
		if len(fmtData) < 26 {
			return 0, 0, 0, 0, 0, errors.New("truncated WAVE_FORMAT_EXTENSIBLE header")
		}
		format = binary.LittleEndian.Uint16(fmtData[24:26])
	}
	switch format {
	case formatPCM:
		if bits != 8 && bits != 16 && bits != 24 && bits != 32 {
			return 0, 0, 0, 0, 0, fmt.Errorf("unsupported PCM bit depth %d", bits)
		}
	case formatIEEEFloat:
		if bits != 32 && bits != 64 {
			return 0, 0, 0, 0, 0, fmt.Errorf("unsupported float bit depth %d", bits)
		}
	default:
		return 0, 0, 0, 0, 0, fmt.Errorf("unsupported WAV format %d", format)
	}
	wantAlign := channels * bits / 8
	if blockAlign == 0 {
		blockAlign = wantAlign
	}
	if blockAlign < wantAlign {
		return 0, 0, 0, 0, 0, fmt.Errorf("invalid block alignment %d", blockAlign)
	}
	return format, channels, sampleRate, blockAlign, bits, nil
}

func decodeSamples(data []byte, format uint16, channels, bits, blockAlign int) ([]float64, error) {
	if len(data)%blockAlign != 0 {
		return nil, errors.New("audio data is not block aligned")
	}
	frames := len(data) / blockAlign
	out := make([]float64, frames*channels)
	pos := 0
	for frame := 0; frame < frames; frame++ {
		base := frame * blockAlign
		for ch := 0; ch < channels; ch++ {
			off := base + ch*bits/8
			var v float64
			switch {
			case format == formatIEEEFloat && bits == 32:
				v = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[off : off+4])))
			case format == formatIEEEFloat && bits == 64:
				v = math.Float64frombits(binary.LittleEndian.Uint64(data[off : off+8]))
			case format == formatPCM && bits == 8:
				v = (float64(data[off]) - 128) / 128
			case format == formatPCM && bits == 16:
				v = float64(int16(binary.LittleEndian.Uint16(data[off:off+2]))) / 32768
			case format == formatPCM && bits == 24:
				u := uint32(data[off]) | uint32(data[off+1])<<8 | uint32(data[off+2])<<16
				if u&0x800000 != 0 {
					u |= 0xff000000
				}
				v = float64(int32(u)) / 8388608
			case format == formatPCM && bits == 32:
				v = float64(int32(binary.LittleEndian.Uint32(data[off:off+4]))) / 2147483648
			default:
				return nil, errors.New("unsupported sample encoding")
			}
			if math.IsNaN(v) || math.IsInf(v, 0) {
				v = 0
			}
			out[pos] = v
			pos++
		}
	}
	return out, nil
}

// Channel returns a copy-free, strided view helper for the requested channel.
// Callers should not retain it after reusing the WAV.
func (w *WAV) Channel(ch int) []float64 {
	if ch < 0 || ch >= w.Channels {
		return nil
	}
	out := make([]float64, w.FrameCount)
	for i := range out {
		out[i] = w.Interleaved[i*w.Channels+ch]
	}
	return out
}

// EncodePCM16 writes a canonical little-endian mono/stereo PCM16 WAV. It is used
// only to build demo material; loaded originals are never re-encoded by the API.
func EncodePCM16(channels, sampleRate int, samples []float64) ([]byte, error) {
	if channels <= 0 || sampleRate <= 0 || len(samples)%channels != 0 {
		return nil, errors.New("invalid PCM16 parameters")
	}
	dataSize := len(samples) * 2
	buf := bytes.NewBuffer(make([]byte, 0, 44+dataSize))
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(formatPCM))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate*channels*2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels*2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	for _, sample := range samples {
		v := int(math.Round(math.Max(-1, math.Min(1, sample)) * 32767))
		_ = binary.Write(buf, binary.LittleEndian, int16(v))
	}
	return buf.Bytes(), nil
}
