package scc

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Byte is one raw byte from the SCC stream, annotated with the frame and
// raw hex token it came from, plus its parity validity.
type Byte struct {
	Raw       byte   // byte as transmitted, parity bit included
	Value     byte   // Raw with the parity bit stripped
	ParityOK  bool   // odd-parity check result
	Frame     int    // index of the frame line it appeared in
	Timecode  string // timecode of that frame line
	RawToken  string // hex token as written in the file
	CarriedIn bool   // true if this byte dangled from the previous frame
}

// Frame is one timecoded SCC line.
type Frame struct {
	Index    int
	Timecode string
	Bytes    []Byte
	CarryOut *Byte // dangling byte handed to the next frame, if any
}

// ParseError describes a malformed token; parsing continues after it.
type ParseError struct {
	Line  int
	Token string
	Err   error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d: token %q: %v", e.Line, e.Token, e.Err)
}

// Parse reads an SCC file. Hex tokens of 4 digits decode to two bytes;
// a stray 2-digit token leaves a dangling byte that is carried into the
// next frame (cross-frame residue), where it pairs with that frame's
// first byte. Malformed tokens are reported and skipped.
func Parse(r io.Reader) ([]Frame, []ParseError) {
	var frames []Frame
	var errs []ParseError

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)

	var pending *Byte // dangling byte carried across a frame boundary
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "Scenarist_SCC") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			errs = append(errs, ParseError{Line: lineNo, Token: line,
				Err: fmt.Errorf("expected timecode and hex data")})
			continue
		}
		fr := Frame{Index: len(frames), Timecode: fields[0]}
		if pending != nil {
			carried := *pending
			carried.CarriedIn = true
			fr.Bytes = append(fr.Bytes, carried)
			pending = nil
		}
		for _, tok := range fields[1:] {
			tok = strings.TrimSuffix(tok, ",")
			switch len(tok) {
			case 4, 2:
			default:
				errs = append(errs, ParseError{Line: lineNo, Token: tok,
					Err: fmt.Errorf("hex token must be 2 or 4 digits")})
				continue
			}
			for i := 0; i < len(tok); i += 2 {
				v, err := hexByte(tok[i : i+2])
				if err != nil {
					errs = append(errs, ParseError{Line: lineNo, Token: tok, Err: err})
					break // skip the rest of a malformed token
				}
				b := Byte{
					Raw:      v,
					Value:    StripParity(v),
					ParityOK: OddParity(v),
					Frame:    fr.Index,
					Timecode: fr.Timecode,
					RawToken: tok[i : i+2],
				}
				fr.Bytes = append(fr.Bytes, b)
			}
		}
		// Odd byte count: hold the last byte for the next frame.
		if len(fr.Bytes)%2 == 1 {
			last := fr.Bytes[len(fr.Bytes)-1]
			fr.Bytes = fr.Bytes[:len(fr.Bytes)-1]
			fr.CarryOut = &last
			pending = &last
		}
		frames = append(frames, fr)
	}
	if err := sc.Err(); err != nil {
		errs = append(errs, ParseError{Line: lineNo, Err: err})
	}
	if pending != nil {
		errs = append(errs, ParseError{Line: lineNo, Token: pending.RawToken,
			Err: fmt.Errorf("dangling byte 0x%02X at end of stream", pending.Raw)})
	}
	return frames, errs
}

func hexByte(s string) (byte, error) {
	var v byte
	for i := 0; i < 2; i++ {
		c := s[i]
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, fmt.Errorf("invalid hex digit %q", c)
		}
		v = v<<4 | d
	}
	return v, nil
}
