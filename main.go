package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
)

type report struct {
	File       string          `json:"file"`
	Fragmented bool            `json:"fragmented_mp4"`
	Tracks     []trackTimeline `json:"tracks"`
	Issues     []issue         `json:"issues,omitempty"`
}

type cliOptions struct {
	format     string
	longFactor float64
	nominal    int64
	trackID    uint
	strict     bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var opts cliOptions
	flags := flag.NewFlagSet("mp4timeline", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.format, "format", "tsv", "output format: tsv or json")
	flags.Float64Var(&opts.longFactor, "long-factor", 1.5, "display/decode threshold as a multiple of nominal stts duration")
	flags.Int64Var(&opts.nominal, "nominal", 0, "override nominal duration in media ticks")
	flags.UintVar(&opts.trackID, "track", 0, "only emit the video track with this track ID")
	flags.BoolVar(&opts.strict, "strict", false, "exit with status 2 when table inconsistencies are found")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: mp4timeline [flags] input.mp4")
		fmt.Fprintln(stderr, "Rebuild sample presentation intervals from stts/ctts/stss without reading mdat.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 1
	}
	if opts.format != "tsv" && opts.format != "json" {
		fmt.Fprintf(stderr, "invalid -format %q: want tsv or json\n", opts.format)
		return 1
	}
	if opts.longFactor <= 1 {
		fmt.Fprintf(stderr, "invalid -long-factor %v: must be greater than 1\n", opts.longFactor)
		return 1
	}
	if opts.nominal < 0 {
		fmt.Fprintln(stderr, "invalid -nominal: must not be negative")
		return 1
	}

	path := flags.Arg(0)
	m, err := parseFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "parse %s: %v\n", path, err)
		return 1
	}
	tracks := filterTracks(m.Tracks, opts.trackID)
	if len(tracks) == 0 {
		if opts.trackID != 0 {
			fmt.Fprintf(stderr, "video track %d not found\n", opts.trackID)
		} else {
			fmt.Fprintln(stderr, "no video track found")
		}
		return 1
	}

	rl := report{
		File:       path,
		Fragmented: m.Fragmented,
		Issues:     append([]issue(nil), m.Issues...),
		Tracks:     make([]trackTimeline, 0, len(tracks)),
	}
	for _, tr := range tracks {
		rl.Tracks = append(rl.Tracks, analyzeTrack(tr, timelineOptions{
			LongFactor:      opts.longFactor,
			NominalOverride: opts.nominal,
		}))
	}

	printIssues(stderr, rl)
	if opts.format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(rl); err != nil {
			fmt.Fprintf(stderr, "encode json: %v\n", err)
			return 1
		}
	} else if err := writeTSV(stdout, rl); err != nil {
		fmt.Fprintf(stderr, "write tsv: %v\n", err)
		return 1
	}
	if opts.strict && reportHasIssues(rl) {
		return 2
	}
	return 0
}

func filterTracks(tracks []*track, id uint) []*track {
	if id == 0 {
		return tracks
	}
	for _, tr := range tracks {
		if uint(tr.ID) == id {
			return []*track{tr}
		}
	}
	return nil
}

func writeTSV(w io.Writer, rl report) error {
	cw := csv.NewWriter(w)
	cw.Comma = '\t'
	header := []string{
		"track_id", "sample", "pts", "dts", "pts_seconds", "dts_seconds",
		"display_start", "display_end", "display_seconds", "display_order",
		"sync", "reason", "detail",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, tr := range rl.Tracks {
		for _, fr := range tr.Frames {
			row := []string{
				strconv.FormatUint(uint64(tr.TrackID), 10),
				strconv.Itoa(fr.Sample),
				strconv.FormatInt(fr.PTS, 10),
				strconv.FormatInt(fr.DTS, 10),
				strconv.FormatFloat(fr.PTSSeconds, 'f', 6, 64),
				strconv.FormatFloat(fr.DTSSeconds, 'f', 6, 64),
				strconv.FormatInt(fr.DisplayStart, 10),
				strconv.FormatInt(fr.DisplayEnd, 10),
				strconv.FormatFloat(fr.DisplaySeconds, 'f', 6, 64),
				strconv.Itoa(fr.DisplayOrder),
				strconv.FormatBool(fr.Sync),
				fr.Reason,
				fr.Detail,
			}
			if err := cw.Write(row); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

func printIssues(w io.Writer, rl report) {
	for _, is := range rl.Issues {
		fmt.Fprintf(w, "warning: %s: %s\n", is.Code, is.Detail)
	}
	for _, tr := range rl.Tracks {
		for _, is := range tr.Issues {
			fmt.Fprintf(w, "warning: track %d: %s: %s\n", tr.TrackID, is.Code, is.Detail)
		}
	}
}

func reportHasIssues(rl report) bool {
	if len(rl.Issues) > 0 {
		return true
	}
	for _, tr := range rl.Tracks {
		if len(tr.Issues) > 0 {
			return true
		}
	}
	return false
}
