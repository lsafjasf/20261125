package anc

import "io"

// Parser bundles scan options for repeated use. The zero value is usable and
// applies the same defaults as Scan.
type Parser struct {
	Options Options
}

// NewParser returns a Parser configured with opts. A nil argument selects the
// defaults.
func NewParser(opts *Options) *Parser {
	p := &Parser{}
	if opts != nil {
		p.Options = *opts
	}
	return p
}

// Parse scans the supplied video lines.
func (p *Parser) Parse(lines []Line) (*Report, error) {
	return Scan(lines, &p.Options)
}

// ParseReader reads a JSON document/array/JSONL stream and scans it.
func (p *Parser) ParseReader(r io.Reader) (*Report, error) {
	return ParseAndScan(r, &p.Options)
}
