// Package analyze reconstructs the global frame order of a DPX delivery
// from image headers and manifests, and detects ordering anomalies.
package analyze

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"dpxscan/internal/dpx"
)

// Frame is one scanned DPX frame with its derived ordering key.
type Frame struct {
	Header    dpx.Header
	Reel      string // delivery reel/volume (parent directory)
	MD5       string // checksum from manifest, if any
	Key       int64  // ordering key within the reel
	KeySource string // "timecode" or "framepos"
	Duplicate bool   // a previous frame already occupies this key
}

// Issue describes one detected anomaly.
type Issue struct {
	Kind    string // duplicate, gap, tc-regression, resolution-change, manifest-missing, unparsed, reel-order-heuristic
	Reel    string
	Message string
}

// Range is a maximal run of contiguous, same-resolution frames in
// global order — i.e. a directly playable segment.
type Range struct {
	Frames []*Frame
	Width  uint32
	Height uint32
}

// Report is the full analysis result.
type Report struct {
	FPS         int
	Frames      []*Frame // global order, duplicates included
	ReelOrder   []string
	Ranges      []Range
	Issues      []Issue
	Suggestions []string
}

// Options controls Scan.
type Options struct {
	FPS       int      // frames per second for timecode math (0 = auto from headers)
	ReelOrder []string // explicit reel order; overrides chaining heuristics
	Manifests []string // manifest file paths
	VerifyMD5 bool     // hash files and compare against manifest
}

// Scan walks root, parses every DPX header, applies manifests, and
// reconstructs the global order without consulting file names.
func Scan(root string, opt Options) (*Report, error) {
	man, err := loadManifests(opt.Manifests)
	if err != nil {
		return nil, err
	}
	frames, issues, err := collectFrames(root, man, opt)
	if err != nil {
		return nil, err
	}
	rep := &Report{Issues: issues}
	rep.FPS = pickFPS(frames, opt.FPS)

	reels := groupByReel(frames)
	assignKeys(reels, rep.FPS)
	for _, r := range reels {
		sortReel(r)
	}
	rep.Issues = append(rep.Issues, detectWithinReel(reels, rep.FPS)...)
	rep.Issues = append(rep.Issues, detectResolution(frames)...)

	order := orderReels(reels, opt.ReelOrder, rep.FPS, &rep.Issues)
	rep.ReelOrder = order
	for _, name := range order {
		rep.Frames = append(rep.Frames, reels[name]...)
	}
	rep.Ranges = buildRanges(rep.Frames)
	rep.Suggestions = suggest(rep)
	return rep, nil
}

func loadManifests(paths []string) (*Manifest, error) {
	merged := &Manifest{Entries: map[string]ManifestEntry{}}
	for _, p := range paths {
		m, err := LoadManifest(p)
		if err != nil {
			return nil, fmt.Errorf("manifest %s: %w", p, err)
		}
		for k, v := range m.Entries {
			merged.Entries[k] = v
		}
	}
	return merged, nil
}

func collectFrames(root string, man *Manifest, opt Options) ([]*Frame, []Issue, error) {
	var frames []*Frame
	var issues []Issue
	matched := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !dpx.IsDPX(path) {
			return err
		}
		h, perr := dpx.ParseFile(path)
		if perr != nil {
			issues = append(issues, Issue{Kind: "unparsed", Message: perr.Error()})
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		reel := filepath.ToSlash(rel)
		if reel == "." {
			reel = "(root)"
		}
		fr := &Frame{Header: h, Reel: reel}
		if e, ok := man.Lookup(path); ok {
			fr.MD5 = e.MD5
			matched[cleanKey(e.Path)] = true
			if opt.VerifyMD5 && e.MD5 != "" {
				if sum, herr := hashFile(path); herr == nil && sum != e.MD5 {
					issues = append(issues, Issue{Kind: "checksum-mismatch", Reel: reel,
						Message: fmt.Sprintf("%s: md5 %s != manifest %s", path, sum, e.MD5)})
				}
			}
		}
		frames = append(frames, fr)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	for k, e := range man.Entries {
		if !matched[k] && strings.HasSuffix(strings.ToLower(k), ".dpx") {
			issues = append(issues, Issue{Kind: "manifest-missing",
				Message: fmt.Sprintf("manifest lists %s but no matching file was scanned", e.Path)})
		}
	}
	return frames, issues, nil
}

func pickFPS(frames []*Frame, forced int) int {
	if forced > 0 {
		return forced
	}
	for _, f := range frames {
		if f.Header.FrameRate > 0 {
			return int(f.Header.FrameRate + 0.5)
		}
	}
	return 24
}

func groupByReel(frames []*Frame) map[string][]*Frame {
	m := map[string][]*Frame{}
	for _, f := range frames {
		m[f.Reel] = append(m[f.Reel], f)
	}
	return m
}

// assignKeys picks a per-reel ordering key from header data only.
func assignKeys(reels map[string][]*Frame, fps int) {
	for _, frames := range reels {
		nTC, nPos := 0, 0
		for _, f := range frames {
			if f.Header.HasTimeCode {
				nTC++
			}
			if f.Header.HasFramePos {
				nPos++
			}
		}
		src := "framepos"
		if nTC > 0 && nTC >= nPos {
			src = "timecode"
		}
		for _, f := range frames {
			f.KeySource = src
			if src == "timecode" && f.Header.HasTimeCode {
				f.Key = int64(f.Header.TimeCode.AbsFrames(fps))
			} else if f.Header.HasFramePos {
				f.Key = int64(f.Header.FramePos)
			} else {
				f.Key = -1
			}
		}
	}
}

func sortReel(frames []*Frame) {
	sort.SliceStable(frames, func(i, j int) bool {
		if frames[i].Key != frames[j].Key {
			return frames[i].Key < frames[j].Key
		}
		return frames[i].Header.Path < frames[j].Header.Path // deterministic tiebreak only
	})
}
