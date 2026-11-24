package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func writeUint16(w io.Writer, value uint16) {
	var buffer [2]byte
	binary.LittleEndian.PutUint16(buffer[:], value)
	w.Write(buffer[:])
}

func writeUint32(w io.Writer, value uint32) {
	var buffer [4]byte
	binary.LittleEndian.PutUint32(buffer[:], value)
	w.Write(buffer[:])
}

func buildCMAPChunk(t *testing.T, names []string) []byte {
	t.Helper()
	payload := make([]byte, 4+16*len(names))
	binary.LittleEndian.PutUint32(payload, uint32(len(names)))
	for i, name := range names {
		copy(payload[4+i*16:4+(i+1)*16], name)
	}
	var chunk bytes.Buffer
	chunk.WriteString(cmapID)
	writeUint32(&chunk, uint32(len(payload)))
	chunk.Write(payload)
	if len(payload)%2 == 1 {
		chunk.WriteByte(0)
	}
	return chunk.Bytes()
}

func insertCMAP(t *testing.T, input []byte, names []string) []byte {
	t.Helper()
	chunk := buildCMAPChunk(t, names)
	insertAt := 36
	output := append([]byte(nil), input[:insertAt]...)
	output = append(output, chunk...)
	output = append(output, input[insertAt:]...)
	binary.LittleEndian.PutUint32(output[4:8], uint32(len(output)-8))
	return output
}
