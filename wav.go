package mixloud

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

type wavFmt struct {
	tag        uint16
	channels   uint16
	sampleRate uint32
	blockAlign uint16
	bits       uint16
}

func ReadWAVFile(path string) (*Audio, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadWAV(f)
}

func ReadWAV(r io.Reader) (*Audio, error) {
	var riff [12]byte
	if _, err := io.ReadFull(r, riff[:]); err != nil {
		return nil, fmt.Errorf("read RIFF header: %w", err)
	}
	if string(riff[0:4]) != "RIFF" || string(riff[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF/WAVE file")
	}

	var format wavFmt
	var pcm []byte
	var err error
	for {
		var header [8]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return nil, err
		}
		id := string(header[0:4])
		size := binary.LittleEndian.Uint32(header[4:8])
		body := make([]byte, size)
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, fmt.Errorf("read chunk %q: %w", id, err)
		}
		if size%2 == 1 {
			var pad [1]byte
			_, _ = r.Read(pad[:])
		}

		switch id {
		case "fmt ":
			format, err = parseWAVFormat(body)
			if err != nil {
				return nil, err
			}
		case "data":
			pcm = body
		}
	}

	if format.channels == 0 || format.sampleRate == 0 || format.bits == 0 {
		return nil, errors.New("missing or invalid fmt chunk")
	}
	if pcm == nil {
		return nil, errors.New("missing data chunk")
	}

	ch := int(format.channels)
	bytesPerSample := int(format.bits / 8)
	frameBytes := ch * bytesPerSample
	if format.blockAlign > 0 {
		frameBytes = int(format.blockAlign)
	}
	if frameBytes <= 0 || len(pcm) < frameBytes {
		return nil, errors.New("audio data is too short")
	}
	frames := len(pcm) / frameBytes
	out, err := NewAudio(int(format.sampleRate), ch, frames)
	if err != nil {
		return nil, err
	}

	for i := 0; i < frames; i++ {
		base := i * frameBytes
		for c := 0; c < ch; c++ {
			off := base + c*bytesPerSample
			if off+bytesPerSample > len(pcm) {
				break
			}
			out.Channels[c][i] = decodeSample(pcm[off:off+bytesPerSample], format)
		}
	}
	return out, nil
}

func parseWAVFormat(b []byte) (wavFmt, error) {
	if len(b) < 16 {
		return wavFmt{}, errors.New("fmt chunk is too short")
	}
	f := wavFmt{
		tag:        binary.LittleEndian.Uint16(b[0:2]),
		channels:   binary.LittleEndian.Uint16(b[2:4]),
		sampleRate: binary.LittleEndian.Uint32(b[4:8]),
		blockAlign: binary.LittleEndian.Uint16(b[12:14]),
		bits:       binary.LittleEndian.Uint16(b[14:16]),
	}
	if f.tag == 0xFFFE && len(b) >= 40 {
		f.tag = binary.LittleEndian.Uint16(b[24:26])
	}
	if f.tag != 1 && f.tag != 3 {
		return f, fmt.Errorf("unsupported WAV format tag %d (only PCM/float)", f.tag)
	}
	if f.bits != 8 && f.bits != 16 && f.bits != 24 && f.bits != 32 {
		return f, fmt.Errorf("unsupported bit depth %d", f.bits)
	}
	if f.tag == 3 && f.bits != 32 {
		return f, fmt.Errorf("unsupported float bit depth %d", f.bits)
	}
	return f, nil
}

func decodeSample(b []byte, f wavFmt) float64 {
	switch f.bits {
	case 8:
		return (float64(b[0]) - 128) / 128
	case 16:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768
	case 24:
		v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
		if v&0x800000 != 0 {
			v |= -1 << 24
		}
		return float64(v) / 8388608
	case 32:
		if f.tag == 3 {
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
		}
		return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
	default:
		return 0
	}
}

func WriteWAVFile(path string, a *Audio, float32 bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return WriteWAV(f, a, float32)
}

func WriteWAV(w io.Writer, a *Audio, useFloat32 bool) error {
	if a == nil || a.Frames() == 0 {
		return errors.New("cannot write empty audio")
	}
	channels := a.NumChannels()
	bits := uint16(16)
	formatTag := uint16(1)
	if useFloat32 {
		bits = 32
		formatTag = 3
	}
	blockAlign := uint16(channels * int(bits/8))
	byteRate := uint32(a.SampleRate) * uint32(blockAlign)
	dataSize := uint32(a.Frames() * int(blockAlign))

	var head [44]byte
	copy(head[0:4], "RIFF")
	binary.LittleEndian.PutUint32(head[4:8], 36+dataSize)
	copy(head[8:12], "WAVE")
	copy(head[12:16], "fmt ")
	binary.LittleEndian.PutUint32(head[16:20], 16)
	binary.LittleEndian.PutUint16(head[20:22], formatTag)
	binary.LittleEndian.PutUint16(head[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(head[24:28], uint32(a.SampleRate))
	binary.LittleEndian.PutUint32(head[28:32], byteRate)
	binary.LittleEndian.PutUint16(head[32:34], blockAlign)
	binary.LittleEndian.PutUint16(head[34:36], bits)
	copy(head[36:40], "data")
	binary.LittleEndian.PutUint32(head[40:44], dataSize)
	if _, err := w.Write(head[:]); err != nil {
		return err
	}

	buf := make([]byte, 4*channels)
	for i := 0; i < a.Frames(); i++ {
		for c := 0; c < channels; c++ {
			x := a.Channels[c][i]
			if useFloat32 {
				binary.LittleEndian.PutUint32(buf[c*4:c*4+4], math.Float32bits(float32(x)))
			} else {
				if x > 1 {
					x = 1
				} else if x < -1 {
					x = -1
				}
				v := int16(math.Round(x * 32767))
				binary.LittleEndian.PutUint16(buf[c*2:c*2+2], uint16(v))
			}
		}
		stride := int(blockAlign)
		if _, err := w.Write(buf[:stride]); err != nil {
			return err
		}
	}
	return nil
}
