package audit

import (
	"fmt"

	"cutmux/internal/media"
)

type sequenceItem struct {
	hash  uint64
	start int64
	end   int64
}

func alignTracks(a, b *media.Track) (Alignment, []FrameRun, []FrameRun) {
	var out Alignment
	if a == nil || b == nil {
		out.Reason = "track missing"
		return out, nil, nil
	}
	as := presentedSequence(a)
	bs := presentedSequence(b)
	if len(as) == 0 || len(bs) == 0 {
		out.Reason = "one track has no presented samples"
		return out, nil, nil
	}
	if !discriminative(as) || !discriminative(bs) {
		out.Reason = "coded payload hashes are not discriminative"
		return out, nil, nil
	}
	matchesA, matchesB := alignHashes(as, bs)
	onlyA := complement(len(as), matchesA)
	onlyB := complement(len(bs), matchesB)
	out.Common = len(matchesA)
	out.OnlyA = len(onlyA)
	out.OnlyB = len(onlyB)
	out.OnlyARuns = indexRuns(onlyA)
	out.OnlyBRuns = indexRuns(onlyB)
	maxLen := max(len(as), len(bs))
	out.Comparable = out.Common > 0 && out.Common*10 >= maxLen
	if !out.Comparable {
		out.Reason = fmt.Sprintf("only %d common coded payloads in a %d/%d sequence", out.Common, len(as), len(bs))
	}
	return out, frameRuns(out.OnlyARuns, as), frameRuns(out.OnlyBRuns, bs)
}

func presentedSequence(t *media.Track) []sequenceItem {
	ps := t.PresentedSamples()
	out := make([]sequenceItem, 0, len(ps))
	for _, s := range ps {
		out = append(out, sequenceItem{hash: s.Hash, start: s.Start, end: s.End})
	}
	return out
}

func discriminative(s []sequenceItem) bool {
	if len(s) == 0 {
		return false
	}
	first := s[0].hash
	for _, item := range s[1:] {
		if item.hash != first {
			return true
		}
	}
	return len(s) == 1 && first != 0
}

func alignHashes(a, b []sequenceItem) (map[int]bool, map[int]bool) {
	ma := make(map[int]bool)
	mb := make(map[int]bool)
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix].hash == b[prefix].hash {
		ma[prefix], mb[prefix] = true, true
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix &&
		a[len(a)-1-suffix].hash == b[len(b)-1-suffix].hash {
		ma[len(a)-1-suffix], mb[len(b)-1-suffix] = true, true
		suffix++
	}
	am, bm := a[prefix:len(a)-suffix], b[prefix:len(b)-suffix]
	if len(am) == 0 || len(bm) == 0 {
		return ma, mb
	}
	if len(am)*len(bm) <= 4_000_000 {
		lcsExact(am, bm, prefix, prefix, ma, mb)
	} else {
		lcsGreedy(am, bm, prefix, prefix, ma, mb)
	}
	return ma, mb
}

func lcsExact(a, b []sequenceItem, offA, offB int, ma, mb map[int]bool) {
	n, m := len(a), len(b)
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if a[i-1].hash == b[j-1].hash {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	i, j := n, m
	for i > 0 && j > 0 {
		switch {
		case a[i-1].hash == b[j-1].hash:
			ma[offA+i-1], mb[offB+j-1] = true, true
			i--
			j--
		case dp[i-1][j] >= dp[i][j-1]:
			i--
		default:
			j--
		}
	}
}

func lcsGreedy(a, b []sequenceItem, offA, offB int, ma, mb map[int]bool) {
	byHash := make(map[uint64][]int)
	for j, item := range b {
		byHash[item.hash] = append(byHash[item.hash], j)
	}
	next := 0
	for i, item := range a {
		list := byHash[item.hash]
		k := lowerBound(list, next)
		if k >= len(list) {
			continue
		}
		j := list[k]
		ma[offA+i], mb[offB+j] = true, true
		next = j + 1
	}
}

func lowerBound(a []int, v int) int {
	lo, hi := 0, len(a)
	for lo < hi {
		mid := (lo + hi) / 2
		if a[mid] < v {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func complement(n int, matched map[int]bool) []int {
	out := make([]int, 0, n-len(matched))
	for i := 0; i < n; i++ {
		if !matched[i] {
			out = append(out, i)
		}
	}
	return out
}

func indexRuns(indices []int) []IndexRun {
	if len(indices) == 0 {
		return nil
	}
	out := []IndexRun{{Start: indices[0], End: indices[0] + 1}}
	for _, idx := range indices[1:] {
		last := &out[len(out)-1]
		if idx == last.End {
			last.End++
		} else {
			out = append(out, IndexRun{Start: idx, End: idx + 1})
		}
	}
	return out
}

func frameRuns(runs []IndexRun, seq []sequenceItem) []FrameRun {
	out := make([]FrameRun, 0, len(runs))
	for _, r := range runs {
		if r.Start < 0 || r.End > len(seq) || r.Start >= r.End {
			continue
		}
		fr := FrameRun{
			StartIndex: r.Start, EndIndex: r.End,
			StartMs: ms(seq[r.Start].start), EndMs: ms(seq[r.End-1].end),
			Frames: r.End - r.Start,
		}
		for i := r.Start; i < r.End; i++ {
			fr.Hashes = append(fr.Hashes, fmt.Sprintf("%016x", seq[i].hash))
		}
		out = append(out, fr)
	}
	return out
}
