package audit

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"ffv1mkaudit/ebml"
	"ffv1mkaudit/ffv1"
)

func (p *parser) parseCluster(h ebml.Header) {
	clusterTimecode := int64(0)
	p.clusterOffset = h.Offset
	p.forChildren(h, func(child ebml.Header, end int64) {
		switch child.ID {
		case ebml.IDTimecode:
			if value, ok := p.readUint(child); ok {
				clusterTimecode = int64(value)
			}
		case ebml.IDSimpleBlock:
			p.parseBlock(child, clusterTimecode, true, false, "")
		case ebml.IDBlockGroup:
			p.parseBlockGroup(child, clusterTimecode)
		}
	})
}

func (p *parser) parseBlockGroup(h ebml.Header, clusterTimecode int64) {
	var blockHeader *ebml.Header
	hasReference := false
	p.forChildren(h, func(child ebml.Header, end int64) {
		switch child.ID {
		case ebml.IDBlock:
			copy := child
			blockHeader = &copy
		case ebml.IDReferenceBlock:
			hasReference = true
		}
	})
	if blockHeader == nil {
		p.addIssue(Issue{
			Severity: SeverityWarning, Code: "block_group_without_block",
			Message: "BlockGroup does not contain a Block element", Offset: h.Offset,
		})
		return
	}
	keyframeSource := "blockgroup_without_referenceblock"
	if hasReference {
		keyframeSource = "blockgroup_referenceblock"
	}
	p.parseBlock(*blockHeader, clusterTimecode, false, !hasReference, keyframeSource)
}

func (p *parser) readBlockPayload(h ebml.Header) ([]byte, bool) {
	declared := int64(h.Size)
	if declared <= 0 {
		p.addIssue(Issue{Severity: SeverityError, Code: "block_empty", Message: "Block has zero declared size", Offset: h.Offset})
		return nil, false
	}
	if declared > p.opts.MaxFrameBytes {
		p.addIssue(Issue{
			Severity: SeverityError, Code: "block_too_large",
			Message: "Block exceeds --max-frame-bytes safety limit",
			Offset:  h.Offset, EndOffset: h.DataOffset + declared,
		})
		return nil, false
	}
	buf := make([]byte, declared)
	n, err := p.r.ReadAt(buf, h.DataOffset)
	if err != nil && !errors.Is(err, io.EOF) {
		p.addIssue(Issue{Severity: SeverityError, Code: "block_read_failed", Message: err.Error(), Offset: h.Offset})
		return buf[:n], false
	}
	if int64(n) < declared {
		p.addIssue(Issue{
			Severity: SeverityError, Code: "block_truncated",
			Message: "Block payload is truncated at end of file",
			Offset:  h.Offset, EndOffset: h.DataOffset + declared,
		})
		return buf[:n], false
	}
	return buf, true
}

