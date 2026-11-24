package anc

import "fmt"

// MaxPayload is the maximum DC-representable number of UDW.
const MaxPayload = 255

// BuildWords returns a complete, valid 10-bit ANC packet (ADF, parity
// protected DID/SDID/DC, payload and checksum word).
func BuildWords(did, sdidOrDBN byte, payload []byte) ([]Word, error) {
	if len(payload) > MaxPayload {
		return nil, fmt.Errorf("anc: payload length %d exceeds DC capacity %d", len(payload), MaxPayload)
	}
	dc := byte(len(payload))
	words := []Word{ADFZero, ADFOne, ADFOne, WithParity(did), WithParity(sdidOrDBN), WithParity(dc)}
	for _, b := range payload {
		words = append(words, Word(b))
	}
	cs := Checksum9(words[3:]...)
	words = append(words, MakeChecksumWord(cs))
	return words, nil
}

// BuildANC8 returns a complete 8-bit ANC byte packet
// 00 FF FF DID SDID DC UDW... CS8.
func BuildANC8(did, sdidOrDBN byte, payload []byte) ([]byte, error) {
	if len(payload) > MaxPayload {
		return nil, fmt.Errorf("anc: payload length %d exceeds DC capacity %d", len(payload), MaxPayload)
	}
	out := []byte{0x00, 0xFF, 0xFF, did, sdidOrDBN, byte(len(payload))}
	out = append(out, payload...)
	out = append(out, Checksum8(did, sdidOrDBN, byte(len(payload)), payload))
	return out, nil
}

// SerializeWords encodes words into the requested byte container
// (EncU16BE/EncU16LE); EncANC8 strips parity/upper bits and emits the 8-bit
// form including an 8-bit checksum.
func SerializeWords(words []Word, enc Encoding) ([]byte, error) {
	switch enc {
	case EncU16BE:
		out := make([]byte, 2*len(words))
		for i, w := range words {
			out[2*i] = byte(w >> 8)
			out[2*i+1] = byte(w)
		}
		return out, nil
	case EncU16LE:
		out := make([]byte, 2*len(words))
		for i, w := range words {
			out[2*i] = byte(w)
			out[2*i+1] = byte(w >> 8)
		}
		return out, nil
	case EncANC8:
		if len(words) < 7 {
			return nil, fmt.Errorf("anc: short word packet")
		}
		did := words[3].Data()
		sdid := words[4].Data()
		dc := int(words[5].Data())
		if len(words) < 6+dc+1 {
			return nil, fmt.Errorf("anc: word packet does not contain declared payload/checksum")
		}
		payload := make([]byte, dc)
		for i := 0; i < dc; i++ {
			payload[i] = words[6+i].Data()
		}
		return BuildANC8(did, sdid, payload)
	default:
		return nil, &EncodingError{Encoding: enc}
	}
}
