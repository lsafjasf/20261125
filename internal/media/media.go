package media

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"sort"
	"strings"
	"time"
)

type Format string

const (
	FormatMP4      Format = "mp4"
	FormatMatroska Format = "matroska"
)

type Kind string

const (
	KindVideo    Kind = "video"
	KindAudio    Kind = "audio"
	KindSubtitle Kind = "subtitle"
	KindOther    Kind = "other"
)

// Sample is one coded access unit. PTS and Duration use Track.Timescale.
// Hash is a truncated SHA-256 of the coded payload in decode order.
type Sample struct {
	Index          int
	PTS            int64
	Duration       int64
	Size           int64
	Hash           uint64
	Key            bool
	Empty          bool
	DiscardPadding int64
}

type Edit struct {
	SegmentDuration uint64
	MediaTime       int64
	MediaRate       float64
}

type Track struct {
	ID               uint64
	Kind             Kind
	Codec            string
	Language         string
	Name             string
	Timescale        uint64
	MovieTimescale   uint64
	Duration         uint64
	DefaultDuration  int64
	CodecDelay       int64
	SeekPreRoll      int64
	TrackOffset      int64
	TrackTimeScale   float64
	Width            int
	Height           int
	SampleRate       int
	Channels         int
	Edits            []Edit
	Samples          []Sample
	CodecPrivateHash uint64
}

type File struct {
	Path             string
	Format           Format
	Brand            string
	DeclaredDuration int64
	Timescale        uint64
	Tracks           []Track
	Warnings         []string
}

type PresentedSample struct {
	DecodeIndex int
	Start       int64
	End         int64
	Hash        uint64
	Key         bool
}

type Span struct {
	Start int64
	End   int64
}

type Rational struct {
	Num int64
	Den int64
}

func (r Rational) Float() float64 {
	if r.Den == 0 {
		return 0
	}
	return float64(r.Num) / float64(r.Den)
}

func (r Rational) String() string {
	if r.Den == 0 {
		return "0"
	}
	if r.Num == 0 || r.Den == 1 {
		return fmt.Sprintf("%d", r.Num)
	}
	return fmt.Sprintf("%d/%d", r.Num, r.Den)
}

func (f *File) Primary(kind Kind) *Track {
	var best *Track
	bestScore := -1
	for i := range f.Tracks {
		t := &f.Tracks[i]
		if t.Kind != kind {
			continue
		}
		score := len(t.Samples)
		if score == 0 {
			score = 1
		}
		if best == nil || score > bestScore {
			best, bestScore = t, score
		}
	}
	return best
}

func (f *File) MaxPresentedEnd() int64 {
	var end int64
	for i := range f.Tracks {
		if x := f.Tracks[i].PresentedSpan().End; x > end {
			end = x
		}
	}
	return end
}

func (t *Track) RawSpan() Span {
	if len(t.Samples) == 0 {
		return Span{}
	}
	start := t.Samples[0].PTS
	end := rawSampleEnd(t.Samples[0])
	for _, s := range t.Samples[1:] {
		if s.PTS < start {
			start = s.PTS
		}
		if e := rawSampleEnd(s); e > end {
			end = e
		}
	}
	return Span{Start: t.ToNanos(start), End: t.ToNanos(end)}
}

func rawSampleEnd(s Sample) int64 {
	return s.PTS + s.Duration
}

func sampleEnd(s Sample) int64 {
	end := s.PTS + s.Duration - s.DiscardPadding
	if end < s.PTS {
		return s.PTS
	}
	return end
}

func (t *Track) ToNanos(v int64) int64 {
	if t.Timescale == 0 || v == 0 {
		return 0
	}
	neg := v < 0
	u := uint64(v)
	if neg {
		u = uint64(-v)
	}
	hi, lo := bits.Mul64(u, uint64(time.Second))
	if hi >= t.Timescale {
		return 0
	}
	q, _ := bits.Div64(hi, lo, t.Timescale)
	if neg {
		return -int64(q)
	}
	return int64(q)
}

func (t *Track) FromNanos(ns int64) int64 {
	if t.Timescale == 0 {
		return 0
	}
	neg := ns < 0
	u := uint64(ns)
	if neg {
		u = uint64(-ns)
	}
	hi, lo := bits.Mul64(u, t.Timescale)
	if hi >= uint64(time.Second) {
		return 0
	}
	q, _ := bits.Div64(hi, lo, uint64(time.Second))
	if neg {
		return -int64(q)
	}
	return int64(q)
}