func (p *parser) parseBlock(h ebml.Header, clusterTimecode int64, simple bool, keyframe bool, keySource string) {
	p.blockSeq++
	payload, complete := p.readBlockPayload(h)
	if len(payload) == 0 {
		p.appendUnreadableFrame(h, clusterTimecode, simple, keyframe, keySource, payload)
		return
	}

	track, vintLen, _, err := ebml.ReadVINT(payload)
	if err != nil || vintLen+3 > len(payload) {
		frame := p.newFrame(h, clusterTimecode, 0, simple, keyframe, keySource)
		frame.Status = "malformed"
		frame.Issues = append(frame.Issues, Issue{
			Severity: SeverityError, Code: "block_header_invalid",
			Message: "Block track/timecode/flags header is invalid or truncated",
			Offset:  h.DataOffset,
		})
		p.appendFrame(frame)
		return
	}

	relativeTimecode := int16(binary.BigEndian.Uint16(payload[vintLen : vintLen+2]))
	flags := payload[vintLen+2]
	if simple {
		keyframe = flags&0x80 != 0
		keySource = "simpleblock_keyframe_flag"
	}
	lacing := (flags >> 1) & 0x03
	samples, lacingIssue := splitLacedSamples(payload, vintLen+3, lacing)
	if lacingIssue != "" {
		frame := p.newFrame(h, clusterTimecode, relativeTimecode, simple, keyframe, keySource)
		frame.Status = "unsupported_lacing"
		frame.Issues = append(frame.Issues, Issue{
			Severity: SeverityError, Code: "block_lacing_unsupported", Message: lacingIssue,
			Offset: h.DataOffset,
		})
		p.appendFrame(frame)
		return
	}

	for _, sample := range samples {
		frame := p.newFrame(h, clusterTimecode, relativeTimecode, simple, keyframe, keySource)
		frame.BlockNumber = p.blockSeq
		frame.TrackNumber = track
		frame.BlockFlags = fmt.Sprintf("0x%02X", flags)
		frame.Size = len(sample.Data)
		frame.FrameOffset = h.DataOffset + int64(sample.PayloadOffset)
		frame.SHA256 = hashSHA256(sample.Data)
		if !complete {
			frame.Status = "truncated"
			frame.Issues = append(frame.Issues, Issue{
				Severity: SeverityError, Code: "block_truncated",
				Message: "frame bytes are incomplete", Offset: h.Offset,
			})
		}
		if p.trackIsFFV1(track) {
			p.applyFFV1(&frame, sample.Data, ffv1.ModeAuto)
		}
		p.appendFrame(frame)
	}
}

func (p *parser) newFrame(h ebml.Header, clusterTimecode int64, relative int16, simple, keyframe bool, source string) FrameReport {
	timestampNS := (clusterTimecode + int64(relative)) * int64(p.report.TimecodeScaleNS)
	return FrameReport{
		Index:             p.frameSeq,
		BlockNumber:       p.blockSeq,
		ClusterOffset:     p.clusterOffset,
		ClusterTimecode:   clusterTimecode,
		TimestampNS:       timestampNS,
		TimestampSeconds:  float64(timestampNS) / 1_000_000_000,
		BlockElementID:    ebml.HexIDPadded(h.ID, h.IDSize),
		ElementOffset:     h.Offset,
		BlockHeaderOffset: h.DataOffset,
		Position:          h.DataOffset,
		SamplePosition:    h.DataOffset,
		Status:            "ok",
		Keyframe:          keyframe,
		KeyframeSource:    source,
	}
}

func (p *parser) appendUnreadableFrame(h ebml.Header, clusterTC int64, simple, key bool, source string, payload []byte) {
	frame := p.newFrame(h, clusterTC, 0, simple, key, source)
	frame.BlockNumber = p.blockSeq
	frame.TrackNumber = 0
	frame.Size = len(payload)
	frame.FrameOffset = h.DataOffset
	frame.SHA256 = hashSHA256(payload)
	frame.Status = "unreadable"
	frame.Issues = append(frame.Issues, Issue{
		Severity: SeverityError, Code: "block_payload_unreadable",
		Message: "Block payload could not be read", Offset: h.Offset,
	})
	p.appendFrame(frame)
}

func (p *parser) appendFrame(frame FrameReport) {
	frame.FrameNumber = frame.Index + 1
	p.report.Frames = append(p.report.Frames, frame)
	p.report.FrameHashes = append(p.report.FrameHashes, FrameHash{
		FrameIndex: frame.Index, FrameNumber: frame.Index + 1,
		Offset: frame.FrameOffset, Size: frame.Size, SHA256: frame.SHA256,
	})
	p.frameSeq++
}

func (p *parser) trackIsFFV1(trackNumber uint64) bool {
	track, ok := p.tracks[trackNumber]
	if !ok {
		return true
	}
	if track.FFV1 {
		return true
	}
	if track.CodecID == "" {
		return track.Type == 0 || track.Type == 1
	}
	return isFFV1Codec(track.CodecID)
}

func hashSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
