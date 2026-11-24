package aes3

import "math"

// selectCandidates builds one ordered, non-overlapping decode chain. It starts
// at the earliest strong candidate and bridges only to an adjacent expected
// subframe when possible. A later preamble starts a new independent chain via
// the lock-interval logic in Decode.
func selectCandidates(candidates []candidate, h float64) []candidate {
	if len(candidates) == 0 {
		return nil
	}

	chosen := make([]candidate, 0, len(candidates))
	cur := 0
	used := map[int]bool{0: true}
	chosen = append(chosen, candidates[0])

	for {
		next := chooseAdjacent(candidates, cur, used, h)
		if next < 0 {
			next = chooseRelock(candidates, cur, used, h)
		}
		if next < 0 {
			break
		}
		used[next] = true
		chosen = append(chosen, candidates[next])
		cur = next
	}
	return chosen
}

func chooseAdjacent(cs []candidate, cur int, used map[int]bool, h float64) int {
	start := cs[cur].subframe.StartTime
	want := start + 64*h
	window := 1.0 * h
	best := -1
	bestRank := math.Inf(1)

	for i := cur + 1; i < len(cs); i++ {
		if used[i] {
			continue
		}
		got := cs[i].subframe.StartTime
		if got < want-window {
			continue
		}
		if got > want+window {
			break
		}
		rank := math.Abs(got-want) / h
		if !expectedNextChannel(cs[cur].subframe.Channel, cs[i].subframe.Channel) {
			rank += 10
		}
		rank += (1 - cs[i].score) * 100
		if rank < bestRank {
			bestRank = rank
			best = i
		}
	}
	// A wrong-sequence candidate in the window is not good enough to claim
	// adjacency; let relock search take the next non-overlapping candidate.
	if best >= 0 && bestRank >= 10 {
		return -1
	}
	return best
}

func chooseRelock(cs []candidate, cur int, used map[int]bool, h float64) int {
	earliest := cs[cur].subframe.StartTime + 63*h
	for i := cur + 1; i < len(cs); i++ {
		if used[i] {
			continue
		}
		if cs[i].subframe.StartTime >= earliest {
			return i
		}
	}
	return -1
}

func expectedNextChannel(current, next string) bool {
	switch current {
	case "left":
		return next == "right"
	case "right":
		return next == "left"
	default:
		return false
	}
}

func adjacentSubframes(a, b Subframe, h float64) bool {
	delta := b.StartTime - a.StartTime
	return math.Abs(delta-64*h) <= h && expectedNextChannel(a.Channel, b.Channel)
}
