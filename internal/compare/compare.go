// Package compare aligns two FFV1 captures and classifies frame-hash
// differences as encoding-side or decoding-side evidence.
package compare

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"ffv1compare/internal/ffv1"
	"ffv1compare/internal/framemd5"
	"ffv1compare/internal/mkv"
)

type Frame struct {
	Index               int
	Keyframe            bool
	PTSNS               int64
	DurationNS          int64
	SampleSize          int
	EncodedSHA256       string
	SliceCount          int
	SliceCRCEnabled     bool
	SliceCRCOK          bool
	SliceSetSHA256      string
	SliceSequenceSHA256 string
	SliceHashes         []string
	DecodedHash         string
	DecodedHashKind     string
	ExpectedSlices      int
}

type Capture struct {
	Path            string
	FileSize        int64
	CodecID         string
	CodecPrivateOff int64
	Width           uint64
	Height          uint64
	Config          ffv1.Config
	TimestampScale  uint64
	TrackNumber     uint64
	Frames          []Frame
	ContainerCRCs   []mkv.ContainerCRC
	ContainerCRCOK  int
	ContainerCRCBad int
	HashPath        string
	Issues          []string
}

type FrameComparison struct {
	Index      int
	A          Frame
	B          Frame
	PTSDeltaNS int64
	Verdict    string
	Reasons    []string
}

type Result struct {
	A          *Capture
	B          *Capture
	Frames     []FrameComparison
	Verdict    string
	Confidence string
	Reasons    []string
	Counts     map[string]int
}

const (
	VerdictIdentical           = "identical"
	VerdictDecoding            = "decoding_difference"
	VerdictEncoding            = "encoding_difference"
	VerdictEncodingSliceOrder  = "encoding_slice_order"
	VerdictEncodingSliceRaster = "encoding_slice_raster"
	VerdictEncodingPixelFormat = "encoding_pixel_format"
	VerdictEncodingCRC         = "encoding_crc_setting"
	VerdictTimestamp           = "container_timestamp_difference"
	VerdictContainerConfig     = "container_config_difference"
	VerdictBitstreamIdentical  = "bitstream_identical"
)

