package fixture

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"mp4index/bmff"
	"mp4index/movie"
)

// CopyFile copies src to dst verbatim.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// TruncateCopy writes the first n bytes of src to dst.
func TruncateCopy(src, dst string, n int64) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.CopyN(out, in, n); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// LocateSTCO returns the absolute stco box of the first video track.
func LocateSTCO(path string) (bmff.Box, error) {
	f, err := os.Open(path)
	if err != nil {
		return bmff.Box{}, err
	}
	defer f.Close()
	st, _ := f.Stat()

	var stco bmff.Box
	found := false
	err = bmff.IterateRange(f, 0, st.Size(), func(top bmff.Box) error {
		if top.Type != "moov" {
			return nil
		}
		return bmff.IterateChildren(f, top, func(trak bmff.Box) error {
			if trak.Type != "trak" || found {
				return nil
			}
			mdia, ok, err := bmff.FindChild(f, trak, "mdia")
			if err != nil || !ok {
				return err
			}
			minf, ok, err := bmff.FindChild(f, mdia, "minf")
			if err != nil || !ok {
				return err
			}
			stbl, ok, err := bmff.FindChild(f, minf, "stbl")
			if err != nil || !ok {
				return err
			}
			box, ok, err := bmff.FindChild(f, stbl, "stco")
			if err != nil || !ok {
				return err
			}
			stco, found = box, true
			return nil
		})
	})
	if err != nil {
		return bmff.Box{}, err
	}
	if !found {
		return bmff.Box{}, fmt.Errorf("stco box not found in %s", path)
	}
	return stco, nil
}

// PatchSTCO rewrites every stco entry in dst using mutate. It returns the
// number of entries patched. The box size does not change.
func PatchSTCO(dst string, mutate func(index int, value uint32) uint32) (int, error) {
	stco, err := LocateSTCO(dst)
	if err != nil {
		return 0, err
	}
	f, err := os.OpenFile(dst, os.O_RDWR, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	payloadStart := stco.PayloadStart()
	var hdr [8]byte
	if _, err := f.ReadAt(hdr[:], payloadStart); err != nil {
		return 0, err
	}
	count := binary.BigEndian.Uint32(hdr[4:8])
	buf := make([]byte, 4*count)
	entriesStart := payloadStart + 8 // fullbox header(4) + count(4)
	if _, err := f.ReadAt(buf, entriesStart); err != nil {
		return 0, err
	}
	for i := uint32(0); i < count; i++ {
		off := 4 * i
		v := binary.BigEndian.Uint32(buf[off : off+4])
		v = mutate(int(i), v)
		binary.BigEndian.PutUint32(buf[off:off+4], v)
	}
	if _, err := f.WriteAt(buf, entriesStart); err != nil {
		return 0, err
	}
	return int(count), nil
}

// OffsetAllSTCO adds delta to every stco entry of a copied src->dst file.
func OffsetAllSTCO(src, dst string, delta int64) (int, error) {
	if err := CopyFile(src, dst); err != nil {
		return 0, err
	}
	return PatchSTCO(dst, func(_ int, v uint32) uint32 {
		return uint32(int64(v) + delta)
	})
}

// DuplicateIndexRecords creates dst from a moov-last src, inserting duplicate
// stsz/stco records immediately after the given 0 based sample positions.
// Media bytes (and their absolute offsets) are untouched.
func DuplicateIndexRecords(src, dst string, after []int) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, info, err := movie.OpenAndParse(src)
	if err != nil {
		return err
	}
	f.Close()
	if info.Video == nil || info.Video.Index == nil {
		return fmt.Errorf("no video index in %s", src)
	}
	idx := info.Video.Index
	if len(idx.Offsets) == 0 || idx.Offsets64 {
		return fmt.Errorf("duplicate fixture requires stco with entries")
	}

	dup := map[int]bool{}
	for _, a := range after {
		dup[a] = true
	}

	sampleN := int(idx.SampleCount)
	if sampleN == 0 {
		sampleN = len(idx.Sizes)
	}

	newSizes := make([]uint32, 0, sampleN+len(after))
	for i := 0; i < sampleN; i++ {
		var size uint32
		if idx.UniformSize != 0 {
			size = idx.UniformSize
		} else if i < len(idx.Sizes) {
			size = idx.Sizes[i]
		}
		newSizes = append(newSizes, size)
		if dup[i] {
			newSizes = append(newSizes, size)
		}
	}

	stszPayload := make([]byte, 8+4*len(newSizes))
	binary.BigEndian.PutUint32(stszPayload[4:8], uint32(len(newSizes)))
	for i, s := range newSizes {
		binary.BigEndian.PutUint32(stszPayload[8+4*i:12+4*i], s)
	}
	newSTSZ := bmff.BuildFullBox("stsz", 0, 0, stszPayload)

	// With one sample per chunk, duplicate the matching stco entries too;
	// otherwise inflated stsz alone is the corrupted sample table.
	offN := len(idx.Offsets)
	stcoCount := offN
	if offN == sampleN {
		stcoCount = sampleN + len(after)
	}
	stcoPayload := make([]byte, 4+4*stcoCount)
	binary.BigEndian.PutUint32(stcoPayload[0:4], uint32(stcoCount))
	w := 0
	for i := 0; i < offN; i++ {
		binary.BigEndian.PutUint32(stcoPayload[4+4*w:8+4*w], uint32(idx.Offsets[i]))
		w++
		if offN == sampleN && dup[i] {
			binary.BigEndian.PutUint32(stcoPayload[4+4*w:8+4*w], uint32(idx.Offsets[i]))
			w++
		}
	}
	newSTCO := bmff.BuildFullBox("stco", 0, 0, stcoPayload[:4+4*w])

	f2, err := os.Open(src)
	if err != nil {
		return err
	}
	st, _ := f2.Stat()
	chain, stszBox, stcoBox, err := findSTBLChain(f2, st.Size())
	f2.Close()
	if err != nil {
		return err
	}

	delta := int64(len(newSTSZ)-int(stszBox.Size)) + int64(len(newSTCO)-int(stcoBox.Size))
	out := replaceRanges(raw,
		byteRange{stszBox.Start, stszBox.End(), newSTSZ},
		byteRange{stcoBox.Start, stcoBox.End(), newSTCO},
	)
	// Ancestor headers sit before the replaced ranges, so their absolute
	// offsets stay valid; grow each declared box size by the total delta.
	for _, anc := range chain {
		newSize := uint32(anc.Size + delta)
		binary.BigEndian.PutUint32(out[anc.Start:anc.Start+4], newSize)
	}
	return os.WriteFile(dst, out, 0o644)
}

