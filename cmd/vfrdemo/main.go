// Command vfrdemo walks a full VFR retiming workflow:
//
//  1. build a timeline of speed segments,
//  2. sample source frames with sparse sampling and repeated PTS,
//  3. map output PTS back to source frames (with candidates),
//  4. move a cut and watch audio/subtitle markers get recalculated.
package main

import (
	"fmt"

	"vfrmap"
)

func main() {
	tl, err := vfr.BuildTimeline([]vfr.Segment{
		{SrcStart: 0, SrcEnd: vfr.TickRate * 4, Num: 2, Den: 1},
		{SrcStart: vfr.TickRate * 4, SrcEnd: vfr.TickRate * 7, Num: 1, Den: 2},
		{SrcStart: vfr.TickRate * 7, SrcEnd: vfr.TickRate * 10, Num: 1, Den: 1},
	})
	must(err)

	fmt.Printf("timeline: source %s -> output %s\n",
		fmtS(tl.SourceDuration()), fmtS(tl.TotalDuration()))

	src := vfr.SourceFrames(vfr.TickRate/2, 20) // 2 fps, 10 s source
	frames, err := vfr.Sample(tl, src, vfr.SampleConfig{
		SourceFrameInterval: 2,                // keep every 2nd frame
		MinOutputInterval:   vfr.TickRate / 4, // repeat at 4 Hz cadence
	})
	must(err)
	fmt.Printf("sampled %d source frames into %d output frames\n", len(src), len(frames))
	for i, f := range frames {
		if i >= 8 {
			fmt.Println("  ...")
			break
		}
		fmt.Printf("  src#%d pts=%s -> out pts=%s repeat=%d\n",
			f.SourceIndex, fmtS(f.SrcPTS), fmtS(f.OutPTS), f.Repeat)
	}

	mapper := vfr.NewMapper(tl)
	probe := vfr.TickRate * 2
	mapping, err := mapper.MapOutput(probe, frames)
	must(err)
	fmt.Printf("\nmap output %s back to source %s (unique=%v, boundary=%v)\n",
		fmtS(probe), fmtS(mapping.Primary.SrcPTS), mapping.Unique(), mapping.Boundary)
	for _, c := range mapping.Candidates {
		reason := c.Reason
		if reason == vfr.ReasonUnique {
			reason = "unique"
		}
		fmt.Printf("  candidate src pts=%s reason=%s\n", fmtS(c.SrcPTS), reason)
	}

	editor, err := vfr.NewEditor(tl, []string{"cut-a", "cut-b"}, []vfr.Marker{
		{ID: "audio-1", Kind: vfr.MarkerAudio, At: vfr.TickRate * 2, Length: vfr.TickRate / 2},
		{ID: "sub-1", Kind: vfr.MarkerSubtitle, At: vfr.TickRate * 5, Length: vfr.TickRate},
	})
	must(err)
	fmt.Println("\nmarkers before cut move:")
	for _, p := range editor.Recalc() {
		fmt.Printf("  %-7s src=%s -> out=%s..%s\n",
			p.Marker.ID, fmtS(p.Marker.At), fmtS(p.OutAt), fmtS(p.OutEnd))
	}

	res, err := editor.MoveCut("cut-a", vfr.TickRate*6, frames)
	must(err)
	if !res.Applied {
		fmt.Printf("cut move ambiguous, candidates: %+v\n", res.Candidates)
		return
	}
	fmt.Printf("\nmoved cut-a to source %s; markers after recalc:\n", fmtS(res.Cut.At))
	for _, p := range editor.Recalc() {
		fmt.Printf("  %-7s src=%s -> out=%s..%s\n",
			p.Marker.ID, fmtS(p.Marker.At), fmtS(p.OutAt), fmtS(p.OutEnd))
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func fmtS(t vfr.Ticks) string {
	q := float64(t) / float64(vfr.TickRate)
	return fmt.Sprintf("%.3fs", q)
}