// LoadCapture parses one Matroska/FFV1 file and, optionally, a decoded-frame
// hash manifest (FFmpeg framemd5/framecrc text format).
func LoadCapture(path, hashPath string) (*Capture, error) {
	var c *Capture
	var track mkv.Track
	var expected int
	doc, err := mkv.ParseFileWithOptions(path, mkv.ParseOptions{
		OnFrame: func(doc *mkv.Document, fr mkv.Frame) error {
			if c == nil {
				tr, ok := selectVideoTrack(doc)
				if !ok {
					return fmt.Errorf("%s: no FFV1 video track", path)
				}
				track = tr
				c = newCapture(doc, track, path, hashPath)
				if c.Config.Parsed {
					expected = c.Config.NumHSlices * c.Config.NumVSlices
				}
			}
			if fr.TrackNumber != track.Number {
				return nil
			}
			c.addFrame(fr, expected)
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	if c == nil {
		tr, ok := selectVideoTrack(doc)
		if !ok {
			return nil, fmt.Errorf("%s: no FFV1 video track", path)
		}
		track = tr
		c = newCapture(doc, track, path, hashPath)
		if c.Config.Parsed {
			expected = c.Config.NumHSlices * c.Config.NumVSlices
		}
	}
	c.finish(doc)
	if len(c.Frames) == 0 {
		return nil, fmt.Errorf("%s: no video frames found for track %d", path, track.Number)
	}
	if hashPath != "" {
		m, err := framemd5.ParseFile(hashPath)
		if err != nil {
			return nil, err
		}
		hashes := m.Hashes(0)
		for i := range c.Frames {
			if i < len(hashes) {
				c.Frames[i].DecodedHash = hashes[i]
				c.Frames[i].DecodedHashKind = m.Hash
			}
		}
		if len(hashes) != len(c.Frames) {
			c.Issues = append(c.Issues,
				fmt.Sprintf("decoded hash manifest has %d frames, container has %d", len(hashes), len(c.Frames)))
		}
	}
	return c, nil
}

func newCapture(doc *mkv.Document, track mkv.Track, path, hashPath string) *Capture {
	return &Capture{
		Path:            path,
		FileSize:        doc.FileSize,
		CodecID:         track.CodecID,
		CodecPrivateOff: track.CodecPrivateOff,
		Width:           track.Width,
		Height:          track.Height,
		Config:          ffv1.ParseConfig(track.CodecID, track.CodecPrivate),
		TimestampScale:  doc.TimestampScaleNS,
		TrackNumber:     track.Number,
		HashPath:        hashPath,
	}
}

func (c *Capture) addFrame(fr mkv.Frame, expected int) {
	report := ffv1.AnalyzeSlices(fr.Sample, expected)
	sum := sha256.Sum256(fr.Sample)
	c.Frames = append(c.Frames, Frame{
		Index:               len(c.Frames),
		Keyframe:            fr.Keyframe,
		PTSNS:               fr.PTSNS,
		DurationNS:          fr.DurationNS,
		SampleSize:          fr.SampleSize,
		EncodedSHA256:       hex.EncodeToString(sum[:]),
		SliceCount:          report.Count,
		SliceCRCEnabled:     report.CRCEnabled,
		SliceCRCOK:          report.CRCOK,
		SliceSetSHA256:      report.SetSHA256,
		SliceSequenceSHA256: report.SequenceSHA256,
		SliceHashes:         payloadHashes(report),
		ExpectedSlices:      expected,
	})
	if report.Error != "" {
		c.Issues = append(c.Issues, fmt.Sprintf("frame %d: %s", len(c.Frames)-1, report.Error))
	}
}

func (c *Capture) finish(doc *mkv.Document) {
	c.ContainerCRCs = doc.ContainerCRCs
	for _, issue := range doc.Issues {
		c.Issues = append(c.Issues, fmt.Sprintf("%s @%d: %s", issue.Code, issue.Offset, issue.Message))
	}
	for _, crc := range doc.ContainerCRCs {
		if crc.OK {
			c.ContainerCRCOK++
		} else {
			c.ContainerCRCBad++
		}
	}
}

func selectVideoTrack(doc *mkv.Document) (mkv.Track, bool) {
	for _, t := range doc.Tracks {
		if t.Type != 0 && t.Type != 1 {
			continue
		}
		if strings.EqualFold(t.CodecID, "V_FFV1") {
			return t, true
		}
		if strings.EqualFold(t.CodecID, "V_MS/VFW/FOURCC") && len(t.CodecPrivate) >= 20 &&
			string(t.CodecPrivate[16:20]) == "FFV1" {
			return t, true
		}
	}
	return mkv.Track{}, false
}

func payloadHashes(r ffv1.SliceReport) []string {
	out := make([]string, 0, len(r.Slices))
	for _, s := range r.Slices {
		out = append(out, s.PayloadSHA256)
	}
	return out
}

// Compare aligns frames by index and produces a per-frame verdict.
func Compare(a, b *Capture) *Result {
	res := &Result{A: a, B: b, Counts: make(map[string]int)}
	n := len(a.Frames)
	if len(b.Frames) < n {
		n = len(b.Frames)
	}
	for i := 0; i < n; i++ {
		fc := compareFrame(a.Frames[i], b.Frames[i], samePixelFormat(a, b), sameSliceRaster(a, b))
		res.Frames = append(res.Frames, fc)
		res.Counts[fc.Verdict]++
	}
	if len(a.Frames) != len(b.Frames) {
		res.Reasons = append(res.Reasons,
			fmt.Sprintf("frame count differs: A=%d B=%d; only the first %d frames were aligned",
				len(a.Frames), len(b.Frames), n))
	}
	res.Verdict, res.Confidence, res.Reasons = fileVerdict(res)
	return res
}

func samePixelFormat(a, b *Capture) bool {
	return a.Config.PixelFormat == b.Config.PixelFormat &&
		a.Config.BitsPerRawSample == b.Config.BitsPerRawSample &&
		a.Config.Colorspace == b.Config.Colorspace
}

func sameSliceRaster(a, b *Capture) bool {
	return a.Config.NumHSlices == b.Config.NumHSlices &&
		a.Config.NumVSlices == b.Config.NumVSlices
}

func compareFrame(a, b Frame, samePixFmt, sameRaster bool) FrameComparison {
	fc := FrameComparison{Index: a.Index, A: a, B: b, PTSDeltaNS: b.PTSNS - a.PTSNS}
	sameEncoded := a.EncodedSHA256 == b.EncodedSHA256
	sameSliceSet := a.SliceSetSHA256 != "" && a.SliceSetSHA256 == b.SliceSetSHA256
	sameSliceSeq := a.SliceSequenceSHA256 != "" && a.SliceSequenceSHA256 == b.SliceSequenceSHA256
	sameDecodedKnown := a.DecodedHash != "" && b.DecodedHash != ""
	sameDecoded := sameDecodedKnown && a.DecodedHash == b.DecodedHash
	samePTS := a.PTSNS == b.PTSNS
	sameSlices := a.SliceCount == b.SliceCount && sameSliceSeq
	sameCRC := a.SliceCRCEnabled == b.SliceCRCEnabled && a.SliceCRCOK == b.SliceCRCOK

	add := func(format string, args ...any) {
		fc.Reasons = append(fc.Reasons, fmt.Sprintf(format, args...))
	}

	switch {
	case sameEncoded:
		if !samePixFmt {
			fc.Verdict = VerdictContainerConfig
			add("encoded sample bytes are identical but the FFV1 configuration/pixel format differs")
		} else if sameDecodedKnown && !sameDecoded {
			fc.Verdict = VerdictDecoding
			add("encoded sample is byte-identical but decoded frame hashes differ")
		} else if !samePTS {
			fc.Verdict = VerdictTimestamp
			add("encoded sample is byte-identical; only the container timestamp differs")
		} else if sameDecodedKnown {
			fc.Verdict = VerdictIdentical
			add("encoded sample and decoded frame hash are identical")
		} else {
			fc.Verdict = VerdictBitstreamIdentical
			add("encoded sample is byte-identical; no decoded hash was supplied")
		}
		if !sameSlices {
			add("slice split differs despite identical sample bytes (unexpected)")
		}
		return fc
	}

	// Encoded samples differ. Prefer explanations that are internal to the
	// stream/container before falling back to a generic encoding verdict.
	switch {
	case sameSliceSet && a.SliceCount == b.SliceCount && !sameSliceSeq:
		fc.Verdict = VerdictEncodingSliceOrder
		add("same slice payload multiset, different storage order")
	case sameSliceSet && a.SliceCount == b.SliceCount && a.SliceCRCEnabled != b.SliceCRCEnabled:
		fc.Verdict = VerdictEncodingCRC
		add("same slice payload multiset; per-slice CRC setting differs")
	case !sameCRC && sameSliceSet:
		fc.Verdict = VerdictEncodingCRC
		add("same slice payload multiset; CRC validation state differs")
	case !samePixFmt:
		fc.Verdict = VerdictEncodingPixelFormat
		add("encoded pixel format/bit depth differs")
	case !sameRaster:
		fc.Verdict = VerdictEncodingSliceRaster
		add("slice raster/slice count differs")
	case sameDecodedKnown && sameDecoded:
		fc.Verdict = VerdictEncoding
		add("different encoded bytes decode to identical frame hashes")
	default:
		fc.Verdict = VerdictEncoding
		add("encoded sample differs")
	}
	if !samePTS {
		add("frame timestamps also differ by %d ns", fc.PTSDeltaNS)
	}
	if sameDecodedKnown && !sameDecoded {
		add("decoded frame hashes also differ; this can be caused by encoding differences and does not by itself prove a decoder defect")
	}
	return fc
}

func fileVerdict(res *Result) (string, string, []string) {
	var reasons []string
	a, b := res.A, res.B
	configDiff := a.Config.PixelFormat != b.Config.PixelFormat ||
		a.Config.BitsPerRawSample != b.Config.BitsPerRawSample ||
		a.Config.NumHSlices != b.Config.NumHSlices ||
		a.Config.NumVSlices != b.Config.NumVSlices ||
		a.Config.CoderType != b.Config.CoderType ||
		a.Config.MicroVersion != b.Config.MicroVersion
	if a.Config.Parsed && b.Config.Parsed && configDiff {
		reasons = append(reasons,
			fmt.Sprintf("codec configuration differs: A=%s B=%s",
				a.Config.Descriptor(), b.Config.Descriptor()))
	}
	if a.Config.Parsed != b.Config.Parsed {
		reasons = append(reasons, "one configuration record could not be decoded")
	}
	if a.ContainerCRCBad > 0 || b.ContainerCRCBad > 0 {
		reasons = append(reasons,
			fmt.Sprintf("container CRC failures: A=%d B=%d", a.ContainerCRCBad, b.ContainerCRCBad))
	}
	if a.TimestampScale != b.TimestampScale {
		reasons = append(reasons,
			fmt.Sprintf("TimestampScale differs: A=%d ns B=%d ns", a.TimestampScale, b.TimestampScale))
	}

	decoding := res.Counts[VerdictDecoding]
	encoding := res.Counts[VerdictEncoding] +
		res.Counts[VerdictEncodingSliceOrder] +
		res.Counts[VerdictEncodingSliceRaster] +
		res.Counts[VerdictEncodingCRC] +
		res.Counts[VerdictEncodingPixelFormat]
	timestamps := res.Counts[VerdictTimestamp]
	containerConfig := res.Counts[VerdictContainerConfig]
	identical := res.Counts[VerdictIdentical] + res.Counts[VerdictBitstreamIdentical]

	allDecodedKnown := len(res.Frames) > 0
	allDecodedEqual := true
	anyEncodedDiff := false
	anyEncodedSame := false
	for _, f := range res.Frames {
		if f.A.DecodedHash == "" || f.B.DecodedHash == "" {
			allDecodedKnown = false
		} else if f.A.DecodedHash != f.B.DecodedHash {
			allDecodedEqual = false
		}
		if f.A.EncodedSHA256 != f.B.EncodedSHA256 {
			anyEncodedDiff = true
		} else {
			anyEncodedSame = true
		}
	}

	switch {
	case decoding > 0:
		reasons = append(reasons,
			fmt.Sprintf("%d frame(s) have byte-identical encoded samples but different decoded hashes", decoding))
		if encoding > 0 {
			return "mixed_encoding_and_decoding_difference", "high", reasons
		}
		return "decoding_difference", "high", reasons
	case encoding > 0:
		if allDecodedKnown && allDecodedEqual {
			reasons = append(reasons,
				"all decoded frame hashes match; the difference is confined to the encoded bitstream/container")
			return "encoding_difference", "high", reasons
		}
		confidence := "medium"
		if configDiff {
			reasons = append(reasons, "the codec configuration (pixel format/slice raster) is part of the encoded stream")
			confidence = "high"
		}
		if res.Counts[VerdictEncodingSliceOrder] > 0 {
			reasons = append(reasons, "slice payloads are merely reordered; timestamps and coding parameters are otherwise consistent")
		}
		if res.Counts[VerdictEncodingSliceRaster] > 0 {
			reasons = append(reasons, "the slice raster was re-cut while decoded pixels stayed comparable")
		}
		return "encoding_difference", confidence, reasons
	case timestamps > 0 && identical+timestamps == len(res.Frames):
		reasons = append(reasons, "codec-level fingerprints match; only container timestamps differ")
		return "container_timestamp_difference", "high", reasons
	case containerConfig > 0 && containerConfig+identical == len(res.Frames):
		reasons = append(reasons,
			"sample bytes match but the FFV1 configuration differs; compare CodecPrivate before blaming a decoder")
		return VerdictContainerConfig, "high", reasons
	case identical == len(res.Frames):
		reasons = append(reasons, "all aligned frame fingerprints match")
		return "no_difference", "high", reasons
	default:
		if anyEncodedSame && anyEncodedDiff {
			reasons = append(reasons, "captures are partially aligned; inspect the per-frame table")
		}
		return "undetermined", "low", reasons
	}
}
