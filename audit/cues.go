package audit

import (
	"ffv1mkaudit/ebml"
)

func (p *parser) parseCues(cues ebml.Header) {
	p.forChildren(cues, func(pointH ebml.Header, pointEnd int64) {
		if pointH.ID != ebml.IDCuePoint {
			return
		}
		cueTime := int64(0)
		p.forChildren(pointH, func(h ebml.Header, end int64) {
			switch h.ID {
			case ebml.IDCueTime:
				if value, ok := p.readUint(h); ok {
					cueTime = int64(value)
				}
			case ebml.IDCueTrackPos:
				cue := CuePosition{CueTimeNS: cueTime * int64(p.report.TimecodeScaleNS)}
				p.parseCuePosition(h, &cue)
				cue.PositionOK = p.resolveCuePosition(&cue)
				p.report.CuePositions = append(p.report.CuePositions, cue)
				if !cue.PositionOK {
					p.addIssue(Issue{
						Severity: SeverityWarning,
						Code:     "cue_position_unresolved",
						Message:  "CueClusterPosition does not point to a Cluster element",
						Offset:   h.Offset,
					})
				}
			}
		})
	})
}

func (p *parser) parseCuePosition(h ebml.Header, cue *CuePosition) {
	p.forChildren(h, func(child ebml.Header, end int64) {
		value, ok := p.readUint(child)
		if !ok {
			return
		}
		switch child.ID {
		case ebml.IDCueTrack:
			cue.Track = value
		case ebml.IDCueClusterPos:
			cue.ClusterPosition = value
		case ebml.IDCueRelativePos:
			cue.RelativePosition = value
		case ebml.IDCueBlockNumber:
			cue.BlockNumber = value
		}
	})
}

func (p *parser) resolveCuePosition(cue *CuePosition) bool {
	relative := int64(cue.ClusterPosition)
	candidates := []int64{}
	if p.hasSeg {
		candidates = append(candidates, p.seg.DataOffset+relative, p.seg.Offset+relative)
	}
	candidates = append(candidates, relative)
	for _, offset := range candidates {
		if offset < 0 || offset >= p.size {
			continue
		}
		h, err := ebml.ReadHeaderAt(p.r, offset)
		if err == nil && h.ID == ebml.IDCluster {
			cue.ResolvedOffset = offset
			cue.ResolvedElementID = ebml.HexIDPadded(h.ID, h.IDSize)
			return true
		}
	}
	return false
}
