package wavio

import (
	"encoding/binary"
	"math"
	"os"
)

// WriteMono16 writes a mono 16-bit PCM WAV file from float64 samples in
// [-1,1]. Values are clipped rather than scaled, preserving relative
// amplitudes used by the analysis pipeline.
func WriteMono16(path string, mono []float64, sampleRate int) error {
	dataBytes := len(mono) * 2
	riffSize := 36 + dataBytes
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr := make([]byte, 44)
	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(riffSize))
	copy(hdr[8:12], "WAVE")
	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16)
	binary.LittleEndian.PutUint16(hdr[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(hdr[22:24], 1) // mono
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(hdr[32:34], 2)
	binary.LittleEndian.PutUint16(hdr[34:36], 16)
	copy(hdr[36:40], "data")
	binary.LittleEndian.PutUint32(hdr[40:44], uint32(dataBytes))
	if _, err := f.Write(hdr); err != nil {
		return err
	}
	buf := make([]byte, 2)
	for _, v := range mono {
		s := int32(math.Round(v * 32767))
		if s > 32767 {
			s = 32767
		} else if s < -32768 {
			s = -32768
		}
		binary.LittleEndian.PutUint16(buf, uint16(s))
		if _, err := f.Write(buf); err != nil {
			return err
		}
	}
	return nil
}
