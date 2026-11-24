package anc

import "encoding/hex"

// Line is one captured video line. Provide either Words or Bytes and select the
// matching Encoding. Field uses 1/2 for interlaced video and 0 for
// progressive/unknown.
type Line struct {
	Field  int
	Number int

	// Encoding overrides the scanner default. Required meaningfully only when
	// Bytes is set.
	Encoding Encoding

	// Words contains explicit 10-bit ANC words (EncWords).
	Words []Word

	// Bytes is the raw line blob for u16be/u16le/anc8/v210 encodings.
	Bytes []byte

	// StreamOffset optionally locates the line inside a larger contiguous
	// capture; it is added to every packet's stream_offset.
	StreamOffset int
}

// unit is one decoded ANC word and its span inside the original line bytes.
type unit struct {
	w     Word
	start int // first byte of the word in the source line
	end   int // one past the last byte of the word
}

type normLine struct {
	line  Line
	enc   Encoding
	units []unit
	raw   []byte // bytes used for raw slices (original bytes when available)
}

func (l Line) encoding(fallback Encoding) Encoding {
	if l.Encoding != "" {
		return l.Encoding
	}
	if len(l.Words) > 0 && len(l.Bytes) == 0 {
		return EncWords
	}
	return fallback
}

// normalize turns any supported line representation into indexed units while
// preserving exact byte offsets into the supplied raw bytes.
func normalize(l Line, fallback Encoding) (*normLine, error) {
	enc := l.encoding(fallback)
	nl := &normLine{line: l, enc: enc}

	switch enc {
	case EncWords:
		nl.raw = make([]byte, 0, 2*len(l.Words))
		nl.units = make([]unit, len(l.Words))
		for i, w := range l.Words {
			w &= MaxWord
			start := len(nl.raw)
			nl.raw = append(nl.raw, byte(w>>8), byte(w)) // canonical u16be raw
			nl.units[i] = unit{w: w, start: start, end: start + 2}
		}
	case EncU16BE, EncU16LE:
		b := l.Bytes
		nl.raw = b
		n := len(b) / 2
		nl.units = make([]unit, n)
		for i := 0; i < n; i++ {
			var w Word
			if enc == EncU16BE {
				w = Word(b[2*i])<<8 | Word(b[2*i+1])
			} else {
				w = Word(b[2*i]) | Word(b[2*i+1])<<8
			}
			nl.units[i] = unit{w: w & MaxWord, start: 2 * i, end: 2*i + 2}
		}
	case EncANC8:
		b := l.Bytes
		nl.raw = b
		nl.units = make([]unit, len(b))
		for i, v := range b {
			nl.units[i] = unit{w: Word(v), start: i, end: i + 1}
		}
	case EncV210:
		units, consumed := decodeV210(l.Bytes)
		nl.raw = l.Bytes
		nl.units = units
		_ = consumed
	default:
		return nil, &EncodingError{Encoding: enc}
	}
	return nl, nil
}

// EncodingError reports an unsupported line encoding.
type EncodingError struct {
	Encoding Encoding
}

func (e *EncodingError) Error() string {
	return "anc: unsupported encoding " + string(e.Encoding)
}

// decodeV210 unpacks 10-bit samples from packed V210 groups: every 4 bytes
// carry three words. The returned byte spans point into the source blob.
func decodeV210(b []byte) ([]unit, int) {
	var units []unit
	off := 0
	for off+4 <= len(b) {
		d := b[off : off+4]
		w0 := Word(d[0]) | Word(d[1]&0x03)<<8
		w1 := Word(d[1]>>2) | Word(d[2]&0x0F)<<6
		w2 := Word(d[2]>>4) | Word(d[3])<<4
		// w0 starts at bit 0 of byte off, w1 at bit 2 of off+1,
		// w2 at bit 4 of off+2. Byte offsets name the containing byte.
		// Spans name every byte the word's bits touch (words cross bytes).
		units = append(units,
			unit{w: w0 & MaxWord, start: off, end: off + 2},
			unit{w: w1 & MaxWord, start: off + 1, end: off + 3},
			unit{w: w2 & MaxWord, start: off + 2, end: off + 4},
		)
		off += 4
	}
	return units, off
}

// encodeV210 is the inverse of decodeV210 for complete triplets; it is mainly
// used by fixtures and tests.
func encodeV210(words []Word) []byte {
	out := make([]byte, 0, 4*((len(words)+2)/3))
	for i := 0; i < len(words); i += 3 {
		var a, b, c Word
		a = words[i] & MaxWord
		if i+1 < len(words) {
			b = words[i+1] & MaxWord
		}
		if i+2 < len(words) {
			c = words[i+2] & MaxWord
		}
		chunk := []byte{
			byte(a),
			byte((a>>8)&0x03) | byte((b&0x3F)<<2),
			byte((b>>6)&0x0F) | byte((c&0x0F)<<4),
			byte((c >> 4) & 0xFF),
		}
		out = append(out, chunk...)
	}
	return out
}

func hexEncode(b []byte) string { return hex.EncodeToString(b) }
