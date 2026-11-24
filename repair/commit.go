package repair

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"mp4index/bmff"
	"mp4index/movie"
)

// ErrSourceDestinationSame is returned when output would alias the source.
var ErrSourceDestinationSame = errors.New("repair: output path must be a separate file from input")

// RepairFile analyzes srcPath and writes the repaired movie to dstPath using a
// temp file plus atomic rename. The source is opened read-only and is never
// replaced; any failure removes the temp file and leaves both source and any
// existing destination untouched.
func RepairFile(srcPath, dstPath string, opts Options) (*Report, error) {
	if dstPath == "" {
		return nil, errors.New("repair: output path required")
	}
	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		return nil, err
	}
	absDst, err := filepath.Abs(dstPath)
	if err != nil {
		return nil, err
	}
	if absSrc == absDst {
		return nil, ErrSourceDestinationSame
	}

	srcStat, err := os.Stat(absSrc)
	if err != nil {
		return nil, fmt.Errorf("stat source: %w", err)
	}
	if dstStat, err := os.Stat(absDst); err == nil {
		if os.SameFile(srcStat, dstStat) {
			return nil, ErrSourceDestinationSame
		}
	}

	plan, err := Analyze(absSrc, opts)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(absDst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".mp4repair-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create temp output in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}

	if err := plan.writeOutput(tmp, absSrc); err != nil {
		cleanup()
		return nil, fmt.Errorf("write repaired output: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return nil, fmt.Errorf("sync temp output: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return nil, err
	}

	if err := verifyOutput(tmpName, plan.IndexedSampleCount()); err != nil {
		os.Remove(tmpName)
		return nil, fmt.Errorf("output verification failed, original untouched: %w", err)
	}

	if err := os.Rename(tmpName, absDst); err != nil {
		os.Remove(tmpName)
		return nil, fmt.Errorf("commit output (rename): %w", err)
	}

	plan.Report.WroteOutput = true
	plan.Report.Output = absDst
	return plan.Report, nil
}

// IndexedSampleCount returns kept sample count.
func (p *Plan) IndexedSampleCount() int { return countKept(p.Samples) }

func (p *Plan) mdatHeaderSize() int {
	if p.PayloadBytes+8 > math.MaxUint32 {
		return 16
	}
	return 8
}

func (p *Plan) mdatHeader() []byte {
	if p.mdatHeaderSize() == 16 {
		h := make([]byte, 16)
		bmff.PutU32(h[0:4], 1)
		copy(h[4:8], "mdat")
		setU64(h[8:16], uint64(16+p.PayloadBytes))
		return h
	}
	h := make([]byte, 8)
	bmff.PutU32(h[0:4], uint32(8+p.PayloadBytes))
	copy(h[4:8], "mdat")
	return h
}

func setU64(b []byte, v uint64) {
	for i := 7; i >= 0; i-- {
		b[i] = byte(v)
		v >>= 8
	}
}

// resolveMoov builds the final moov and assigns absolute output offsets.
func (p *Plan) resolveMoov() []byte {
	hdrSize := p.mdatHeaderSize()

	build := func(use64 bool, payloadStart int64) []byte {
		sizes := make([]uint32, 0, len(p.Samples))
		durs := make([]uint32, 0, len(p.Samples))
		ctts := make([]int32, 0, len(p.Samples))
		offs := make([]uint64, 0, len(p.Samples))
		keys := make([]uint32, 0)
		extras := make([][]byte, 0, len(p.ExtraTraks))
		for _, et := range p.ExtraTraks {
			extras = append(extras, p.remapExtraTrak(et, payloadStart))
		}
		var number uint32
		var duration uint64
		for i, s := range p.Samples {
			if !s.keep {
				continue
			}
			number++
			sizes = append(sizes, uint32(s.size))
			durs = append(durs, s.duration)
			if i < len(p.Ctts) {
				ctts = append(ctts, p.Ctts[i])
			} else {
				ctts = append(ctts, 0)
			}
			duration += uint64(s.duration)
			off := p.mapOffset(s.srcOff, payloadStart)
			off64 := off
			offs = append(offs, off64)
			if s.key {
				keys = append(keys, number)
			}
		}
		params := moovParams{
			timescale:  p.TimeScale,
			duration:   uint32(duration),
			trackID:    1,
			width:      p.Width,
			height:     p.Height,
			stsd:       p.Stsd,
			durations:  durs,
			ctts:       ctts,
			sizes:      sizes,
			offsets:    offs,
			keyframes:  keys,
			force64:    use64,
			extraTraks: extras,
		}
		return buildMoov(params)
	}

	// Resolve mdat payload start. Faststart places moov before mdat, so a
	// dummy pass measures moov; moov-last puts mdat directly after ftyp.
	var payloadStart int64
	if p.MoovLast {
		payloadStart = int64(len(p.Ftyp)) + int64(hdrSize)
	} else {
		dummy := build(false, 0)
		payloadStart = int64(len(p.Ftyp)) + int64(len(dummy)) + int64(hdrSize)
	}
	moov := build(false, payloadStart)

	need64 := false
	for _, s := range p.Samples {
		if s.keep && p.mapOffset(s.srcOff, payloadStart) >= uint64(1<<32) {
			need64 = true
		}
	}
	if need64 {
		payloadStart64 := payloadStart
		if !p.MoovLast {
			moov64Len := len(build(true, 0))
			payloadStart64 = int64(len(p.Ftyp)) + int64(moov64Len) + int64(hdrSize)
		}
		moov = build(true, payloadStart64)
	}
	return moov
}

func (p *Plan) mapOffset(srcOff, payloadStart int64) uint64 {
	if v, ok := p.mapOffsetOK(srcOff, payloadStart); ok {
		return v
	}
	return uint64(payloadStart)
}

func (p *Plan) mapOffsetOK(srcOff, payloadStart int64) (uint64, bool) {
	for _, sec := range p.Sections {
		if srcOff >= sec.srcStart && srcOff < sec.srcEnd {
			return uint64(payloadStart + sec.relStart + (srcOff - sec.srcStart)), true
		}
	}
	for _, sec := range p.Sections {
		if srcOff == sec.srcEnd {
			return uint64(payloadStart + sec.relStart + sec.length), true
		}
	}
	return 0, false
}

func (p *Plan) writeOutput(w io.Writer, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	if _, err := w.Write(p.Ftyp); err != nil {
		return err
	}

	buf := make([]byte, 1<<20)
	writeMdat := func() error {
		if _, err := w.Write(p.mdatHeader()); err != nil {
			return err
		}
		for _, sec := range p.Sections {
			rs := io.NewSectionReader(src, sec.srcStart, sec.length)
			if _, err := io.CopyBuffer(w, rs, buf); err != nil {
				return fmt.Errorf("copy media payload at %d: %w", sec.srcStart, err)
			}
		}
		return nil
	}

	if p.MoovLast {
		if err := writeMdat(); err != nil {
			return err
		}
		_, err := w.Write(p.resolveMoov())
		return err
	}
	if _, err := w.Write(p.resolveMoov()); err != nil {
		return err
	}
	return writeMdat()
}

func verifyOutput(path string, wantSamples int) error {
	f, info, err := movie.OpenAndParse(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if info.Video == nil || info.Video.Index == nil {
		return errors.New("repaired file has no video sample table")
	}
	idx := info.Video.Index
	if int(idx.SampleCount) != wantSamples {
		return fmt.Errorf("stsz count %d != planned %d", idx.SampleCount, wantSamples)
	}
	offCount := len(idx.Offsets)
	if offCount != wantSamples {
		return fmt.Errorf("stco entry count %d != planned %d", offCount, wantSamples)
	}
	var sz uint64
	for _, s := range idx.Sizes {
		sz += uint64(s)
	}
	if idx.UniformSize != 0 {
		sz = uint64(idx.UniformSize) * uint64(idx.SampleCount)
	}
	for i, o := range idx.Offsets {
		if int64(o) >= info.FileSize {
			return fmt.Errorf("sample %d offset %d beyond file size %d", i+1, o, info.FileSize)
		}
		if i < len(idx.Sizes) {
			size := int64(idx.Sizes[i])
			if int64(o)+size > info.FileSize {
				return fmt.Errorf("sample %d ends beyond file size", i+1)
			}
			if err := checkNALClosure(f, int64(o), size, info.Video.LengthSize); err != nil {
				return fmt.Errorf("sample %d index does not match payload: %w", i+1, err)
			}
		}
	}
	_ = sz
	return nil
}

// checkNALClosure verifies that size bytes at off exactly contain whole
// length-prefixed NAL units, proving stco/stsz agree with the media payload.
func checkNALClosure(f *os.File, off, size int64, lengthSize int) error {
	if lengthSize == 0 {
		lengthSize = 4
	}
	var lbuf [4]byte
	pos := int64(0)
	for pos < size {
		if size-pos < int64(lengthSize)+1 {
			return errors.New("trailing bytes smaller than NAL header")
		}
		if _, err := f.ReadAt(lbuf[:lengthSize], off+pos); err != nil {
			return err
		}
		var nalLen int64
		for i := 0; i < lengthSize; i++ {
			nalLen = nalLen<<8 | int64(lbuf[i])
		}
		wire := int64(lengthSize) + nalLen
		if nalLen < 1 || wire > size-pos {
			return errors.New("invalid NAL length")
		}
		pos += wire
	}
	return nil
}
