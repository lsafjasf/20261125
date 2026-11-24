package audit

import (
	"encoding/binary"
	"fmt"
)

type rawEdit struct {
	segmentDuration uint64
	mediaTime       int64
	rateRaw         uint32
}

func parseELST(box *Box) (*ELSTInfo, []rawEdit, *Finding) {
	if box == nil {
		return nil, nil, &Finding{
			Severity: "warning", Code: "elst_missing",
			Message: "edts exists but elst not found", BoxPath: "/moov/trak/edts/elst",
		}
	}

	p, err := box.Payload()
	if err != nil {
		return nil, nil, &Finding{
			Severity: "error", Code: "elst_read_failed", Message: err.Error(),
			BoxPath: "/moov/trak/edts/elst",
		}
	}
	if len(p) < 8 {
		return nil, nil, &Finding{
			Severity: "error", Code: "short_elst_payload",
			Message: fmt.Sprintf("elst payload length %d < 8", len(p)),
			BoxPath: "/moov/trak/edts/elst",
		}
	}

	version := p[0]
	count := binary.BigEndian.Uint32(p[4:8])
	entrySize := 12
	if version == 1 {
		entrySize = 20
	}
	needed := 8 + int(count)*entrySize
	if len(p) < needed {
		return &ELSTInfo{Offset: box.Start, Version: version, EntryCount: count}, nil, &Finding{
			Severity: "error", Code: "truncated_elst_entries",
			Message: fmt.Sprintf("elst payload has %d bytes, %d entries need %d", len(p), count, needed),
			BoxPath: "/moov/trak/edts/elst",
		}
	}

	info := &ELSTInfo{Offset: box.Start, Version: version, EntryCount: count}
	entries := make([]rawEdit, 0, count)
	pos := 8
	for i := uint32(0); i < count; i++ {
		var edit rawEdit
		if version == 1 {
			edit.segmentDuration = binary.BigEndian.Uint64(p[pos : pos+8])
			edit.mediaTime = int64(binary.BigEndian.Uint64(p[pos+8 : pos+16]))
			edit.rateRaw = binary.BigEndian.Uint32(p[pos+16 : pos+20])
			pos += 20
		} else {
			edit.segmentDuration = uint64(binary.BigEndian.Uint32(p[pos : pos+4]))
			edit.mediaTime = int64(int32(binary.BigEndian.Uint32(p[pos+4 : pos+8])))
			edit.rateRaw = binary.BigEndian.Uint32(p[pos+8 : pos+12])
			pos += 12
		}
		entries = append(entries, edit)
	}
	return info, entries, nil
}