// PresentedSamples maps coded samples through an MP4 edit list, or applies
// Matroska CodecDelay / DiscardPadding. The result is in nanoseconds.
func (t *Track) PresentedSamples() []PresentedSample {
	if t == nil || len(t.Samples) == 0 {
		return nil
	}
	if t.MovieTimescale == 0 {
		return t.presentedWithoutMovieEdits()
	}
	return t.presentedWithEdits()
}

func (t *Track) presentedWithoutMovieEdits() []PresentedSample {
	out := make([]PresentedSample, 0, len(t.Samples))
	for _, s := range t.Samples {
		start := t.ToNanos(s.PTS)
		end := t.ToNanos(sampleEnd(s))
		if t.CodecDelay > 0 {
			start -= t.CodecDelay
			end -= t.CodecDelay
		}
		if start < 0 {
			start = 0
		}
		if end < start {
			end = start
		}
		out = append(out, PresentedSample{
			DecodeIndex: s.Index, Start: start, End: end,
			Hash: s.Hash, Key: s.Key,
		})
	}
	sortPresented(out)
	return out
}

func (t *Track) presentedWithEdits() []PresentedSample {
	if len(t.Edits) == 0 {
		return t.presentedWithoutMovieEdits()
	}
	type raw struct {
		s     Sample
		start int64
		end   int64
	}
	raws := make([]raw, 0, len(t.Samples))
	for _, s := range t.Samples {
		raws = append(raws, raw{s: s, start: s.PTS, end: sampleEnd(s)})
	}
	sort.SliceStable(raws, func(i, j int) bool {
		if raws[i].start != raws[j].start {
			return raws[i].start < raws[j].start
		}
		return raws[i].s.Index < raws[j].s.Index
	})

	var cursor uint64
	out := make([]PresentedSample, 0, len(t.Samples))
	for _, e := range t.Edits {
		seg := e.SegmentDuration
		if seg == 0 {
			continue
		}
		if e.MediaTime == -1 {
			cursor += seg
			continue
		}
		rate := e.MediaRate
		if rate == 0 {
			rate = 1
		}
		mediaStart := e.MediaTime
		mediaLen := int64(float64(seg) * float64(t.Timescale) / float64(t.MovieTimescale) * rate)
		mediaEnd := mediaStart + mediaLen
		for _, r := range raws {
			lo, hi := max64(r.start, mediaStart), min64(r.end, mediaEnd)
			if hi <= lo {
				continue
			}
			scale := float64(t.MovieTimescale) / (float64(t.Timescale) * rate)
			startMovie := int64(cursor) + int64(float64(lo-mediaStart)*scale+0.5)
			endMovie := int64(cursor) + int64(float64(hi-mediaStart)*scale+0.5)
			startNS := scaleNanos(startMovie, t.MovieTimescale)
			endNS := scaleNanos(endMovie, t.MovieTimescale)
			out = append(out, PresentedSample{
				DecodeIndex: r.s.Index, Start: startNS, End: endNS,
				Hash: r.s.Hash, Key: r.s.Key,
			})
		}
		cursor += seg
	}
	sortPresented(out)
	return out
}

func scaleNanos(v int64, ts uint64) int64 {
	if ts == 0 {
		return 0
	}
	neg := v < 0
	u := uint64(v)
	if neg {
		u = uint64(-v)
	}
	hi, lo := bits.Mul64(u, uint64(time.Second))
	if hi >= ts {
		return 0
	}
	q, _ := bits.Div64(hi, lo, ts)
	if neg {
		return -int64(q)
	}
	return int64(q)
}

func sortPresented(s []PresentedSample) {
	sort.SliceStable(s, func(i, j int) bool {
		if s[i].Start != s[j].Start {
			return s[i].Start < s[j].Start
		}
		return s[i].DecodeIndex < s[j].DecodeIndex
	})
}

func (t *Track) PresentedSpan() Span {
	ps := t.PresentedSamples()
	if len(ps) == 0 {
		return Span{}
	}
	return Span{Start: ps[0].Start, End: ps[len(ps)-1].End}
}

func (t *Track) RawDuration() int64 {
	return t.ToNanos(int64(t.Duration))
}

func (t *Track) PresentedDuration() int64 {
	s := t.PresentedSpan()
	if s.End <= s.Start {
		return 0
	}
	return s.End - s.Start
}

