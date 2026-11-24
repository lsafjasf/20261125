package repair

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"

	"mp4index/bmff"
	"mp4index/h264scan"
	"mp4index/movie"
)

type planSample struct {
	scanIdx  int
	srcOff   int64
	size     int64
	key      bool
	duration uint32
	hash     string
	keep     bool
	outOff   uint64
}

type outSection struct {
	srcStart int64
	srcEnd   int64
	outStart int64 // absolute offset in output mdat payload space delta basis
	relStart int64 // offset inside concatenated output payload
	length   int64
}

// Plan is a fully resolved repair: scanned samples plus output layout.
type Plan struct {
	SrcPath       string
	Info          *movie.Info
	Ftyp          []byte
	Stsd          []byte
	TimeScale     uint32
	Width         uint32
	Height        uint32
	LengthSize    int
	Codec         string
	Ctts          []int32
	Samples       []planSample
	Sections      []outSection
	PayloadBytes  int64
	MdatHeader    int
	Report        *Report
	UsedReference bool
	MoovLast      bool
	ExtraTraks    [][]byte // preserved non-video track boxes
}

// Analyze scans srcPath and builds the repair plan without writing anything.
func Analyze(srcPath string, opts Options) (*Plan, error) {
	f, info, err := movie.OpenAndParse(srcPath)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	defer f.Close()

	rep := &Report{
		Input:       srcPath,
		FileSize:    info.FileSize,
		Intervals:   []Interval{},
		Unrecovered: []UnrecoveredRange{},
	}

	usedRef := false
	var refInfo *movie.Info
	if (!info.HasMoov || info.Video == nil || len(info.Video.StsdBox) == 0) && opts.Reference != "" {
		rf, rinfo, rerr := movie.OpenAndParse(opts.Reference)
		if rerr != nil {
			return nil, fmt.Errorf("open reference: %w", rerr)
		}
		refInfo = rinfo
		rf.Close()
		if rinfo.Video != nil && len(rinfo.Video.StsdBox) > 0 {
			usedRef = true
		}
	}
	rep.Damage.MissingMoov = !info.HasMoov
	rep.Damage.UsedReference = usedRef

	var track *movie.Track
	if info.Video != nil {
		track = info.Video
	} else if refInfo != nil && refInfo.Video != nil {
		track = refInfo.Video
	}
	if track == nil || len(track.StsdBox) == 0 {
		return nil, errors.New("no video sample description (stsd/avcC) found; pass a healthy reference with -ref")
	}

	ts := track.TimeScale
	if ts == 0 {
		ts = opts.Timescale
	}
	if ts == 0 {
		ts = 15360
	}
	rep.MediaTimescale = ts

	codec := h264scan.CodecAVC
	if track.Codec == "hvc1" || track.Codec == "hev1" {
		codec = h264scan.CodecHEVC
	}
	lengthSize := track.LengthSize
	if lengthSize == 0 {
		lengthSize = 4
	}

	if len(info.Mdats) == 0 {
		return nil, errors.New("no mdat media section found")
	}

	// ---- scan every mdat payload -----------------------------------------
	var scanned []h264scan.Sample
	var scanGaps []h264scan.Gap
	for _, sec := range info.Mdats {
		if sec.PayloadLength <= 0 {
			continue
		}
		res, serr := h264scan.ScanRange(f, sec.PayloadOffset,
			sec.PayloadOffset+sec.PayloadLength,
			h264scan.Options{Codec: codec, LengthSize: lengthSize})
		if serr != nil {
			return nil, fmt.Errorf("scan mdat at %d: %w", sec.PayloadOffset, serr)
		}
		scanned = append(scanned, res.Samples...)
		scanGaps = append(scanGaps, res.Gaps...)
	}
	sort.SliceStable(scanned, func(i, j int) bool { return scanned[i].Offset < scanned[j].Offset })

	if len(scanned) == 0 {
		return nil, errors.New("no complete samples found in media payload")
	}

	// ---- durations aligned to scanned samples ----------------------------
	durations := alignedDurations(track, len(scanned), ts, opts.FPS)
	ctts := alignedCtts(track, len(scanned))
	normalizeCtts(ctts)

	// ---- content hashes + duplicate detection ----------------------------
	dupStream := 0
	prevHash := ""
	samples := make([]planSample, len(scanned))
	for i, s := range scanned {
		h := hashRange(f, s.Offset, s.Size)
		if i > 0 && h == prevHash &&
			scanned[i].Offset == scanned[i-1].Offset+scanned[i-1].Size {
			dupStream++
		}
		prevHash = h
		samples[i] = planSample{
			scanIdx:  i,
			srcOff:   s.Offset,
			size:     s.Size,
			key:      s.Keyframe,
			duration: durations[i],
			hash:     h,
			keep:     true,
		}
	}
	rep.Damage.StreamDuplicateSamples = dupStream

	if opts.DedupeFrames {
		for i := 1; i < len(samples); i++ {
			if samples[i].hash == samples[i-1].hash &&
				samples[i].srcOff == samples[i-1].srcOff+samples[i-1].size {
				samples[i].keep = false
			}
		}
	}

	// ---- old index diagnostics -------------------------------------------
	diagnoseOldIndex(info, scanned, rep)

	// ---- unrecovered byte ranges from scan gaps ---------------------------
	for _, g := range scanGaps {
		kind := "scan_gap"
		if containsTrunc(g.Reason) {
			kind = "truncated_tail"
			rep.Damage.Truncated = true
		}
		rep.Unrecovered = append(rep.Unrecovered, UnrecoveredRange{
			Kind: kind, StartOffset: g.Start, EndOffset: g.End,
			Length: g.End - g.Start, Detail: g.Reason,
		})
	}
	if track.Index != nil && int(track.Index.SampleCount) > len(scanned) {
		excess := int(track.Index.SampleCount) - len(scanned)
		dupExtra := rep.Damage.IndexDuplicateRecords
		if rep.Damage.IndexDuplicateOffsets > dupExtra {
			dupExtra = rep.Damage.IndexDuplicateOffsets
		}
		unexplained := excess - dupExtra
		if unexplained < 0 {
			unexplained = 0
		}
		if unexplained > 0 {
			rep.Damage.Truncated = true
			rep.Unrecovered = append(rep.Unrecovered, UnrecoveredRange{
				Kind: "missing_index_samples", MissingSamples: unexplained,
				Detail: fmt.Sprintf("stsz lists %d samples but only %d complete samples were scanned; %d extra slots not explained by duplicate records",
					track.Index.SampleCount, len(scanned), unexplained),
			})
		}
	}
	for _, sec := range info.Mdats {
		if sec.DeclaredLength > sec.PayloadLength {
			rep.Damage.Truncated = true
		}
	}

	// ---- output sections (verbatim mdat payload copy) --------------------
	sections := make([]outSection, 0, len(info.Mdats))
	var rel int64
	var payloadBytes int64
	for _, sec := range info.Mdats {
		sections = append(sections, outSection{
			srcStart: sec.PayloadOffset,
			srcEnd:   sec.PayloadOffset + sec.PayloadLength,
			relStart: rel,
			length:   sec.PayloadLength,
		})
		rel += sec.PayloadLength
		payloadBytes += sec.PayloadLength
	}

	ftyp := info.FtypBox
	if len(ftyp) == 0 && refInfo != nil {
		ftyp = refInfo.FtypBox
	}
	if len(ftyp) == 0 {
		ftyp = defaultFtyp()
	}

	var extraTraks [][]byte
	for _, tr := range info.Tracks {
		if tr == info.Video {
			continue
		}
		if len(tr.RawTrak) > 0 {
			extraTraks = append(extraTraks, tr.RawTrak)
		}
	}

	plan := &Plan{
		SrcPath:       srcPath,
		Info:          info,
		Ftyp:          ftyp,
		Stsd:          track.StsdBox,
		TimeScale:     ts,
		Width:         track.Width,
		Height:        track.Height,
		LengthSize:    lengthSize,
		Codec:         track.Codec,
		Ctts:          ctts,
		Samples:       samples,
		Sections:      sections,
		PayloadBytes:  payloadBytes,
		Report:        rep,
		UsedReference: usedRef,
		MoovLast:      opts.MoovLast,
		ExtraTraks:    extraTraks,
	}

	buildIntervals(plan)
	rep.ScannedSamples = len(scanned)
	rep.IndexedSamples = countKept(samples)
	rep.PayloadBytes = payloadBytes
	rep.Keyframes = collectKeyNumbers(samples)
	if n := rep.IndexedSamples; n > 0 {
		var totalDur uint64
		for _, s := range samples {
			if s.keep {
				totalDur += uint64(s.duration)
			}
		}
		if totalDur > 0 {
			rep.NominalFPS = float64(ts) / float64(totalDur/uint64(n))
		}
	}
	return plan, nil
}

