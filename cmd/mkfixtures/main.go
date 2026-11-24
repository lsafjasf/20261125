package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"editlist-audit/internal/fixture"
)

func main() {
	outDir := flag.String("out", "testdata/cases", "directory for generated read-only audit fixtures")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create fixture directory: %v\n", err)
		os.Exit(1)
	}

	for _, cfg := range cases() {
		data := fixture.BuildMOV(cfg)
		path := filepath.Join(*outDir, cfg.FileName)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(data))
	}
}

func baseCase(name string, movieTS, mediaTS, duration uint32, edits ...fixture.Edit) fixture.Config {
	return fixture.Config{
		FileName:       name,
		MovieTimescale: movieTS,
		MovieDuration:  duration,
		TrackID:        1,
		TrackDuration:  duration,
		MediaTimescale: mediaTS,
		MediaDuration:  mediaTS, // one second of PCM frames
		PCMFrames:      int(mediaTS),
		ToneHz:         440,
		Edits:          edits,
	}
}

func cases() []fixture.Config {
	normal := uint32(0x00010000)
	double := uint32(0x00020000)

	return []fixture.Config{
		baseCase("01_empty_edit_delay.mov", 1000, 48000, 1250,
			fixture.Edit{Duration: 250, MediaTime: -1, Rate: normal},
			fixture.Edit{Duration: 1000, MediaTime: 0, Rate: normal},
		),
		baseCase("02_negative_origin_shift.mov", 1000, 44100, 800,
			fixture.Edit{Duration: 800, MediaTime: 8820, Rate: normal},
		),
		baseCase("03_negative_media_time_field.mov", 1000, 48000, 800,
			fixture.Edit{Duration: 800, MediaTime: -2, Rate: normal},
		),
		baseCase("04_zero_rate_dwell.mov", 1000, 48000, 1000,
			fixture.Edit{Duration: 500, MediaTime: 12000, Rate: 0},
			fixture.Edit{Duration: 500, MediaTime: 24000, Rate: normal},
		),
		baseCase("05_cross_timescale_600_to_22050.mov", 600, 22050, 250,
			fixture.Edit{Duration: 250, MediaTime: 0, Rate: normal},
		),
		baseCase("06_media_rate_2x.mov", 1000, 48000, 500,
			fixture.Edit{Duration: 500, MediaTime: 0, Rate: double},
		),
	}
}
