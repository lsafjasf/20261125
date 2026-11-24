package mkv

import (
	"encoding/binary"

	"ffv1compare/internal/ebml"
)

func (p *parser) parseCluster(h ebml.Header) {
	var clusterTC int64
	p.children(h, func(ch ebml.Header, end int64) {
		switch ch.ID {
		case ebml.IDTimecode:
			if v, ok := p.readUint(ch, end); ok {
				clusterTC = int64(v)
			}
		case ebml.IDSimpleBlock:
			p.parseBlock(ch, end, clusterTC, true, false, "simpleblock_flag")
		case ebml.IDBlockGroup:
			p.parseBlockGroup(ch, clusterTC)
		}
	})
	p.verifyCRC(h)
}

func (p *parser) parseBlockGroup(h ebml.Header, clusterTC int64) {
	var block *ebml.Header
	hasReference := false
	p.children(h, func(ch ebml.Header, end int64) {
		switch ch.ID {
		case ebml.IDBlock:
			copy := ch
			block = &copy
		case ebml.IDReferenceBlock:
			hasReference = true
		}
	})
	if block == nil {
		p.addIssue(h.Offset, "blockgroup_no_block", "BlockGroup has no Block child")
		return
	}
	source := "blockgroup_without_reference"
	if hasReference {
		source = "blockgroup_reference"
	}
	p.parseBlock(*block, block.End(h.End(p.size)), clusterTC, false, !hasReference, source)
}

func (p *parser) parseBlock(h ebml.Header, end int64, clusterTC int64, simple, keyframe bool, source string) {
	if h.Size == 0 {
		p.addIssue(h.Offset, "block_empty", "block payload is empty")
		return
	}
	payload, err := ebml.ReadBytes(p.r, h.DataOffset, int64(h.Size))
	if err != nil {
		p.addIssue(h.Offset, "block_read", "%v", err)
		return
	}
	track, n, _, err := ebml.ReadVINT(payload)
	if err != nil || n+3 > len(payload) {
		p.addIssue(h.Offset, "block_header", "invalid block track/timecode/flags header")
		return
	}
	rel := int16(binary.BigEndian.Uint16(payload[n : n+2]))
	flags := payload[n+2]
	if simple {
		keyframe = flags&0x80 != 0
	}
	lacing := (flags >> 1) & 0x03
	if lacing != 0 {
		p.addIssue(h.Offset, "block_lacing", "laced blocks are not supported")
		return
	}
	sample := payload[n+3:]
	fr := Frame{
		Index:          len(p.doc.Frames),
		TrackNumber:    track,
		Keyframe:       keyframe,
		KeyframeSource: source,
		PTSNS:          (clusterTC + int64(rel)) * int64(p.doc.TimestampScaleNS),
		BlockOffset:    h.Offset,
		SampleOffset:   h.DataOffset + int64(n+3),
		SampleSize:     len(sample),
		Sample:         sample,
	}
	if t, ok := p.tracks[track]; ok && t.DefaultDurationNS > 0 {
		fr.DurationNS = int64(t.DefaultDurationNS)
	}
	if p.onFrame != nil {
		if err := p.onFrame(p.doc, fr); err != nil {
			p.err = err
		}
		return
	}
	p.doc.Frames = append(p.doc.Frames, fr)
}
