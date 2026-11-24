package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseRejectsDuplicateDataChunk(t *testing.T) {
	input := testWAV(t, [][]int16{{1, 2}})
	extra := []byte("data")
	extra = append(extra, 0, 0, 0, 0)
	patched := append(input[:len(input)-0], extra...)
	patched[4] = byte(len(patched) - 8)
	patched[5] = byte((len(patched) - 8) >> 8)

	_, err := parseWAV(bytes.NewReader(patched))
	if err == nil || !strings.Contains(err.Error(), "duplicate data") {
		t.Fatalf("err = %v, want duplicate data rejection", err)
	}
}

func TestParseRejectsCMAPCountMismatch(t *testing.T) {
	input := testWAV(t, [][]int16{{1, 2}})
	cmap := buildCMAPChunk(t, []string{"FL"})
	insertAt := 36
	patched := append([]byte(nil), input[:insertAt]...)
	patched = append(patched, cmap...)
	patched = append(patched, input[insertAt:]...)
	patched[4] = byte(len(patched) - 8)
	patched[5] = byte((len(patched) - 8) >> 8)

	_, err := parseWAV(bytes.NewReader(patched))
	if err == nil || !strings.Contains(err.Error(), "cmap declares") {
		t.Fatalf("err = %v, want cmap count mismatch", err)
	}
}

func TestReadPCM8BitIsUnsigned(t *testing.T) {
	if got := readPCM([]byte{0}); got != -128 {
		t.Fatalf("zero 8-bit value = %d, want -128", got)
	}
	if got := readPCM([]byte{255}); got != 127 {
		t.Fatalf("max 8-bit value = %d, want 127", got)
	}
}