func (t *Track) HeadTrim() int64 {
	if len(t.Edits) > 0 && t.MovieTimescale > 0 {
		raw := t.RawSpan().Start
		for _, e := range t.Edits {
			if e.MediaTime < 0 {
				continue
			}
			mediaStart := t.ToNanos(e.MediaTime)
			if mediaStart > raw {
				return mediaStart - raw
			}
			break
		}
	}
	if t.CodecDelay > 0 {
		return t.CodecDelay
	}
	raw := t.RawSpan().Start
	presented := t.PresentedSpan().Start
	if raw > presented {
		return raw - presented
	}
	return 0
}

func (t *Track) TailTrim() int64 {
	if len(t.Samples) > 0 && t.Samples[len(t.Samples)-1].DiscardPadding > 0 {
		return t.Samples[len(t.Samples)-1].DiscardPadding
	}
	raw := t.RawSpan().End
	presented := t.PresentedSpan().End
	if raw > presented {
		return raw - presented
	}
	return 0
}

func (t *Track) FrameRate() Rational {
	if t.Kind != KindVideo || len(t.Samples) == 0 || t.Timescale == 0 {
		return Rational{}
	}
	counts := make(map[int64]int64)
	var best int64
	var bestCount int64
	for _, s := range t.Samples {
		if s.Duration <= 0 {
			continue
		}
		counts[s.Duration]++
		if counts[s.Duration] > bestCount ||
			(counts[s.Duration] == bestCount && s.Duration < best) {
			best, bestCount = s.Duration, counts[s.Duration]
		}
	}
	var r Rational
	if best > 0 {
		r = reduce(int64(t.Timescale), best)
	} else {
		span := t.RawSpan()
		if span.End > span.Start && len(t.Samples) > 1 {
			r = approximateFPS(float64(len(t.Samples)-1) * float64(time.Second) /
				float64(span.End-span.Start))
		}
	}
	return snapFrameRate(r)
}

func reduce(n, d int64) Rational {
	if d == 0 {
		return Rational{}
	}
	if d < 0 {
		n, d = -n, -d
	}
	g := gcd(abs64(n), d)
	if g > 1 {
		n /= g
		d /= g
	}
	return Rational{Num: n, Den: d}
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func approximateFPS(f float64) Rational {
	if f <= 0 {
		return Rational{}
	}
	best := Rational{Num: int64(math.Round(f * 1000)), Den: 1000}
	bestErr := math.Abs(best.Float() - f)
	for den := int64(1); den <= 1001 && bestErr > 1e-7; den++ {
		num := int64(math.Round(f * float64(den)))
		if num <= 0 {
			continue
		}
		r := reduce(num, den)
		err := math.Abs(r.Float() - f)
		if err < bestErr {
			best, bestErr = r, err
		}
	}
	return best
}

var commonRates = []Rational{
	{24000, 1001}, {24, 1}, {25, 1}, {30000, 1001}, {30, 1},
	{48, 1}, {50, 1}, {60000, 1001}, {60, 1}, {120, 1}, {240, 1},
}

func snapFrameRate(r Rational) Rational {
	if r.Den == 0 || r.Float() <= 0 {
		return r
	}
	f := r.Float()
	for _, c := range commonRates {
		if math.Abs(f-c.Float())/c.Float() < 0.0002 {
			return c
		}
	}
	return r
}

func (t *Track) FrameDurationNanos() int64 {
	r := t.FrameRate()
	if r.Num <= 0 || r.Den <= 0 {
		return 0
	}
	return int64(math.Round(float64(r.Den) * float64(time.Second) / float64(r.Num)))
}

func FrameRateEqual(a, b Rational) bool {
	if a.Den == 0 || b.Den == 0 {
		return false
	}
	return abs64(a.Num*b.Den-b.Num*a.Den)*1_000_000 <= max64(a.Num*b.Den, b.Num*a.Den)
}

func (t *Track) Description() string {
	var b strings.Builder
	b.WriteString(string(t.Kind))
	if t.Codec != "" {
		fmt.Fprintf(&b, " %s", t.Codec)
	}
	if t.Width > 0 && t.Height > 0 {
		fmt.Fprintf(&b, " %dx%d", t.Width, t.Height)
	}
	if t.SampleRate > 0 {
		fmt.Fprintf(&b, " %dHz", t.SampleRate)
	}
	if t.Channels > 0 {
		fmt.Fprintf(&b, "/%dch", t.Channels)
	}
	return b.String()
}

func (t *Track) Validate() error {
	if t.Timescale == 0 && len(t.Samples) > 0 {
		return errors.New("track has samples but zero timescale")
	}
	for i, s := range t.Samples {
		if s.Index != i {
			return fmt.Errorf("sample index %d stored at %d", s.Index, i)
		}
	}
	return nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
