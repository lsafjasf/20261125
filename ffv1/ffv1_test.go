package ffv1

import "testing"

func appendCRCSlice(frame []byte, body []byte) []byte {
	sizeField := len(body)
	footer := []byte{byte(sizeField >> 16), byte(sizeField >> 8), byte(sizeField), 0}
	slicePrefix := append(append([]byte{}, body...), footer...)
	crc := CRC32Raw(slicePrefix)
	slice := append(slicePrefix, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
	return append(frame, slice...)
}

func appendNoCRCSlice(frame []byte, body []byte) []byte {
	sizeField := len(body)
	footer := []byte{byte(sizeField >> 16), byte(sizeField >> 8), byte(sizeField)}
	return append(append(frame, body...), footer...)
}

func TestAnalyzeCRCSliceChain(t *testing.T) {
	firstBody := append([]byte{0xFC, 0x15, 0x80}, make([]byte, 100)...)
	frame := appendCRCSlice(nil, firstBody)
	secondStart := len(frame)
	frame = appendCRCSlice(frame, make([]byte, 140))

	report := Analyze(frame, ModeCRC, 2)
	if report.SliceCount != 2 {
		t.Fatalf("SliceCount=%d, want 2; issues=%v", report.SliceCount, report.Issues)
	}
	if !report.ChainOK || report.UnparsedPrefix != 0 {
		t.Fatalf("chain not complete: ok=%v prefix=%d issues=%v", report.ChainOK, report.UnparsedPrefix, report.Issues)
	}
	if !report.CRCOK {
		t.Fatalf("CRCOK=false, slices=%+v", report.Slices)
	}
	if report.Slices[1].Start != secondStart {
		t.Fatalf("second start=%d, want %d", report.Slices[1].Start, secondStart)
	}
	if report.SliceCountOK != true {
		t.Fatalf("SliceCountOK=false")
	}
}

func TestAnalyzeDetectsCorruptSliceCRC(t *testing.T) {
	firstBody := append([]byte{0xFC}, []byte("slice-zero-body")...)
	frame := appendCRCSlice(nil, firstBody)
	badStart := len(frame)
	frame = appendCRCSlice(frame, []byte("slice-one-body"))
	frame[badStart+3] ^= 0xFF

	report := Analyze(frame, ModeCRC, 2)
	if report.SliceCount != 2 {
		t.Fatalf("SliceCount=%d issues=%v", report.SliceCount, report.Issues)
	}
	if report.CRCOK {
		t.Fatalf("CRCOK=true after byte flip")
	}
	if report.Slices[0].CRCStatus != CRCValid {
		t.Fatalf("slice 0 should remain valid: %+v", report.Slices[0])
	}
	if report.Slices[1].CRCStatus != CRCInvalid {
		t.Fatalf("slice 1 should be invalid: %+v", report.Slices[1])
	}
}

func TestAnalyzeNoCRCMode(t *testing.T) {
	firstBody := append([]byte{0x80}, make([]byte, 32)...)
	frame := appendNoCRCSlice(nil, firstBody)
	frame = appendNoCRCSlice(frame, make([]byte, 48))

	report := Analyze(frame, ModeNoCRC, 2)
	if report.SliceCount != 2 || !report.ChainOK {
		t.Fatalf("count=%d chain=%v issues=%v", report.SliceCount, report.ChainOK, report.Issues)
	}
	if report.CRCEnabled {
		t.Fatalf("CRCEnabled=true in no-CRC mode")
	}
}

func TestConfigCRC(t *testing.T) {
	config := []byte("configuration-record")
	crc := CRC32Raw(config)
	config = append(config, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
	_, _, ok := AnalyzeConfig(config)
	if !ok {
		t.Fatalf("configuration record CRC should validate")
	}
}
