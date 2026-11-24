package main

import (
	"fmt"
	"io"
	"strings"
)

func candidateMappings(channels int) [][]int {
	if channels > 6 {
		candidates := [][]int{identityMapping(channels)}
		for i := 0; i+1 < channels; i++ {
			mapping := identityMapping(channels)
			mapping[i], mapping[i+1] = mapping[i+1], mapping[i]
			candidates = append(candidates, mapping)
		}
		return candidates
	}
	values := identityMapping(channels)
	results := [][]int{append([]int(nil), values...)}
	var generate func(int)
	generate = func(start int) {
		if start == len(values) {
			candidate := append([]int(nil), values...)
			if !isIdentity(candidate) {
				results = append(results, candidate)
			}
			return
		}
		for i := start; i < len(values); i++ {
			values[start], values[i] = values[i], values[start]
			generate(start + 1)
			values[start], values[i] = values[i], values[start]
		}
	}
	generate(0)
	return results
}

func formatMapping(mapping []int) string {
	parts := make([]string, len(mapping))
	for i, value := range mapping {
		parts[i] = fmt.Sprintf("%d", value+1)
	}
	return strings.Join(parts, ",")
}

func printMappingCandidates(out io.Writer, names []string) {
	fmt.Fprintln(out, "\n可选映射（输出位置 <- 源位置）:")
	for i, candidate := range candidateMappings(len(names)) {
		description := make([]string, len(candidate))
		for outputPosition, sourcePosition := range candidate {
			description[outputPosition] = fmt.Sprintf("%s<-实际位置%d", names[outputPosition], sourcePosition+1)
		}
		fmt.Fprintf(out, "  %d. %s (%s)\n", i+1, formatMapping(candidate), strings.Join(description, " "))
	}
}
