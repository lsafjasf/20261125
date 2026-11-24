package anc

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Document is the JSON input shape: an optional standard/encoding header and
// the captured video lines.
type Document struct {
	Schema            string           `json:"schema"`
	Standard          string           `json:"standard"`
	Encoding          Encoding         `json:"encoding"`
	ContinuationLines int              `json:"continuation_lines"`
	ExpectedLines     map[string][]int `json:"expected_lines,omitempty"`
	Lines             []LineDTO        `json:"lines"`
}

// LineDTO is one JSON line record. It accepts several friendly aliases.
type LineDTO struct {
	Field        int    `json:"field"`
	Line         int    `json:"line"`
	Number       int    `json:"number"`
	Encoding     string `json:"encoding"`
	Words        []int  `json:"words"`
	DataHex      string `json:"data_hex"`
	Hex          string `json:"hex"`
	DataBase64   string `json:"data_base64"`
	Base64       string `json:"base64"`
	StreamOffset int    `json:"stream_offset"`
}

// ToLine converts the DTO into a Line.
func (d LineDTO) ToLine(defaultEnc Encoding) (Line, error) {
	l := Line{
		Field:        d.Field,
		Number:       d.Line,
		Encoding:     Encoding(d.Encoding),
		StreamOffset: d.StreamOffset,
	}
	if l.Number == 0 {
		l.Number = d.Number
	}
	if len(d.Words) > 0 {
		l.Encoding = EncWords
		l.Words = make([]Word, len(d.Words))
		for i, w := range d.Words {
			if w < 0 || int(MaxWord) < w {
				return l, fmt.Errorf("anc: word %d out of 10-bit range on field=%d line=%d", w, d.Field, l.Number)
			}
			l.Words[i] = Word(w)
		}
		return l, nil
	}
	if h := firstNonEmpty(d.DataHex, d.Hex); h != "" {
		b, err := DecodeFlexHex(h)
		if err != nil {
			return l, fmt.Errorf("anc: invalid hex on field=%d line=%d: %w", d.Field, l.Number, err)
		}
		l.Bytes = b
		if l.Encoding == "" {
			l.Encoding = defaultEnc
		}
		return l, nil
	}
	if b64 := firstNonEmpty(d.DataBase64, d.Base64); b64 != "" {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
		if err != nil {
			return l, fmt.Errorf("anc: invalid base64 on field=%d line=%d: %w", d.Field, l.Number, err)
		}
		l.Bytes = b
		if l.Encoding == "" {
			l.Encoding = defaultEnc
		}
		return l, nil
	}
	if l.Encoding == "" {
		l.Encoding = defaultEnc
	}
	return l, nil
}

// ParseAndScan reads a JSON document, a JSON array, or JSONL line records and
// scans them. Caller options override document header values where set.
func ParseAndScan(r io.Reader, caller *Options) (*Report, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("anc: empty input")
	}

	var doc Document
	switch trimmed[0] {
	case '{':
		var head struct {
			Lines json.RawMessage `json:"lines"`
		}
		_ = json.Unmarshal(data, &head)
		if len(head.Lines) > 0 {
			if err := json.Unmarshal(data, &doc); err != nil {
				return nil, fmt.Errorf("anc: invalid JSON document: %w", err)
			}
			break
		}
		// A single line object or newline-delimited line records.
		dtos, err := parseJSONL(data)
		if err != nil {
			return nil, err
		}
		doc.Lines = dtos
	case '[':
		var dtos []LineDTO
		if err := json.Unmarshal(data, &dtos); err != nil {
			return nil, fmt.Errorf("anc: invalid JSON line array: %w", err)
		}
		doc.Lines = dtos
	default:
		dtos, err := parseJSONL(data)
		if err != nil {
			return nil, err
		}
		doc.Lines = dtos
	}

	lines := make([]Line, 0, len(doc.Lines))
	for _, dto := range doc.Lines {
		l, err := dto.ToLine(firstNonEmptyEnc(callerEncoding(caller), doc.Encoding, EncU16BE))
		if err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}

	cfg := Options{}
	if caller != nil {
		cfg = *caller
	}
	if cfg.Encoding == "" {
		cfg.Encoding = doc.Encoding
	}
	if cfg.ContinuationLines == 0 {
		cfg.ContinuationLines = doc.ContinuationLines
	}
	if cfg.ExpectedLines == nil && len(doc.ExpectedLines) > 0 {
		expected, err := parseExpectedLines(doc.ExpectedLines)
		if err != nil {
			return nil, err
		}
		cfg.ExpectedLines = expected
	}
	rep, err := Scan(lines, &cfg)
	if err != nil {
		return nil, err
	}
	rep.Standard = doc.Standard
	return rep, nil
}

