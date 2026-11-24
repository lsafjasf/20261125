// Command dpxgen writes a synthetic messy DPX delivery (headers only)
// for trying out dpxscan: two reels, a duplicate, a gap, a timecode
// rollback, and a resolution change.
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func bcd(h, m, s, f int) uint32 {
	d := func(v int) uint32 { return uint32(v/10)<<4 | uint32(v%10) }
	return d(h)<<24 | d(m)<<16 | d(s)<<8 | d(f)
}

func writeDPX(path string, w, h, pos uint32, fps float32, tc uint32) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf := make([]byte, 4096)
	bo := binary.BigEndian
	copy(buf[0:], "SDPX")
	bo.PutUint32(buf[4:], 4096)
	copy(buf[8:], "V2.0")
	copy(buf[36:], filepath.Base(path))
	bo.PutUint32(buf[772:], w)
	bo.PutUint32(buf[776:], h)
	bo.PutUint32(buf[1772:], pos)
	bo.PutUint32(buf[1776:], 0xFFFFFFFF)
	bo.PutUint32(buf[1784:], math.Float32bits(fps))
	bo.PutUint32(buf[1980:], tc)
	return os.WriteFile(path, buf, 0o644)
}

func main() {
	root := "demo-delivery"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	// Reel_01: TC 01:00:00:00, 6 frames, misleading descending names.
	for i := 0; i < 6; i++ {
		p := filepath.Join(root, "Reel_01", fmt.Sprintf("shot_%04d.dpx", 900-i))
		must(writeDPX(p, 2048, 1556, uint32(i+1), 24, bcd(1, 0, 0, i)))
	}
	// Reel_02: TC restarts at 00:59:00:00, framepos restarts, plus a
	// duplicate (frame 2), a gap (frame 4 missing) and one 1920x1080 frame.
	type fr struct {
		name           string
		w, h, pos      uint32
		hh, mm, ss, ff int
	}
	frames := []fr{
		{"b_0001.dpx", 2048, 1556, 1, 0, 59, 0, 0},
		{"b_0002.dpx", 2048, 1556, 2, 0, 59, 0, 1},
		{"b_0002_copy.dpx", 2048, 1556, 2, 0, 59, 0, 1},
		{"b_0005.dpx", 2048, 1556, 5, 0, 59, 0, 4},
		{"b_0006.dpx", 1920, 1080, 6, 0, 59, 0, 5},
	}
	for _, f := range frames {
		must(writeDPX(filepath.Join(root, "Reel_02", f.name), f.w, f.h, f.pos, 24, bcd(f.hh, f.mm, f.ss, f.ff)))
	}
	fmt.Println("wrote demo delivery to", root)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "dpxgen:", err)
		os.Exit(1)
	}
}
