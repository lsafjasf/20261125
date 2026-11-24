package main

import (
	"encoding/binary"
	"fmt"
	"io"
)

const maxMP4Depth = 32

type boxHeader struct {
	typ        string
	start      int64
	headerSize int64
	headerEnd  int64
	end        int64
}

type boxNode struct {
	BoxHash
	header   boxHeader
	Children []*boxNode
}

var containerBoxTypes = map[string]bool{
	"moov": true, "trak": true, "mdia": true, "minf": true,
	"stbl": true, "edts": true, "dinf": true, "udta": true,
	"meta": true, "ilst": true, "moof": true, "traf": true,
	"mvex": true, "sinf": true, "schi": true,
}

var ignorableBoxTypes = map[string]bool{
	"free": true, "skip": true, "wide": true, "uuid": true,
	"udta": true, "meta": true, "ilst": true,
}

func readFullAt(r io.ReaderAt, p []byte, off int64) error {
	n, err := r.ReadAt(p, off)
	if err != nil {
		if err == io.EOF && n == len(p) {
			return nil
		}
		return err
	}
	return nil
}

func readBoxHeader(r io.ReaderAt, start, bound int64) (boxHeader, error) {
	var first [8]byte
	if err := readFullAt(r, first[:], start); err != nil {
		return boxHeader{}, fmt.Errorf("read box header at %d: %w", start, err)
	}

	size32 := binary.BigEndian.Uint32(first[0:4])
	h := boxHeader{
		typ:        string(first[4:8]),
		start:      start,
		headerSize: 8,
	}

	switch {
	case size32 == 1:
		var large [8]byte
		if err := readFullAt(r, large[:], start+8); err != nil {
			return boxHeader{}, fmt.Errorf("read largesize for %q: %w", h.typ, err)
		}
		h.end = start + int64(binary.BigEndian.Uint64(large[:]))
		h.headerSize = 16
	case size32 == 0:
		h.end = bound
	default:
		h.end = start + int64(size32)
	}
	h.headerEnd = start + h.headerSize

	if h.end < h.headerEnd || h.end > bound {
		return boxHeader{}, fmt.Errorf("box %q at %d has invalid end %d (bound %d)", h.typ, start, h.end, bound)
	}
	return h, nil
}

func buildBoxTree(r io.ReaderAt, size int64) ([]*boxNode, []BoxHash, error) {
	var flat []BoxHash
	roots, err := scanBoxRange(r, 0, size, "", 0, &flat)
	if err != nil {
		return nil, nil, err
	}
	return roots, flat, nil
}

func scanBoxRange(r io.ReaderAt, start, end int64, parentPath string, depth int, flat *[]BoxHash) ([]*boxNode, error) {
	if depth > maxMP4Depth {
		return nil, fmt.Errorf("box nesting exceeds %d", maxMP4Depth)
	}

	var nodes []*boxNode
	pos := start
	counts := map[string]int{}

	for pos < end {
		h, err := readBoxHeader(r, pos, end)
		if err != nil {
			return nil, err
		}

		counts[h.typ]++
		path := fmt.Sprintf("%s/%s[%d]", parentPath, h.typ, counts[h.typ])
		hash, err := hashRange(r, h.start, h.end)
		if err != nil {
			return nil, fmt.Errorf("hash box %s: %w", path, err)
		}

		info := BoxHash{
			Path:      path,
			Type:      h.typ,
			Index:     counts[h.typ],
			Start:     h.start,
			HeaderEnd: h.headerEnd,
			End:       h.end,
			Size:      h.end - h.start,
			SHA256:    hash,
			Ignorable: ignorableBoxTypes[h.typ],
		}
		node := &boxNode{BoxHash: info, header: h}
		*flat = append(*flat, info)

		childStart := h.headerEnd
		childBound := h.end
		if h.typ == "meta" && h.end-h.headerEnd >= 4 {
			childStart += 4 // meta is a FullBox before its child boxes.
		}
		if containerBoxTypes[h.typ] && childStart < childBound {
			children, err := scanBoxRange(r, childStart, childBound, path, depth+1, flat)
			if err != nil {
				return nil, fmt.Errorf("box %s: %w", path, err)
			}
			node.Children = children
		}

		nodes = append(nodes, node)
		pos = h.end
	}
	return nodes, nil
}

func (n *boxNode) child(typ string) *boxNode {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Type == typ {
			return c
		}
	}
	return nil
}

func (n *boxNode) all(typ string) []*boxNode {
	if n == nil {
		return nil
	}
	var out []*boxNode
	var walk func(*boxNode)
	walk = func(node *boxNode) {
		if node.Type == typ {
			out = append(out, node)
		}
		for _, c := range node.Children {
			walk(c)
		}
	}
	for _, c := range n.Children {
		walk(c)
	}
	return out
}

func findAllBoxes(roots []*boxNode, typ string) []*boxNode {
	var out []*boxNode
	for _, root := range roots {
		out = append(out, root.all(typ)...)
	}
	return out
}
