package lut3d

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	maxLUTSize       = 256
	maxLineBytes     = 1 << 20
	defaultLUTDomain = 1
)

type domainMode uint8

const (
	domainDefault domainMode = iota
	domainUnit
	domainExplicit
	domainInputRange
)

type parser struct {
	scanner *bufio.Scanner
	line    int

	title     string
	titleLine int

	size     int
	sizeLine int

	mode     domainMode
	modeLine int
	minSeen  bool
	maxSeen  bool
	minLine  int
	maxLine  int
	domain   [3]AxisDomain

	dataStarted bool
	entries     []RGB
}

// Parse reads one complete CUBE 3D LUT. It never returns a partial LUT.
func Parse(r io.Reader) (*LUT, error) {
	p := &parser{
		scanner: bufio.NewScanner(r),
		domain: [3]AxisDomain{
			{Min: 0, Max: defaultLUTDomain},
			{Min: 0, Max: defaultLUTDomain},
			{Min: 0, Max: defaultLUTDomain},
		},
	}
	p.scanner.Buffer(make([]byte, 4096), maxLineBytes)
	return p.parse()
}

// ParseBytes is Parse over an in-memory input.
func ParseBytes(data []byte) (*LUT, error) {
	return Parse(bytes.NewReader(data))
}

// ParseFile opens path and applies the same atomic strict parser as Parse.
func ParseFile(path string) (*LUT, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()
	return Parse(f)
}

func (p *parser) parse() (*LUT, error) {
	for p.scanner.Scan() {
		p.line++
		raw := p.scanner.Text()
		if p.line == 1 {
			raw = strings.TrimPrefix(raw, "\ufeff")
		}
		if !utf8.ValidString(raw) {
			return nil, p.fail(ErrSyntax, "input is not valid UTF-8")
		}
		if strings.IndexByte(raw, 0) >= 0 {
			return nil, p.fail(ErrSyntax, "NUL byte is not allowed")
		}
		line := stripComment(raw)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if isNumber(fields[0]) {
			if err := p.addEntry(fields); err != nil {
				return nil, err
			}
			continue
		}
		if p.dataStarted {
			return nil, p.fail(ErrSyntax, "directive %q cannot appear after table data begins", fields[0])
		}
		if err := p.parseDirective(line, fields); err != nil {
			return nil, err
		}
	}
	if err := p.scanner.Err(); err != nil {
		return nil, &ParseError{
			Line:    p.line + 1,
			Kind:    ErrSyntax,
			Message: "cannot read input",
			Err:     err,
		}
	}
	return p.finish()
}

func isNumber(token string) bool {
	return floatTokenRE.MatchString(token)
}

func (p *parser) parseDirective(line string, fields []string) error {
	switch fields[0] {
	case "TITLE":
		return p.parseTitle(line)
	case "UNIT":
		return p.parseUnit(fields)
	case "DOMAIN_MIN":
		return p.parseDomainBound(fields, true)
	case "DOMAIN_MAX":
		return p.parseDomainBound(fields, false)
	case "LUT_3D_INPUT_RANGE":
		return p.parseInputRange(fields)
	case "LUT_3D_SIZE":
		return p.parseSize(fields)
	case "LUT_1D_SIZE":
		return p.fail(ErrUnsupported, "1D LUTs are not supported; expected LUT_3D_SIZE")
	default:
		return p.fail(ErrSyntax, "unknown directive %q", fields[0])
	}
}

func (p *parser) parseTitle(line string) error {
	payload := strings.TrimSpace(line[len("TITLE"):])
	if payload == "" {
		return p.fail(ErrSyntax, "TITLE requires a value")
	}
	if p.titleLine != 0 {
		return p.fail(ErrHeader, "duplicate TITLE; first declared on line %d", p.titleLine)
	}
	title, err := parseTitleValue(payload)
	if err != nil {
		return &ParseError{Line: p.line, Kind: ErrSyntax, Message: "invalid TITLE", Err: err}
	}
	p.title = title
	p.titleLine = p.line
	return nil
}

