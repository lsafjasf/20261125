// Package wavio reads uncompressed PCM and IEEE-float WAV files using only
// the Go standard library. No resampling, denoising or level normalisation
// is performed: samples are converted to float64 in their raw relative
// amplitudes so all forensic evidence is preserved.
package wavio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
)

type WAV struct {
	SampleRate    int
	Channels      int
	BitsPerSample int
	AudioFormat   uint16
	// Mono is the channel-downmix (arithmetic mean) of the raw channels.
	Mono []float64
	// Frames holds the original interleaved channels, frame-major.
	Frames [][]float64
}

func ReadFile(path string) (*WAV, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

func Parse(raw []byte) (*WAV, error) {
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF/WAVE file")
	}
	var (
		audioFormat, channels, bps uint16
		sampleRate, byteRate       uint32
		blockAlign                 uint16
		data                       []byte
	)
	for pos := 12; pos+8 <= len(raw); {
		id := string(raw[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(raw[pos+4 : pos+8]))
		body := pos + 8
		if body+size > len(raw) {
			return nil, fmt.Errorf("truncated chunk %q", id)
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, errors.New("fmt chunk too small")
			}
			audioFormat = binary.LittleEndian.Uint16(raw[body:])
			channels = binary.LittleEndian.Uint16(raw[body+2:])
			sampleRate = binary.LittleEndian.Uint32(raw[body+4:])
			byteRate = binary.LittleEndian.Uint32(raw[body+8:])
			blockAlign = binary.LittleEndian.Uint16(raw[body+12:])
			bps = binary.LittleEndian.Uint16(raw[body+14:])
		case "data":
			data = append(data, raw[body:body+size]...)
		}
		pos = body + size
		if size%2 == 1 {
			pos++
		}
	}
	if data == nil {
		return nil, errors.New("no data chunk")
	}
	if channels == 0 || sampleRate == 0 {
		return nil, errors.New("invalid fmt header")
	}
	if audioFormat != 1 && audioFormat != 3 {
		return nil, fmt.Errorf("unsupported audio format %d (only PCM=1 and FLOAT=3)", audioFormat)
	}
	w := &WAV{
		SampleRate:    int(sampleRate),
		Channels:      int(channels),
		BitsPerSample: int(bps),
		AudioFormat:   audioFormat,
	}
	frames, mono, err := decodeSamples(data, int(audioFormat), int(channels), int(bps))
	if err != nil {
		return nil, err
	}
	w.Frames, w.Mono = frames, mono
	_ = byteRate
	_ = blockAlign
	return w, nil
}

func decodeSamples(data []byte, format, ch, bps int) ([][]float64, []float64, error) {
	bytesPerSample := bps / 8
	if bytesPerSample == 0 || len(data)%bytesPerSample != 0 {
		return nil, nil, errors.New("bad sample width")
	}
	totalSamples := len(data) / bytesPerSample
	if totalSamples%ch != 0 {
		return nil, nil, errors.New("data length not a multiple of channel count")
	}
	nframes := totalSamples / ch
	frames := make([][]float64, nframes)
	mono := make([]float64, nframes)
	r := bytes.NewReader(data)
	for f := 0; f < nframes; f++ {
		row := make([]float64, ch)
		sum := 0.0
		for c := 0; c < ch; c++ {
			v, err := readOne(r, format, bps)
			if err != nil {
				return nil, nil, err
			}
			row[c] = v
			sum += v
		}
		frames[f] = row
		mono[f] = sum / float64(ch)
	}
	return frames, mono, nil
}

func readOne(r *bytes.Reader, format, bps int) (float64, error) {
	switch {
	case format == 3 && bps == 32:
		var u uint32
		if err := binary.Read(r, binary.LittleEndian, &u); err != nil {
			return 0, err
		}
		return float64(float32FromBits(u)), nil
	case format == 3 && bps == 64:
		var u uint64
		if err := binary.Read(r, binary.LittleEndian, &u); err != nil {
			return 0, err
		}
		return float64(float64FromBits(u)), nil
	case format == 1 && bps == 16:
		var u int16
		if err := binary.Read(r, binary.LittleEndian, &u); err != nil {
			return 0, err
		}
		return float64(u) / 32768.0, nil
	case format == 1 && bps == 24:
		buf := make([]byte, 3)
		if _, err := r.Read(buf); err != nil {
			return 0, err
		}
		u := int32(buf[0]) | int32(buf[1])<<8 | int32(buf[2])<<16
		if u&0x800000 != 0 {
			u |= ^0xffffff
		}
		return float64(u) / 8388608.0, nil
	case format == 1 && bps == 32:
		var u int32
		if err := binary.Read(r, binary.LittleEndian, &u); err != nil {
			return 0, err
		}
		return float64(u) / 2147483648.0, nil
	case format == 1 && bps == 8:
		var u uint8
		if err := binary.Read(r, binary.LittleEndian, &u); err != nil {
			return 0, err
		}
		return (float64(u) - 128) / 128.0, nil
	default:
		return 0, fmt.Errorf("unsupported format/bps combination: %d/%d", format, bps)
	}
}

func float32FromBits(u uint32) float32 {
	return math.Float32frombits(u)
}

func float64FromBits(u uint64) float64 {
	return math.Float64frombits(u)
}
