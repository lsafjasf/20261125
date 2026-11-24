package wav

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// WriteMono24 writes a sample-exact mono preview as 24-bit PCM. It only
// changes gain by clamping samples that already exceed full scale.
func WriteMono24(path string, sampleRate int, samples []float64) error {
	interleaved := make([]float64, len(samples))
	copy(interleaved, samples)
	return WritePCM24(path, sampleRate, 1, interleaved)
}

// WritePCM24 writes an interleaved 24-bit integer PCM WAV.
func WritePCM24(path string, sampleRate, channels int, samples []float64) error {
	if sampleRate <= 0 {
		return fmt.Errorf("invalid sample rate %d", sampleRate)
	}
	if channels < 1 {
		return fmt.Errorf("invalid channel count %d", channels)
	}
	if len(samples)%channels != 0 {
		return fmt.Errorf("interleaved sample count %d is not divisible by %d channels", len(samples), channels)
	}
	pcm := make([]byte, len(samples)*3)
	for i, sample := range samples {
		if math.IsNaN(sample) {
			sample = 0
		}
		sample = math.Max(-1, math.Min(1, sample))
		value := int32(math.Round(sample * 8388607))
		pcm[i*3] = byte(value)
		pcm[i*3+1] = byte(value >> 8)
		pcm[i*3+2] = byte(value >> 16)
	}
	blockAlign := 3
	blockAlign = channels * 3
	avgBytes := sampleRate * blockAlign
	dataSize := uint32(len(pcm))
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(avgBytes))
	binary.LittleEndian.PutUint16(header[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(header[34:36], 24)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err = file.Write(header); err == nil {
		_, err = file.Write(pcm)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