func parseTitleValue(payload string) (string, error) {
	if payload[0] != '"' {
		return payload, nil
	}
	var out strings.Builder
	for i := 1; i < len(payload); {
		switch payload[i] {
		case '\\':
			if i+1 >= len(payload) {
				return "", fmt.Errorf("unterminated escape")
			}
			next := payload[i+1]
			if next == '"' || next == '\\' {
				out.WriteByte(next)
			} else {
				out.WriteByte('\\')
				out.WriteByte(next)
			}
			i += 2
		case '"':
			if strings.TrimSpace(payload[i+1:]) != "" {
				return "", fmt.Errorf("unexpected characters after closing quote")
			}
			return out.String(), nil
		default:
			out.WriteByte(payload[i])
			i++
		}
	}
	return "", fmt.Errorf("unterminated quoted value")
}

func (p *parser) parseUnit(fields []string) error {
	if len(fields) != 1 {
		return p.fail(ErrSyntax, "UNIT takes no arguments")
	}
	if p.mode == domainUnit {
		return p.fail(ErrHeader, "duplicate UNIT")
	}
	if p.mode != domainDefault {
		return p.fail(ErrHeader, "UNIT cannot be combined with another domain declaration")
	}
	p.mode = domainUnit
	p.modeLine = p.line
	return nil
}

func (p *parser) parseDomainBound(fields []string, min bool) error {
	name := "DOMAIN_MAX"
	if min {
		name = "DOMAIN_MIN"
	}
	if len(fields) != 4 {
		return p.fail(ErrSyntax, "%s requires exactly 3 values", name)
	}
	if p.mode != domainDefault && p.mode != domainExplicit {
		return p.fail(ErrHeader, "%s cannot be combined with %s", name, p.modeName())
	}
	if min && p.minSeen {
		return p.fail(ErrHeader, "duplicate DOMAIN_MIN; first declared on line %d", p.minLine)
	}
	if !min && p.maxSeen {
		return p.fail(ErrHeader, "duplicate DOMAIN_MAX; first declared on line %d", p.maxLine)
	}
	values, err := p.threeValues(fields[1:])
	if err != nil {
		return err
	}
	p.mode = domainExplicit
	for i := range p.domain {
		if min {
			p.domain[i].Min = values[i]
		} else {
			p.domain[i].Max = values[i]
		}
	}
	if min {
		p.minSeen = true
		p.minLine = p.line
	} else {
		p.maxSeen = true
		p.maxLine = p.line
	}
	return nil
}

func (p *parser) parseInputRange(fields []string) error {
	if len(fields) != 3 {
		return p.fail(ErrSyntax, "LUT_3D_INPUT_RANGE requires exactly 2 values")
	}
	if p.mode != domainDefault {
		return p.fail(ErrHeader, "LUT_3D_INPUT_RANGE cannot be combined with %s", p.modeName())
	}
	lo, err := p.floatAt(fields[1])
	if err != nil {
		return err
	}
	hi, err := p.floatAt(fields[2])
	if err != nil {
		return err
	}
	if !(lo < hi) {
		return p.fail(ErrDomain, "LUT_3D_INPUT_RANGE minimum %g must be less than maximum %g", lo, hi)
	}
	if math.IsInf(hi-lo, 0) {
		return p.fail(ErrDomain, "LUT_3D_INPUT_RANGE span is not finite")
	}
	p.mode = domainInputRange
	p.modeLine = p.line
	for i := range p.domain {
		p.domain[i] = AxisDomain{Min: lo, Max: hi}
	}
	return nil
}

func (p *parser) parseSize(fields []string) error {
	if len(fields) != 2 {
		return p.fail(ErrSyntax, "LUT_3D_SIZE requires exactly one integer")
	}
	if p.sizeLine != 0 {
		return p.fail(ErrHeader, "duplicate LUT_3D_SIZE; first declared on line %d", p.sizeLine)
	}
	n, err := parseIntToken(fields[1])
	if err != nil {
		return &ParseError{Line: p.line, Kind: ErrSyntax, Message: "invalid LUT_3D_SIZE", Err: err}
	}
	if n < 2 {
		return p.fail(ErrHeader, "LUT_3D_SIZE must be at least 2")
	}
	if n > maxLUTSize {
		return p.fail(ErrHeader, "LUT_3D_SIZE %d exceeds safety limit %d", n, maxLUTSize)
	}
	p.size = n
	p.sizeLine = p.line
	p.entries = make([]RGB, 0, n*n*n)
	return nil
}

