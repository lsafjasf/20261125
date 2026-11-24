package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepairSamplesReordersPCMAndKeepsDeclaration(t *testing.T) {
	original := testWAV(t, [][]int16{{1, 10}, {2, 20}, {3, 30}})
	w := mustParse(t, original)

	result, err := repairWAV(w, repairRequest{
		mode:    modeSamples,
		mapping: []int{1, 0},
	})
	if err != nil {
		t.Fatalf("repairWAV: %v", err)
	}
	if !result.dataChanged {
		t.Fatal("expected PCM payload to change")
	}
	if result.fmtChanged || result.cmapChanged {
		t.Fatalf("samples mode changed declaration: fmt=%t cmap=%t", result.fmtChanged, result.cmapChanged)
	}

	repaired := mustParse(t, result.output)
	if got, want := firstSamples(repaired), []int64{10, 1}; !intsEqual(got, want) {
		t.Fatalf("first frame = %v, want %v", got, want)
	}
	if got, want := repaired.declaredNames(), w.declaredNames(); !strings.EqualFold(strings.Join(got, ","), strings.Join(want, ",")) {
		t.Fatalf("declared names = %v, want %v", got, want)
	}
	if bytes.Equal(result.output, original) {
		t.Fatal("output bytes unexpectedly equal original")
	}
}

func TestRepairDeclarationKeepsPCMPayload(t *testing.T) {
	original := testWAV(t, [][]int16{{1, 10}, {2, 20}, {3, 30}})
	w := mustParse(t, original)

	result, err := repairWAV(w, repairRequest{
		mode:    modeDeclaration,
		mapping: []int{0, 1},
		names:   []string{"FR", "FL"},
	})
	if err != nil {
		t.Fatalf("repairWAV: %v", err)
	}
	if result.dataChanged {
		t.Fatal("declaration mode changed PCM payload")
	}
	if result.afterData != result.beforeData {
		t.Fatalf("data hash changed: before=%s after=%s", result.beforeData, result.afterData)
	}

	repaired := mustParse(t, result.output)
	if got, want := repaired.cmap.names, []string{"FR", "FL"}; !strings.EqualFold(strings.Join(got, ","), strings.Join(want, ",")) {
		t.Fatalf("cmap = %v, want %v", got, want)
	}
	if got, want := firstSamples(repaired), []int64{1, 10}; !intsEqual(got, want) {
		t.Fatalf("first frame = %v, want %v", got, want)
	}
}

func TestRepairExistingDeclarationKeepsPCMPayload(t *testing.T) {
	original := testWAV(t, [][]int16{{1, 10}, {2, 20}, {3, 30}})
	withMap := insertCMAP(t, original, []string{"CH1", "CH2"})
	w := mustParse(t, withMap)

	result, err := repairWAV(w, repairRequest{
		mode:    modeDeclaration,
		mapping: []int{0, 1},
		names:   []string{"FR", "FL"},
	})
	if err != nil {
		t.Fatalf("repairWAV: %v", err)
	}
	if result.dataChanged {
		t.Fatal("declaration replacement changed PCM payload")
	}
	repaired := mustParse(t, result.output)
	if got, want := repaired.cmap.names, []string{"FR", "FL"}; !strings.EqualFold(strings.Join(got, ","), strings.Join(want, ",")) {
		t.Fatalf("cmap = %v, want %v", got, want)
	}
	if repaired.data.offset != w.data.offset {
		t.Fatalf("data offset changed from %d to %d for same-size cmap", w.data.offset, repaired.data.offset)
	}
}

func TestSamplesModePreservesExistingCMAP(t *testing.T) {
	withMap := insertCMAP(t, testWAV(t, [][]int16{{1, 10}, {2, 20}}), []string{"CH1", "CH2"})
	w := mustParse(t, withMap)
	result, err := repairWAV(w, repairRequest{mode: modeSamples, mapping: []int{1, 0}})
	if err != nil {
		t.Fatalf("repairWAV: %v", err)
	}
	repaired := mustParse(t, result.output)
	if got, want := repaired.cmap.names, []string{"CH1", "CH2"}; !strings.EqualFold(strings.Join(got, ","), strings.Join(want, ",")) {
		t.Fatalf("cmap = %v, want %v", got, want)
	}
	if repaired.data.offset != w.data.offset {
		t.Fatalf("data offset changed from %d to %d", w.data.offset, repaired.data.offset)
	}
}

func TestInvalidMappingRejected(t *testing.T) {
	w := mustParse(t, testWAV(t, [][]int16{{1, 10}}))
	_, err := repairWAV(w, repairRequest{mode: modeSamples, mapping: []int{0, 0}})
	if err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("err = %v, want repeated source position", err)
	}
}

func TestDeclarationModeRejectsSampleReorder(t *testing.T) {
	w := mustParse(t, testWAV(t, [][]int16{{1, 10}}))
	_, err := repairWAV(w, repairRequest{
		mode:    modeDeclaration,
		mapping: []int{1, 0},
		names:   []string{"FR", "FL"},
	})
	if err == nil || !strings.Contains(err.Error(), "must not reorder samples") {
		t.Fatalf("err = %v, want declaration mode reorder rejection", err)
	}
}

func TestBackupReadbackAndRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.wav")
	original := testWAV(t, [][]int16{{1, 10}, {2, 20}})
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	backup, err := createBackup(path, time.Date(2026, 11, 25, 1, 2, 3, 4, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	repaired := testWAV(t, [][]int16{{10, 1}, {20, 2}})
	if err := atomicWrite(path, repaired); err != nil {
		t.Fatal(err)
	}
	hash, err := readback(path, repaired)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if hash != hashBytes(repaired) {
		t.Fatalf("readback hash = %s, want %s", hash, hashBytes(repaired))
	}

	rolledHash, err := rollback(path, backup.path)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rolledHash != hashBytes(original) {
		t.Fatalf("rollback hash = %s, want %s", rolledHash, hashBytes(original))
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, original) {
		t.Fatal("rollback did not restore original bytes")
	}
}

func TestCreateBackupUsesPredictableCollisionSuffix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.wav")
	if err := os.WriteFile(path, testWAV(t, [][]int16{{1, 2}}), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 11, 25, 1, 2, 3, 4, time.UTC)
	first, err := createBackup(path, when)
	if err != nil {
		t.Fatal(err)
	}
	second, err := createBackup(path, when)
	if err != nil {
		t.Fatal(err)
	}
	want := first.path + ".1"
	if second.path != want {
		t.Fatalf("second backup = %q, want %q", second.path, want)
	}
}

func testWAV(t *testing.T, frames [][]int16) []byte {
	t.Helper()
	channels := 2
	dataSize := len(frames) * channels * 2
	var output bytes.Buffer
	output.WriteString("RIFF")
	writeUint32(&output, uint32(36+dataSize))
	output.WriteString("WAVE")
	output.WriteString("fmt ")
	writeUint32(&output, 16)
	writeUint16(&output, pcmFormat)
	writeUint16(&output, uint16(channels))
	writeUint32(&output, 48000)
	writeUint32(&output, 48000*uint32(channels)*2)
	writeUint16(&output, uint16(channels*2))
	writeUint16(&output, 16)
	output.WriteString("data")
	writeUint32(&output, uint32(dataSize))
	for _, frame := range frames {
		for _, sample := range frame {
			writeUint16(&output, uint16(sample))
		}
	}
	return output.Bytes()
}

func mustParse(t *testing.T, input []byte) *wavFile {
	t.Helper()
	w, err := parseWAV(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("parseWAV: %v", err)
	}
	return w
}

func intsEqual(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
