package audit

import (
	"ffv1mkaudit/ebml"
)

func (p *parser) forChildren(h ebml.Header, fn func(child ebml.Header, end int64)) {
	end := p.elementEnd(h)
	offset := h.DataOffset
	for offset < end {
		child, ok := p.readHeader(offset)
		if !ok {
			return
		}
		childEnd := p.elementEndWithin(child, end)
		fn(child, childEnd)
		if childEnd <= offset {
			return
		}
		offset = childEnd
	}
}

func (p *parser) parseEBMLHeader(root ebml.Header) {
	p.forChildren(root, func(h ebml.Header, end int64) {
		switch h.ID {
		case ebml.IDDocType:
			p.report.DocType = p.readString(h)
		case ebml.IDDocTypeVersion:
			if value, ok := p.readUint(h); ok {
				p.report.DocTypeVersion = int(value)
			}
		}
	})
}

func (p *parser) parseSegment(seg ebml.Header) {
	p.forChildren(seg, func(h ebml.Header, end int64) {
		switch h.ID {
		case ebml.IDInfo:
			p.parseInfo(h)
		case ebml.IDTracks:
			p.parseTracks(h)
		case ebml.IDCues:
			p.parseCues(h)
		case ebml.IDCluster:
			p.parseCluster(h)
		}
	})
}

func (p *parser) parseInfo(h ebml.Header) {
	p.forChildren(h, func(child ebml.Header, end int64) {
		switch child.ID {
		case ebml.IDTimecodeScale:
			if value, ok := p.readUint(child); ok && value > 0 {
				p.report.TimecodeScaleNS = value
			}
		}
	})
}

func (p *parser) parseTracks(h ebml.Header) {
	p.forChildren(h, func(child ebml.Header, end int64) {
		if child.ID == ebml.IDTrackEntry {
			track := p.parseTrackEntry(child)
			p.tracks[track.Number] = track
			p.report.Tracks = append(p.report.Tracks, track)
		}
	})
}

func (p *parser) parseTrackEntry(h ebml.Header) TrackInfo {
	track := TrackInfo{}
	p.forChildren(h, func(child ebml.Header, end int64) {
		switch child.ID {
		case ebml.IDTrackNumber:
			if value, ok := p.readUint(child); ok {
				track.Number = value
			}
		case ebml.IDTrackUID:
			if value, ok := p.readUint(child); ok {
				track.UID = value
			}
		case ebml.IDTrackType:
			if value, ok := p.readUint(child); ok {
				track.Type = value
			}
		case ebml.IDCodecID:
			track.CodecID = p.readString(child)
		case ebml.IDDefaultDuration:
			if value, ok := p.readUint(child); ok {
				track.DefaultDuration = value
			}
		case ebml.IDCodecPrivate:
			data := p.readBytes(child)
			analyzeCodecPrivate(&track, data, child.DataOffset, p)
		case ebml.IDVideo:
			p.parseVideo(child, &track)
		}
	})
	return track
}

func (p *parser) parseVideo(h ebml.Header, track *TrackInfo) {
	p.forChildren(h, func(child ebml.Header, end int64) {
		switch child.ID {
		case ebml.IDPixelWidth:
			if value, ok := p.readUint(child); ok {
				track.Width = value
			}
		case ebml.IDPixelHeight:
			if value, ok := p.readUint(child); ok {
				track.Height = value
			}
		}
	})
}

func isFFV1Codec(codecID string) bool {
	return codecID == "V_FFV1" || codecID == "FFV1"
}
