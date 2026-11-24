package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestRepair24BitSamples(t *testing.T) {
	frames := []struct{ left, right int32 }{
		{1, -1000},
		{2, -2000},
	}
	data := bytes.NewBuffer(nil)
	for _, frame := range frames {
		writeInt24(data, frame.left)
		writeInt24(data, frame.right)
	}
	input := buildWAV(t, 24, data.Bytes())
	w := mustParse(t, input)

	result, err := repairWAV(w, repairRequest{
		mode:    modeSamples,
		mapping: []int{1, 0},
	})
	if err != nil {
		t.Fatalf("repairWAV: %v", err)
	}
	repaired := mustParse(t, result.output)
	if got, want := firstSamples(repaired), []int64{-1000, 1}; !intsEqual(got, want) {
		t.Fatalf("first frame = %v, want %v", got, want)
	}
}

func writeInt24(w *bytes.Buffer, value int32) {
	var buffer [4]byte
	binary.LittleEndian.PutUint32(buffer[:], uint32(value))
	w.Write(buffer[:3])
}

func buildWAV(t *testing.T, bitsPerSample int, data []byte) []byte {
	t.Helper()
	channels := 2
	blockAlign := channels * bitsPerSample / 8
	var output bytes.Buffer
	output.WriteString("RIFF")
	writeUint32(&output, uint32(36+len(data)))
	output.WriteString("WAVE")
	output.WriteString("fmt ")
	writeUint32(&output, 16)
	writeUint16(&output, pcmFormat)
	writeUint16(&output, uint16(channels))
	writeUint32(&output, 48000)
	writeUint32(&output, 48000*uint32(blockAlign))
	writeUint16(&output, uint16(blockAlign))
	writeUint16(&output, uint16(bitsPerSample))
	output.WriteString("data")
	writeUint32(&output, uint32(len(data)))
	output.Write(data)
	return output.Bytes()
}
