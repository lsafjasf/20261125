package audit

import (
	"ffv1mkaudit/ebml"
)

func (p *parser) collectUnknowns() {
	p.walkRange(0, p.size, 0, 0)
}

func (p *parser) walkRange(start, end int64, parent uint32, depth int) {
	if depth > 24 || start < 0 || end > p.size || start >= end {
		return
	}
	offset := start
	for offset < end {
		h, err := ebml.ReadHeaderAt(p.r, offset)
		if err != nil {
			return
		}
		childEnd := h.DataOffset + int64(h.Size)
		if h.UnknownSize {
			childEnd = end
		}
		if childEnd > p.size {
			childEnd = p.size
		}
		if childEnd <= offset {
			return
		}
		if !ebml.IsKnown(h.ID) {
			parentName := ""
			if parent != 0 {
				parentName = ebml.HexIDPadded(parent, idByteWidth(parent))
			}
			p.report.UnknownElements = append(p.report.UnknownElements, UnknownElement{
				ID:           ebml.HexIDPadded(h.ID, h.IDSize),
				Name:         ebml.Name(h.ID),
				ParentID:     parentName,
				Offset:       h.Offset,
				HeaderSize:   h.HeaderSize,
				DataOffset:   h.DataOffset,
				DeclaredSize: int64(h.Size),
				SizeUnknown:  h.UnknownSize,
			})
		} else if ebml.IsContainer(h.ID) && h.ID != ebml.IDSimpleBlock && h.ID != ebml.IDBlock {
			p.verifyContainerCRC(h, childEnd)
			limit := childEnd
			if limit > end {
				limit = end
			}
			p.walkRange(h.DataOffset, limit, h.ID, depth+1)
		}
		offset = childEnd
	}
}

func idByteWidth(id uint32) int {
	switch {
	case id <= 0xff:
		return 1
	case id <= 0xffff:
		return 2
	case id <= 0xffffff:
		return 3
	default:
		return 4
	}
}
