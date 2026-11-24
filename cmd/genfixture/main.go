package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"aes3check/aes3"
)

type file struct {
	Description string      `json:"description"`
	TimeUnit    string      `json:"time_unit"`
	HalfUnit    float64     `json:"half_unit"`
	Edges       []aes3.Edge `json:"edges"`
}

func main() {
	out := flag.String("out", "testdata", "directory for generated JSON fixtures")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir string) error {
	specs := make([]aes3.SubframeInput, 0, 10)
	for frame := 0; frame < 5; frame++ {
		leftCode := "X"
		leftStatus := frame%2 == 0
		if frame == 0 {
			leftCode = "Z"
			leftStatus = true
		}
		specs = append(specs,
			aes3.SubframeInput{Code: leftCode, Sample24: int32(1000 + frame*17), ChannelStatusBit: leftStatus, UserBit: frame%3 == 0},
			aes3.SubframeInput{Code: "Y", Sample24: int32(-2000 - frame*19), ChannelStatusBit: frame%2 == 1, UserBit: frame%4 == 0},
		)
	}
	clean := aes3.EncodeEdges(specs)

	loss := make([]aes3.Edge, 0, len(clean))
	for _, edge := range clean {
		if edge.Time >= 4*64 && edge.Time < 5*64 {
			continue
		}
		loss = append(loss, edge)
	}

	files := map[string]file{
		"sample-edges.json": {
			Description: "Five synthetic AES3 frames encoded as BMC edges in half-UI units.",
			TimeUnit:    "half_ui",
			HalfUnit:    1,
			Edges:       clean,
		},
		"loss-edges.json": {
			Description: "Subframe 4 edges removed; the following Y preamble relocks.",
			TimeUnit:    "half_ui",
			HalfUnit:    1,
			Edges:       loss,
		},
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, doc := range files {
		raw, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		raw = append(raw, '\n')
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return err
		}
		fmt.Println(path)
	}
	return nil
}
