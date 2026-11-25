package icc

import "os"

const (
	headerSize   = 128
	tagEntrySize = 12
)

// ParseFile reads and parses the supported matrix/TRC subset of an ICC file.
// A non-nil profile is always returned; structural problems are collected in
// Profile.Issues and Tag.Issues instead of aborting the whole analysis.
func ParseFile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, path), nil
}

// Parse decodes an ICC profile from its raw bytes.
func Parse(data []byte, path string) *Profile {
	r := reader{data: data}
	p := &Profile{Path: path, Size: len(data)}

	if len(data) < headerSize {
		p.addIssue("file shorter than 128-byte ICC header: %d bytes", len(data))
		return p
	}
	p.parseHeader(r)
	p.parseTagTable(r)
	for _, tag := range p.Tags {
		if tag.Kind != TagUnsupported {
			p.parseTagData(r, tag)
		}
	}
	return p
}

func (p *Profile) parseHeader(r reader) {
	declared, _ := r.u32(0)
	if int(declared) != p.Size {
		p.addIssue("header profile size %d does not match file length %d", declared, p.Size)
	}

	ver, _ := r.u32(8)
	p.Version = Version{
		Major:  int(byte(ver >> 24)),
		Minor:  int(byte(ver>>20) & 0x0F),
		Bugfix: int(byte(ver>>16) & 0x0F),
		Raw:    ver,
	}

	p.Class, _ = r.sig(12)
	p.ColorSpace, _ = r.sig(16)
	p.PCS, _ = r.sig(20)

	if sig, ok := r.sig(36); !ok || sig != "acsp" {
		p.addIssue("missing 'acsp' device profile signature at offset 36")
	}

	x, _ := r.u32(68)
	y, _ := r.u32(72)
	z, _ := r.u32(76)
	p.HeaderWP = XYZ{fromS15Fixed16(x), fromS15Fixed16(y), fromS15Fixed16(z)}
}

func (p *Profile) parseTagTable(r reader) {
	count, ok := r.u32(headerSize)
	if !ok {
		p.addIssue("tag table count not readable")
		return
	}
	tableEnd := headerSize + 4 + int(count)*tagEntrySize
	if tableEnd > r.len() {
		p.addIssue("tag table declares %d entries but only %d bytes remain", count, r.len()-headerSize-4)
		return
	}

	seen := map[string]int{}
	type interval struct {
		start, end int
		sig        string
	}
	var ranges []interval

	for i := 0; i < int(count); i++ {
		base := headerSize + 4 + i*tagEntrySize
		sig, _ := r.sig(base)
		off, _ := r.u32(base + 4)
		size, _ := r.u32(base + 8)
		tag := &Tag{
			Signature: sig,
			Offset:    int(off),
			Size:      int(size),
			Kind:      classifyTag(sig),
		}

		if prev, dup := seen[sig]; dup {
			tag.Issues = append(tag.Issues,
				"duplicate tag signature (first seen at entry "+itoa(prev)+")")
			p.addIssue("duplicate tag %q: entry %d duplicates entry %d", sig, i, prev)
		}
		seen[sig] = i

		if tag.Offset < headerSize {
			tag.Issues = append(tag.Issues, "tag data offset inside header/tag table")
			p.addIssue("tag %q: offset %d is before end of tag table", sig, tag.Offset)
		}
		if tag.Size <= 0 {
			tag.Issues = append(tag.Issues, "non-positive tag size")
			p.addIssue("tag %q: illegal size %d", sig, tag.Size)
		}
		if tag.Offset+tag.Size > r.len() {
			tag.Issues = append(tag.Issues, "tag data extends past end of file")
			p.addIssue("tag %q: offset %d + size %d exceeds file length %d",
				sig, tag.Offset, tag.Size, r.len())
		}

		cur := interval{tag.Offset, tag.Offset + tag.Size, sig}
		for _, prev := range ranges {
			// Duplicate tags legitimately share one payload range.
			if cur.sig == prev.sig {
				continue
			}
			if cur.start < prev.end && prev.start < cur.end && tag.Offset >= headerSize {
				tag.Issues = append(tag.Issues, "tag data range overlaps another tag")
				p.addIssue("tag %q: data range [%d,%d) overlaps another tag", sig, cur.start, cur.end)
				break
			}
		}
		ranges = append(ranges, cur)
		p.Tags = append(p.Tags, tag)
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	n := len(b)
	for v > 0 {
		n--
		b[n] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}