func (p *parser) addEntry(fields []string) error {
	p.dataStarted = true
	if p.size == 0 {
		return p.fail(ErrHeader, "table data appears before LUT_3D_SIZE")
	}
	expected := p.size * p.size * p.size
	if len(p.entries) == expected {
		return p.fail(ErrEntryCount, "table is already complete with %d entries", expected)
	}
	if len(fields) != 3 {
		return p.fail(ErrSyntax, "table row must contain exactly 3 values; got %d", len(fields))
	}
	values, err := p.threeValues(fields)
	if err != nil {
		return err
	}
	p.entries = append(p.entries, RGB{R: values[0], G: values[1], B: values[2]})
	return nil
}

func (p *parser) threeValues(tokens []string) ([3]float64, error) {
	var values [3]float64
	for i, token := range tokens {
		v, err := p.floatAt(token)
		if err != nil {
			return values, err
		}
		values[i] = v
	}
	return values, nil
}

func (p *parser) floatAt(token string) (float64, error) {
	v, err := parseFloatToken(token)
	if err != nil {
		return 0, &ParseError{Line: p.line, Kind: ErrSyntax, Message: "invalid number", Err: err}
	}
	return v, nil
}

func (p *parser) finish() (*LUT, error) {
	if p.size == 0 {
		return nil, &ParseError{Line: p.line + 1, Kind: ErrHeader, Message: "missing LUT_3D_SIZE"}
	}
	if p.mode == domainExplicit {
		switch {
		case !p.minSeen:
			return nil, &ParseError{
				Line:    p.maxLine,
				Kind:    ErrDomain,
				Message: "DOMAIN_MAX requires a matching DOMAIN_MIN",
			}
		case !p.maxSeen:
			return nil, &ParseError{
				Line:    p.minLine,
				Kind:    ErrDomain,
				Message: "DOMAIN_MIN requires a matching DOMAIN_MAX",
			}
		}
		for i, d := range p.domain {
			if !(d.Min < d.Max) {
				return nil, &ParseError{
					Line:    p.domainLine(),
					Kind:    ErrDomain,
					Message: fmt.Sprintf("axis %s domain minimum %g must be less than maximum %g", axisName(i), d.Min, d.Max),
				}
			}
			if math.IsInf(d.Max-d.Min, 0) {
				return nil, &ParseError{
					Line:    p.domainLine(),
					Kind:    ErrDomain,
					Message: fmt.Sprintf("axis %s domain span is not finite", axisName(i)),
				}
			}
		}
	}
	expected := p.size * p.size * p.size
	if len(p.entries) != expected {
		return nil, &ParseError{
			Line: p.line + 1,
			Kind: ErrEntryCount,
			Message: fmt.Sprintf(
				"LUT_3D_SIZE %d requires %d entries, got %d",
				p.size, expected, len(p.entries),
			),
		}
	}
	var steps [3]float64
	for i, d := range p.domain {
		steps[i] = d.Step(p.size)
		if !(steps[i] > 0) || math.IsInf(steps[i], 0) {
			return nil, &ParseError{
				Line:    p.domainLine(),
				Kind:    ErrDomain,
				Message: fmt.Sprintf("axis %s has invalid grid step %g", axisName(i), steps[i]),
			}
		}
	}
	return &LUT{
		Title:   p.title,
		Size:    p.size,
		Domain:  p.domain,
		Step:    steps,
		Entries: p.entries,
	}, nil
}

func axisName(i int) string {
	return [3]string{"R", "G", "B"}[i]
}

func (p *parser) domainLine() int {
	if p.mode == domainExplicit {
		if p.maxLine > p.minLine {
			return p.maxLine
		}
		return p.minLine
	}
	return p.modeLine
}

func (p *parser) modeName() string {
	switch p.mode {
	case domainUnit:
		return "UNIT"
	case domainExplicit:
		return "DOMAIN_MIN/DOMAIN_MAX"
	case domainInputRange:
		return "LUT_3D_INPUT_RANGE"
	default:
		return "another domain declaration"
	}
}

func (p *parser) fail(kind ErrorKind, format string, args ...any) *ParseError {
	return &ParseError{
		Line:    p.line,
		Kind:    kind,
		Message: fmt.Sprintf(format, args...),
	}
}