// parseExpectedLines converts JSON keys such as "0x61" or "61" into a
// DID -> expected line numbers map.
func parseExpectedLines(in map[string][]int) (map[byte][]int, error) {
	out := make(map[byte][]int, len(in))
	for key, lines := range in {
		b, err := DecodeFlexHex(key)
		if err != nil || len(b) != 1 {
			return nil, fmt.Errorf("anc: expected_lines key %q must be one hex byte", key)
		}
		out[b[0]] = lines
	}
	return out, nil
}

func parseJSONL(data []byte) ([]LineDTO, error) {
	var out []LineDTO
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		trim := strings.TrimSpace(sc.Text())
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		var dto LineDTO
		if err := json.Unmarshal([]byte(trim), &dto); err != nil {
			return nil, fmt.Errorf("anc: invalid JSONL record on line %d: %w", lineNo, err)
		}
		out = append(out, dto)
	}
	return out, sc.Err()
}

// ParseHexLines reads the lightweight text format: one video line per text
// line, optional "F<field>-L<line>:" location prefix, then hex bytes.
// Text lines starting with '#' are comments.
func ParseHexLines(r io.Reader, enc Encoding) ([]Line, error) {
	if enc == "" {
		enc = EncANC8
	}
	var lines []Line
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	textLine := 0
	for sc.Scan() {
		textLine++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		field, lineNo, rest := 0, textLine, raw
		if strings.HasPrefix(strings.ToUpper(raw), "F") {
			if f, l, tail, ok := parseLocationPrefix(raw); ok {
				field, lineNo, rest = f, l, tail
			}
		}
		b, err := DecodeFlexHex(rest)
		if err != nil {
			return nil, fmt.Errorf("anc: hex-lines line %d: %w", textLine, err)
		}
		lines = append(lines, Line{Field: field, Number: lineNo, Bytes: b, Encoding: enc})
	}
	return lines, sc.Err()
}

// parseLocationPrefix parses "F2-L276: ..." (case insensitive).
func parseLocationPrefix(s string) (field, line int, rest string, ok bool) {
	upper := strings.ToUpper(s)
	dash := strings.Index(upper, "-L")
	colon := strings.Index(upper, ":")
	if dash < 2 || colon < 0 || colon < dash {
		return 0, 0, s, false
	}
	f, err1 := strconv.Atoi(s[1:dash])
	l, err2 := strconv.Atoi(s[dash+2 : colon])
	if err1 != nil || err2 != nil {
		return 0, 0, s, false
	}
	return f, l, strings.TrimSpace(s[colon+1:]), true
}

// DecodeFlexHex decodes hex that may contain spaces, tabs, colons, commas or
// 0x prefixes between bytes.
func DecodeFlexHex(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, "0x", "")
	s = strings.ReplaceAll(s, "0X", "")
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			b.WriteByte(c)
		default:
			// separator: ignore
		}
	}
	h := b.String()
	if len(h)%2 != 0 {
		return nil, fmt.Errorf("odd-length hex data")
	}
	out := make([]byte, len(h)/2)
	for i := 0; i < len(out); i++ {
		v, err := strconv.ParseUint(h[2*i:2*i+2], 16, 8)
		if err != nil {
			return nil, err
		}
		out[i] = byte(v)
	}
	return out, nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func callerEncoding(o *Options) Encoding {
	if o == nil {
		return ""
	}
	return o.Encoding
}

func firstNonEmptyEnc(vals ...Encoding) Encoding {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return EncU16BE
}
