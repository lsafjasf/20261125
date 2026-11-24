package main

import (
	"fmt"
	"math/big"
)

var oneDay = big.NewRat(24*60*60*1000, 1)

func validateCues(cues []*Cue, stage int) []Issue {
	var issues []Issue
	seenStart := make(map[string]int)
	seenEnd := make(map[string]int)
	seenRange := make(map[string]int)

	for _, cue := range cues {
		startKey := cue.Start.RatString()
		endKey := cue.End.RatString()
		rangeKey := startKey + "->" + endKey

		if previous, ok := seenStart[startKey]; ok {
			issues = append(issues, Issue{
				Stage: stage, CueIndex: cue.Index, Kind: "duplicate_start", Severity: "warning",
				Message: fmt.Sprintf("duplicate in point with cue %d at %s", previous, startKey),
			})
		}
		if previous, ok := seenEnd[endKey]; ok {
			issues = append(issues, Issue{
				Stage: stage, CueIndex: cue.Index, Kind: "duplicate_end", Severity: "warning",
				Message: fmt.Sprintf("duplicate out point with cue %d at %s", previous, endKey),
			})
		}
		if previous, ok := seenRange[rangeKey]; ok {
			issues = append(issues, Issue{
				Stage: stage, CueIndex: cue.Index, Kind: "duplicate_range", Severity: "warning",
				Message: fmt.Sprintf("duplicate in/out range with cue %d (%s)", previous, rangeKey),
			})
		}
		seenStart[startKey] = cue.Index
		seenEnd[endKey] = cue.Index
		seenRange[rangeKey] = cue.Index

		duration := new(big.Rat).Sub(cue.End, cue.Start)
		if duration.Sign() < 0 {
			issues = append(issues, Issue{
				Stage: stage, CueIndex: cue.Index, Kind: "end_before_start", Severity: "error",
				Message: fmt.Sprintf("out point is %s ms before in point", duration.RatString()),
			})
		} else if duration.Sign() == 0 {
			issues = append(issues, Issue{
				Stage: stage, CueIndex: cue.Index, Kind: "zero_duration", Severity: "error",
				Message: "in point and out point are identical",
			})
		}

		if cue.Start.Cmp(oneDay) >= 0 {
			issues = append(issues, Issue{
				Stage: stage, CueIndex: cue.Index, Kind: "start_crosses_midnight", Severity: "warning",
				Message: fmt.Sprintf("in point reaches or passes 24:00:00,000 at %s", startKey),
			})
		}
		if cue.End.Cmp(oneDay) >= 0 {
			issues = append(issues, Issue{
				Stage: stage, CueIndex: cue.Index, Kind: "end_crosses_midnight", Severity: "warning",
				Message: fmt.Sprintf("out point reaches or passes 24:00:00,000 at %s", endKey),
			})
		}
	}
	return issues
}

func hasError(issues []Issue) bool {
	for _, issue := range issues {
		if issue.Severity == "error" {
			return true
		}
	}
	return false
}
