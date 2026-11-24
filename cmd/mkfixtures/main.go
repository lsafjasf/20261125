// Command mkfixtures creates the three corruption sample groups from healthy
// ffmpeg generated source files: truncated.mp4, duplicate.mp4 and
// badoffsets.mp4. It never modifies its sources.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mp4index/fixture"
	"mp4index/movie"
	"mp4index/repair"
)

func main() {
	synthetic := flag.Bool("synthetic", false, "generate the three groups from the deterministic Go fixture (no ffmpeg/sources needed)")
	fast := flag.String("fast", "samples/source-fast.mp4", "faststart MP4 used for the truncation group")
	late := flag.String("late", "samples/source.mp4", "moov-last MP4 used for duplicate/offset groups")
	dir := flag.String("dir", "samples/cases", "directory for corrupted samples")
	flag.Parse()

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		fatal(err)
	}

	if *synthetic {
		buildSynthetic(*dir)
		return
	}

	// 1) Truncation: cut the faststart file at 70% of its mdat payload.
	cut, err := mdatFractionCut(*fast, 0.7)
	if err != nil {
		fatal(err)
	}
	truncPath := filepath.Join(*dir, "truncated.mp4")
	if err := fixture.TruncateCopy(*fast, truncPath, cut); err != nil {
		fatal(err)
	}
	fmt.Println("truncated :", truncPath, "cut at", cut)

	// 2) Wrong offsets: shift every stco entry.
	badPath := filepath.Join(*dir, "badoffsets.mp4")
	n, err := fixture.OffsetAllSTCO(*late, badPath, 8192)
	if err != nil {
		fatal(err)
	}
	fmt.Println("badoffsets:", badPath, "patched entries", n)

	// 3) Duplicate records: normalize to one sample per chunk moov-last via
	//    the repair writer, then insert duplicated stsz/stco records.
	base := filepath.Join(*dir, ".base-one-per-chunk.mp4")
	if _, err := repair.RepairFile(*late, base, repair.Options{MoovLast: true}); err != nil {
		fatal(err)
	}
	dupPath := filepath.Join(*dir, "duplicate.mp4")
	if err := fixture.DuplicateIndexRecords(base, dupPath, []int{3, 8, 14, 21}); err != nil {
		fatal(err)
	}
	_ = os.Remove(base)
	fmt.Println("duplicate :", dupPath)
}

func buildSynthetic(dir string) {
	// Truncation: healthy faststart file cut mid sample #15.
	fastPath := filepath.Join(dir, ".syn-fast.mp4")
	g, err := fixture.Build(fixture.Config{Path: fastPath, Frames: 24, GOP: 8, MoovFirst: true, WithAUD: true})
	if err != nil {
		fatal(err)
	}
	cutAt := g.Samples[14].Offset + int64(g.Samples[14].Size/2)
	truncPath := filepath.Join(dir, "synthetic-truncated.mp4")
	if err := fixture.TruncateCopy(fastPath, truncPath, cutAt); err != nil {
		fatal(err)
	}
	fmt.Println("truncated :", truncPath)

	// Duplicate index records.
	dupPath := filepath.Join(dir, "synthetic-duplicate.mp4")
	if _, err := fixture.Build(fixture.Config{
		Path: dupPath, Frames: 24, GOP: 8, MoovFirst: true, WithAUD: true,
		DupAfter: []int{3, 10},
	}); err != nil {
		fatal(err)
	}
	fmt.Println("duplicate :", dupPath)

	// Wrong offsets.
	healthy := filepath.Join(dir, ".syn-healthy.mp4")
	if _, err := fixture.Build(fixture.Config{Path: healthy, Frames: 24, GOP: 8, MoovFirst: true}); err != nil {
		fatal(err)
	}
	badPath := filepath.Join(dir, "synthetic-badoffsets.mp4")
	if _, err := fixture.OffsetAllSTCO(healthy, badPath, 97); err != nil {
		fatal(err)
	}
	fmt.Println("badoffsets:", badPath)
	_ = os.Remove(fastPath)
	_ = os.Remove(healthy)
}

func mdatFractionCut(path string, frac float64) (int64, error) {
	f, info, err := movie.OpenAndParse(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if len(info.Mdats) == 0 {
		return 0, fmt.Errorf("%s has no mdat", path)
	}
	sec := info.Mdats[0]
	return sec.PayloadOffset + int64(float64(sec.PayloadLength)*frac), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "mkfixtures:", err)
	os.Exit(1)
}
