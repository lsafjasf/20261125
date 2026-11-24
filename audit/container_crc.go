package audit

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"

	"ffv1mkaudit/ebml"
)

func (p *parser) verifyContainerCRC(parent ebml.Header, end int64) {
	declaredEnd := parent.DataOffset + int64(parent.Size)
	if parent.UnknownSize || declaredEnd > p.size || end <= parent.DataOffset {
		return
	}
	first, err := ebml.ReadHeaderAt(p.r, parent.DataOffset)
	if err != nil || first.ID != ebml.IDCRC32 {
		return
	}
	check := ContainerCRC{
		ParentElementID:  ebml.HexIDPadded(parent.ID, parent.IDSize),
		ParentOffset:     parent.Offset,
		CRCElementOffset: first.Offset,
	}
	if first.Size != 4 {
		check.OK = false
		p.report.ContainerCRCs = append(p.report.ContainerCRCs, check)
		p.addIssue(Issue{
			Severity: SeverityError, Code: "matroska_crc_invalid_length",
			Message: fmt.Sprintf("CRC-32 element has size %d, want 4", first.Size),
			Offset:  first.Offset,
		})
		return
	}
	var storedBytes [4]byte
	if _, err := p.r.ReadAt(storedBytes[:], first.DataOffset); err != nil {
		return
	}
	stored := binary.LittleEndian.Uint32(storedBytes[:])
	crcElementEnd := first.DataOffset + 4
	hash := crc32.NewIEEE()
	if _, err := io.Copy(hash, io.NewSectionReader(p.r, crcElementEnd, end-crcElementEnd)); err != nil {
		p.addIssue(Issue{
			Severity: SeverityWarning, Code: "matroska_crc_read_failed",
			Message: err.Error(), Offset: first.Offset,
		})
		return
	}
	calculated := hash.Sum32()
	check.StoredCRC = fmt.Sprintf("%08X", stored)
	check.CalculatedCRC = fmt.Sprintf("%08X", calculated)
	check.OK = stored == calculated
	p.report.ContainerCRCs = append(p.report.ContainerCRCs, check)
	if !check.OK {
		p.addIssue(Issue{
			Severity: SeverityError, Code: "matroska_crc_mismatch",
			Message: fmt.Sprintf("%s CRC-32 mismatch: stored %08X calculated %08X", check.ParentElementID, stored, calculated),
			Offset:  first.Offset, EndOffset: crcElementEnd,
		})
	}
}
