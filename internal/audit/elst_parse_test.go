package audit

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestParseELSTVersion1(t *testing.T) {
	payload := make([]byte, 8+20)
	payload[0] = 1
	binary.BigEndian.PutUint32(payload[4:8], 1)
	binary.BigEndian.PutUint64(payload[8:16], 9_000_000_000)
	binary.BigEndian.PutUint64(payload[16:24], ^uint64(0))
	binary.BigEndian.PutUint32(payload[24:28], 0x00010000)

	boxBytes := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(boxBytes[0:4], uint32(len(boxBytes)))
	copy(boxBytes[4:8], "elst")
	copy(boxBytes[8:], payload)

	box := &Box{
		Type: "elst", Start: 0, HeaderSize: 8, Size: int64(len(boxBytes)),
		PayloadStart: 8, reader: bytes.NewReader(boxBytes),
	}
	info, entries, finding := parseELST(box)
	if finding != nil {
		t.Fatalf("parseELST finding: %+v", finding)
	}
	if info.Version != 1 || info.EntryCount != 1 {
		t.Fatalf("info=%+v", info)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries)=%d", len(entries))
	}
	if entries[0].segmentDuration != 9_000_000_000 || entries[0].mediaTime != -1 {
		t.Fatalf("entry=%+v", entries[0])
	}
}