func hashRange(ra io.ReaderAt, off, n int64) string {
	h := sha256.New()
	if n <= 0 {
		return ""
	}
	if _, err := io.CopyN(h, io.NewSectionReader(ra, off, n), n); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func alignedDurations(track *movie.Track, n int, timescale uint32, fps float64) []uint32 {
	out := make([]uint32, n)
	var expanded []uint32
	if track.Index != nil {
		expanded = expandStts(track.Index.Stts, n+64)
	}
	fallback := uint32(0)
	if len(expanded) > 0 {
		for _, d := range expanded {
			if d > 0 {
				fallback = d
			}
		}
	}
	if fallback == 0 {
		if fps <= 0 {
			fps = 30
		}
		fallback = uint32(float64(timescale) / fps)
		if fallback == 0 {
			fallback = 512
		}
	}
	for i := 0; i < n; i++ {
		if i < len(expanded) && expanded[i] > 0 {
			out[i] = expanded[i]
		} else {
			out[i] = fallback
		}
	}
	return out
}

func alignedCtts(track *movie.Track, n int) []int32 {
	out := make([]int32, n)
	for i := 0; i < n && i < len(track.Ctts); i++ {
		out[i] = track.Ctts[i]
	}
	return out
}

// normalizeCtts shifts composition offsets so the minimum is zero, replacing
// the role of an edit list without carrying timescale-dependent elst bytes.
func normalizeCtts(c []int32) {
	if len(c) == 0 {
		return
	}
	min := c[0]
	for _, v := range c[1:] {
		if v < min {
			min = v
		}
	}
	if min != 0 {
		for i := range c {
			c[i] -= min
		}
	}
}

func expandStts(entries []movie.SttsEntry, capHint int) []uint32 {
	const maxExpand = 5_000_000
	out := make([]uint32, 0, min(capHint, 1024))
	for _, e := range entries {
		c := e.Count
		if uint64(len(out))+uint64(c) > maxExpand {
			c = uint32(maxExpand - len(out))
		}
		for i := uint32(0); i < c; i++ {
			out = append(out, e.Duration)
		}
		if len(out) >= maxExpand {
			break
		}
	}
	return out
}

func diagnoseOldIndex(info *movie.Info, scanned []h264scan.Sample, rep *Report) {
	if info.Video == nil || info.Video.Index == nil {
		return
	}
	old := info.Video.Index
	rep.Damage.OldIndexSampleCount = int(old.SampleCount)
	rep.Damage.OldIndexOffsetCount = len(old.Offsets)

	validOff := make(map[int64]bool, len(scanned))
	for _, s := range scanned {
		validOff[s.Offset] = true
	}

	seen := make(map[uint64]int, len(old.Offsets))
	dupOff := 0
	for _, o := range old.Offsets {
		seen[o]++
		if seen[o] > 1 {
			dupOff++
		}
	}
	rep.Damage.IndexDuplicateOffsets = dupOff

	// Duplicate (offset,size) records line up duplicated stco/stsz entries.
	dupRec := 0
	recordSeen := make(map[string]int)
	for i, o := range old.Offsets {
		var sz uint32
		if old.UniformSize != 0 {
			sz = old.UniformSize
		} else if i < len(old.Sizes) {
			sz = old.Sizes[i]
		}
		key := fmt.Sprintf("%d:%d", o, sz)
		recordSeen[key]++
		if recordSeen[key] > 1 {
			dupRec++
		}
	}
	rep.Damage.IndexDuplicateRecords = dupRec

	bad := 0
	for _, o := range old.Offsets {
		if !validOff[int64(o)] {
			bad++
		}
	}
	rep.Damage.BadOffsets = bad
}

func containsTrunc(s string) bool {
	return len(s) >= 9 && (indexOf(s, "truncat") >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func countKept(ss []planSample) int {
	n := 0
	for _, s := range ss {
		if s.keep {
			n++
		}
	}
	return n
}

func collectKeyNumbers(ss []planSample) []int {
	out := []int{}
	num := 0
	for _, s := range ss {
		if !s.keep {
			continue
		}
		num++
		if s.key {
			out = append(out, num)
		}
	}
	return out
}

func defaultFtyp() []byte {
	payload := make([]byte, 16)
	copy(payload[0:4], "isom") // major brand
	payload[6] = 0x02          // minor version 0x00000200
	copy(payload[8:12], "isom")
	copy(payload[12:16], "mp41")
	return bmff.BuildBox("ftyp", payload)
}
