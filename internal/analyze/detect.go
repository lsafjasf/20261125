package analyze

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// detectWithinReel finds duplicates, gaps, and timecode regressions
// inside each reel, plus identical checksums across the delivery.
func detectWithinReel(reels map[string][]*Frame, fps int) []Issue {
	var issues []Issue
	names := sortedKeys(reels)
	byMD5 := map[string][]string{}
	for _, name := range names {
		frames := reels[name]
		seen := map[int64]*Frame{}
		var prev *Frame
		for _, f := range frames {
			if f.MD5 != "" {
				byMD5[f.MD5] = append(byMD5[f.MD5], f.Header.Path)
			}
			if f.Key < 0 {
				issues = append(issues, Issue{Kind: "unparsed", Reel: name,
					Message: fmt.Sprintf("%s: no usable frame position or timecode in header", f.Header.Path)})
				continue
			}
			if prev, dup := seen[f.Key]; dup {
				f.Duplicate = true
				issues = append(issues, Issue{Kind: "duplicate", Reel: name,
					Message: fmt.Sprintf("key %d claimed by both %s and %s",
						f.Key, short(prev.Header.Path), short(f.Header.Path))})
				continue
			}
			seen[f.Key] = f
			if prev != nil {
				if prev.Key >= 0 && f.Key-prev.Key > 1 {
					issues = append(issues, Issue{Kind: "gap", Reel: name,
						Message: fmt.Sprintf("missing %d frame(s): keys %d..%d absent (between %s and %s)",
							f.Key-prev.Key-1, prev.Key+1, f.Key-1,
							short(prev.Header.Path), short(f.Header.Path))})
				}
				// Timecode regression: ordered by framepos but TC went backwards.
				if f.KeySource == "framepos" && prev.Header.HasTimeCode && f.Header.HasTimeCode {
					a := prev.Header.TimeCode.AbsFrames(fps)
					b := f.Header.TimeCode.AbsFrames(fps)
					if b < a {
						issues = append(issues, Issue{Kind: "tc-regression", Reel: name,
							Message: fmt.Sprintf("timecode regresses %s -> %s between %s and %s",
								prev.Header.TimeCode, f.Header.TimeCode,
								short(prev.Header.Path), short(f.Header.Path))})
					}
				}
			}
			prev = f
		}
	}
	for sum, paths := range byMD5 {
		if len(paths) > 1 {
			issues = append(issues, Issue{Kind: "duplicate",
				Message: fmt.Sprintf("identical md5 %s in %d files: %s", sum, len(paths), strings.Join(paths, ", "))})
		}
	}
	return issues
}

// detectResolution reports resolution changes across the delivery.
func detectResolution(frames []*Frame) []Issue {
	type res struct{ w, h uint32 }
	byRes := map[res][]string{}
	for _, f := range frames {
		r := res{f.Header.Width, f.Header.Height}
		byRes[r] = append(byRes[r], f.Header.Path)
	}
	if len(byRes) <= 1 {
		return nil
	}
	var parts []string
	for r, paths := range byRes {
		parts = append(parts, fmt.Sprintf("%dx%d (%d frames, e.g. %s)", r.w, r.h, len(paths), short(paths[0])))
	}
	sort.Strings(parts)
	return []Issue{{Kind: "resolution-change",
		Message: "mixed resolutions: " + strings.Join(parts, "; ")}}
}

// orderReels chains reels by timecode continuity; reels that cannot be
// chained fall back to name order with an explicit heuristic warning.
func orderReels(reels map[string][]*Frame, explicit []string, fps int, issues *[]Issue) []string {
	if len(explicit) > 0 {
		var order []string
		in := map[string]bool{}
		for _, r := range explicit {
			if _, ok := reels[r]; ok && !in[r] {
				order = append(order, r)
				in[r] = true
			}
		}
		for _, r := range sortedKeys(reels) {
			if !in[r] {
				order = append(order, r)
			}
		}
		return order
	}
	type span struct {
		name       string
		start, end int64
	}
	var spans []span
	for name, frames := range reels {
		lo, hi := int64(-1), int64(-1)
		for _, f := range frames {
			if f.Key < 0 {
				continue
			}
			if lo < 0 || f.Key < lo {
				lo = f.Key
			}
			if f.Key > hi {
				hi = f.Key
			}
		}
		spans = append(spans, span{name, lo, hi})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].name < spans[j].name })

	// Greedy chaining: pick the lowest-start unchained reel, then follow
	// the reel whose start continues this reel's end (within 2 frames).
	used := map[string]bool{}
	var order []string
	remaining := func() *span {
		var best *span
		for i := range spans {
			if used[spans[i].name] || spans[i].start < 0 {
				continue
			}
			if best == nil || spans[i].start < best.start {
				best = &spans[i]
			}
		}
		return best
	}
	for {
		cur := remaining()
		if cur == nil {
			break
		}
		order = append(order, cur.name)
		used[cur.name] = true
		for {
			var next *span
			for i := range spans {
				if used[spans[i].name] || spans[i].start < 0 {
					continue
				}
				if d := spans[i].start - cur.end; d >= 1 && d <= 2 {
					if next == nil || spans[i].start < next.start {
						next = &spans[i]
					}
				}
			}
			if next == nil {
				break
			}
			order = append(order, next.name)
			used[next.name] = true
			cur = next
		}
	}
	// Leftover reels (no chainable timecodes): append by name, flagged.
	var rest []string
	for _, s := range spans {
		if !used[s.name] {
			rest = append(rest, s.name)
		}
	}
	if len(rest) > 0 {
		*issues = append(*issues, Issue{Kind: "reel-order-heuristic",
			Message: fmt.Sprintf("reel(s) %s could not be chained by timecode; appended in name order — verify manually",
				strings.Join(rest, ", "))})
		order = append(order, rest...)
	}
	return order
}

// buildRanges splits the global order into playable contiguous runs of
// constant resolution, skipping duplicate frames.
func buildRanges(global []*Frame) []Range {
	var ranges []Range
	var prev *Frame
	for _, f := range global {
		if f.Duplicate || f.Key < 0 {
			continue
		}
		contiguous := prev != nil && f.Key == prev.Key+1 &&
			f.Header.Width == prev.Header.Width && f.Header.Height == prev.Header.Height
		if !contiguous {
			ranges = append(ranges, Range{Width: f.Header.Width, Height: f.Header.Height})
		}
		r := &ranges[len(ranges)-1]
		r.Frames = append(r.Frames, f)
		prev = f
	}
	return ranges
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func short(path string) string {
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return path
}