// findSTBLChain returns ancestor boxes [moov,trak,mdia,minf,stbl] and the
// stsz/stco boxes of the first video sample table.
func findSTBLChain(ra io.ReaderAt, size int64) ([]bmff.Box, bmff.Box, bmff.Box, error) {
	var chain []bmff.Box
	var stsz, stco bmff.Box
	found := false

	findInSTBL := func(stbl bmff.Box) error {
		return bmff.IterateChildren(ra, stbl, func(c bmff.Box) error {
			switch c.Type {
			case "stsz":
				stsz = c
			case "stco":
				stco = c
			}
			return nil
		})
	}

	_ = bmff.IterateRange(ra, 0, size, func(moov bmff.Box) error {
		if moov.Type != "moov" || found {
			return nil
		}
		return bmff.IterateChildren(ra, moov, func(trak bmff.Box) error {
			if trak.Type != "trak" || found {
				return nil
			}
			mdia, ok, err := bmff.FindChild(ra, trak, "mdia")
			if err != nil || !ok {
				return err
			}
			minf, ok, err := bmff.FindChild(ra, mdia, "minf")
			if err != nil || !ok {
				return err
			}
			stbl, ok, err := bmff.FindChild(ra, minf, "stbl")
			if err != nil || !ok {
				return err
			}
			if err := findInSTBL(stbl); err != nil {
				return err
			}
			chain = []bmff.Box{moov, trak, mdia, minf, stbl}
			found = stsz.Type != "" && stco.Type != ""
			return nil
		})
	})
	if !found {
		return nil, stsz, stco, fmt.Errorf("stbl/stsz/stco chain not found")
	}
	return chain, stsz, stco, nil
}

type byteRange struct {
	start int64
	end   int64
	data  []byte
}

// replaceRanges applies non-overlapping replacements from right to left.
func replaceRanges(raw []byte, ranges ...byteRange) []byte {
	sorted := append([]byteRange(nil), ranges...)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].start > sorted[i].start {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	out := append([]byte(nil), raw...)
	for _, r := range sorted {
		head := append([]byte(nil), out[:r.start]...)
		tail := append([]byte(nil), out[r.end:]...)
		out = append(head, append(r.data, tail...)...)
	}
	return out
}
