package repair

import "encoding/binary"

// remapExtraTrak returns a copy of a raw non-video trak box whose chunk
// offsets are translated into the freshly written output layout. Box lengths
// never change, so the same bytes remain structurally valid.
func (p *Plan) remapExtraTrak(raw []byte, payloadStart int64) []byte {
	out := append([]byte(nil), raw...)

	stblAbs, ok := findBoxPath(out, "mdia", "minf", "stbl")
	if !ok {
		return out
	}
	stbl := out[stblAbs:]

	if stcoRel, found := childBoxStart(stbl, "stco"); found {
		box := stblAbs + stcoRel
		count := binary.BigEndian.Uint32(out[box+12 : box+16])
		entries := box + 16
		for i := uint32(0); i < count; i++ {
			at := entries + int(i)*4
			old := uint64(binary.BigEndian.Uint32(out[at : at+4]))
			if neu, ok := p.mapOffsetOK(int64(old), payloadStart); ok && neu < uint64(1<<32) {
				binary.BigEndian.PutUint32(out[at:at+4], uint32(neu))
			}
		}
		return out
	}

	if coRel, found := childBoxStart(stbl, "co64"); found {
		box := stblAbs + coRel
		count := binary.BigEndian.Uint32(out[box+12 : box+16])
		entries := box + 16
		for i := uint32(0); i < count; i++ {
			at := entries + int(i)*8
			old := binary.BigEndian.Uint64(out[at : at+8])
			if neu, ok := p.mapOffsetOK(int64(old), payloadStart); ok {
				binary.BigEndian.PutUint64(out[at:at+8], neu)
			}
		}
	}
	return out
}

// findBoxPath resolves parent/child box types from the start of buf, which
// must itself be the first parent box (e.g. trak).
func findBoxPath(buf []byte, path ...string) (int, bool) {
	base := 0
	cur := buf
	for _, name := range path {
		rel, ok := childBoxStart(cur, name)
		if !ok {
			return 0, false
		}
		base += rel
		cur = cur[rel:]
	}
	return base, true
}

// childBoxStart returns the index of the wanted child box header relative to
// the start of parent (a full box including its 8/16 byte header).
func childBoxStart(parent []byte, want string) (int, bool) {
	if len(parent) < 8 {
		return 0, false
	}
	header := 8
	pos := 8
	size := int(binary.BigEndian.Uint32(parent[0:4]))
	if size == 1 && len(parent) >= 16 {
		header = 16
		pos = 16
		size = int(binary.BigEndian.Uint64(parent[8:16]))
	}
	end := len(parent)
	if size > 0 && size < end {
		end = size
	}
	_ = header
	for pos+8 <= end {
		s := int(binary.BigEndian.Uint32(parent[pos : pos+4]))
		t := string(parent[pos+4 : pos+8])
		h := 8
		if s == 1 && pos+16 <= end {
			s = int(binary.BigEndian.Uint64(parent[pos+8 : pos+16]))
			h = 16
		}
		if t == want {
			return pos, true
		}
		if s < h || pos+s > end {
			return 0, false
		}
		pos += s
	}
	return 0, false
}
