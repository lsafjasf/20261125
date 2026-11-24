package analyze

import (
	"fmt"
	"sort"

	"tearcheck/internal/detect"
)

type eventMember struct {
	frameIndex int
	seamIndex  int
	seam       detect.Seam
}

type eventWork struct {
	index      int
	id         string
	members    []eventMember
	ordinals   []int
	continuity string
	lastOrd    int
	lastRow    int
	lastOffset int
}

type eventBundle struct {
	events    []*eventWork
	seamEvent [][]int
}

func buildEvents(frames []loadedFrame, settings detect.Settings) eventBundle {
	seamEvent := make([][]int, len(frames))
	for index := range frames {
		seamEvent[index] = make([]int, len(frames[index].evidence.Seams))
		for seamIndex := range seamEvent[index] {
			seamEvent[index][seamIndex] = -1
		}
	}

	var events []*eventWork
	var active []*eventWork
	ordinal := -1
	for frameIndex := range frames {
		if frames[frameIndex].info.Duplicate {
			continue
		}
		ordinal++
		active = filterActive(active, ordinal, settings.MaxEventGapFrames)
		seams := frames[frameIndex].evidence.Seams
		if len(seams) == 0 {
			continue
		}

		order := make([]int, len(seams))
		for index := range order {
			order[index] = index
		}
		sort.Slice(order, func(i, j int) bool {
			left, right := seams[order[i]], seams[order[j]]
			if left.Row != right.Row {
				return left.Row < right.Row
			}
			if left.MatchScoreBP != right.MatchScoreBP {
				return left.MatchScoreBP > right.MatchScoreBP
			}
			return order[i] < order[j]
		})

		assigned := make(map[*eventWork]bool, len(active))
		for _, seamIndex := range order {
			seam := seams[seamIndex]
			var best *eventWork
			bestCost := int(^uint(0) >> 1)
			for _, candidate := range active {
				if assigned[candidate] {
					continue
				}
				gap := ordinal - candidate.lastOrd
				if gap < 0 || gap > settings.MaxEventGapFrames {
					continue
				}
				rowJump := absInt(seam.Row - candidate.lastRow)
				shiftJump := absInt(seam.OffsetPX - candidate.lastOffset)
				if rowJump > settings.MaxRowJumpPX || shiftJump > settings.MaxShiftJumpPX {
					continue
				}
				cost := gap*100 + rowJump*5 + shiftJump*7
				if cost < bestCost {
					best = candidate
					bestCost = cost
				}
			}
			if best == nil {
				best = &eventWork{
					index:      len(events),
					members:    []eventMember{},
					ordinals:   []int{},
					lastOrd:    ordinal,
					lastRow:    seam.Row,
					lastOffset: seam.OffsetPX,
				}
				events = append(events, best)
				active = append(active, best)
			}
			best.members = append(best.members, eventMember{
				frameIndex: frameIndex,
				seamIndex:  seamIndex,
				seam:       seam,
			})
			best.ordinals = append(best.ordinals, ordinal)
			best.lastOrd = ordinal
			best.lastRow = seam.Row
			best.lastOffset = seam.OffsetPX
			assigned[best] = true
			seamEvent[frameIndex][seamIndex] = best.index
		}
	}

	for index, event := range events {
		event.id = fmt.Sprintf("EVT-%06d", index+1)
		event.continuity = classifyContinuity(event.ordinals, settings.MinContinuousFrames)
	}
	return eventBundle{events: events, seamEvent: seamEvent}
}

func filterActive(active []*eventWork, ordinal, maxGap int) []*eventWork {
	kept := active[:0]
	for _, event := range active {
		if ordinal-event.lastOrd <= maxGap {
			kept = append(kept, event)
		}
	}
	return kept
}

func classifyContinuity(ordinals []int, minContinuous int) string {
	if len(ordinals) <= 1 {
		return "isolated"
	}
	consecutive := true
	for index := 1; index < len(ordinals); index++ {
		if ordinals[index]-ordinals[index-1] != 1 {
			consecutive = false
			break
		}
	}
	if consecutive && len(ordinals) >= minContinuous {
		return "continuous"
	}
	return "intermittent"
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
