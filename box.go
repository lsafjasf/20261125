package mp4diag

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"unicode"
)

var containerBoxes = map[string]bool{
	"moov": true,
	"trak": true,
	"mdia": true,
	"minf": true,
	"stbl": true,
	"edts": true,
	"mvex": true,
	"moof": true,
	"traf": true,
	"udta": true,
}

type rawBox struct {
	info         BoxInfo
	header       [16]byte
	headerLength int64
}

func readBoxHeader(r io.ReaderAt, offset, limit int64) (rawBox, error) {
	if limit-offset < 8 {
		return rawBox{}, fmt.Errorf("truncated box header at %d: %d byte(s) remain", offset, maxInt64(0, limit-offset))
	}
	var header [16]byte
	if _, err := r.ReadAt(header[:8], offset); err != nil {
		return rawBox{}, err
	}
	size32 := binary.BigEndian.Uint32(header[0:4])
	boxType := string(header[4:8])
	headerSize := int64(8)
	declared := uint64(size32)

	if size32 == 1 {
		if limit-offset < 16 {
			return rawBox{}, fmt.Errorf("truncated 64-bit box header at %d", offset)
		}
		if _, err := r.ReadAt(header[8:16], offset+8); err != nil {
			return rawBox{}, err
		}
		declared = binary.BigEndian.Uint64(header[8:16])
		headerSize = 16
	} else if size32 == 0 {
		declared = uint64(limit - offset)
	}

	if declared > math.MaxInt64 {
		return rawBox{}, fmt.Errorf("box size %d at %d overflows int64", declared, offset)
	}
	info := BoxInfo{
		Type:         boxType,
		Offset:       offset,
		HeaderSize:   headerSize,
		DeclaredSize: int64(declared),
		DeclaredEnd:  offset + int64(declared),
		ActualEnd:    limit,
		Valid:        true,
	}
	if int64(declared) < headerSize {
		info.Valid = false
		info.IssueCode = "box_size_smaller_than_header"
		info.Issue = fmt.Sprintf("declared size %d is smaller than header %d", declared, headerSize)
	} else if info.DeclaredEnd > limit {
		info.Valid = false
		info.IssueCode = "box_end_beyond_parent"
		info.Issue = fmt.Sprintf("box ends at %d but parent/file boundary is %d", info.DeclaredEnd, limit)
	} else if !printableFourCC(boxType) {
		info.Valid = false
		info.IssueCode = "invalid_box_type"
		info.Issue = "box type contains non-printable characters"
	}
	return rawBox{info: info, header: header, headerLength: headerSize}, nil
}

func printableFourCC(value string) bool {
	for _, r := range value {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func scanBoxes(r io.ReaderAt, start, limit int64, parentPath string, depth int, boxes *[]BoxInfo, findings *[]Finding) {
	if depth > 12 || start >= limit {
		return
	}
	siblingCount := map[string]int{}
	for offset := start; offset < limit; {
		raw, err := readBoxHeader(r, offset, limit)
		if err != nil {
			loc := &BoxLocator{Path: parentPath + "/<truncated>", Type: "?", Offset: offset}
			*findings = append(*findings, Finding{
				Layer:    LayerContainer,
				Severity: SeverityError,
				Code:     "box_header_truncated",
				Message:  err.Error(), Box: loc,
			})
			*boxes = append(*boxes, BoxInfo{Path: loc.Path, Type: "?", Offset: offset, Valid: false, IssueCode: "box_header_truncated", Issue: err.Error()})
			return
		}

		siblingCount[raw.info.Type]++
		name := raw.info.Type
		if siblingCount[raw.info.Type] > 1 {
			name = fmt.Sprintf("%s[%d]", raw.info.Type, siblingCount[raw.info.Type])
		}
		raw.info.Path = parentPath + "/" + name
		*boxes = append(*boxes, raw.info)

		if !raw.info.Valid {
			*findings = append(*findings, Finding{
				Layer: LayerContainer, Severity: SeverityError, Code: raw.info.IssueCode,
				Message: raw.info.Issue,
				Box:     &BoxLocator{Path: raw.info.Path, Type: raw.info.Type, Offset: raw.info.Offset},
			})
			return
		}

		payload := offset + raw.headerLength
		boxEnd := raw.info.DeclaredEnd
		if containerBoxes[raw.info.Type] {
			scanBoxes(r, payload, boxEnd, raw.info.Path, depth+1, boxes, findings)
		}
		offset = boxEnd
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
