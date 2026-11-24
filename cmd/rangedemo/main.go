package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"rangeaudit/internal/audit"
)

type fixture struct {
	id string
	fn func(index int) uint8
}

func main() {
	outDir := flag.String("out", "artifacts/demo", "output directory")
	flag.Parse()
	if err := generate(*outDir); err != nil {
		fmt.Fprintln(os.Stderr, "rangedemo:", err)
		os.Exit(1)
	}
}

func generate(outDir string) error {
	fixtures := []fixture{
		{"synthetic-limited-hard-rails", limitedRails},
		{"synthetic-ambiguous-inside", ambiguousInside},
		{"synthetic-full-lifted", fullLifted},
		{"synthetic-full-low-key", fullLowKey},
	}
	fixtureDir := filepath.Join(outDir, "fixtures")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		return err
	}
	m := audit.Manifest{Defaults: audit.ClipSpec{Declared: audit.DeclaredFull, MaxSamples: 2_000_000}}
	for _, f := range fixtures {
		path := filepath.Join(fixtureDir, f.id+".png")
		if err := writePNG(path, 640, 360, f.fn); err != nil {
			return err
		}
		m.Clips = append(m.Clips, audit.ClipSpec{ID: f.id, Path: filepath.Join("fixtures", f.id+".png")})
	}
	manifestPath := filepath.Join(outDir, "manifest.json")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return err
	}
	specs, baseDir, err := audit.LoadManifest(manifestPath)
	if err != nil {
		return err
	}
	report := audit.Run(specs, baseDir, 2_000_000)
	if err := writeFile(filepath.Join(outDir, "report.json"), func(f *os.File) error {
		return audit.WriteJSON(f, report)
	}); err != nil {
		return err
	}
	return writeFile(filepath.Join(outDir, "report.md"), func(f *os.File) error {
		return audit.WriteMarkdown(f, report)
	})
}

func writePNG(path string, width, height int, valueAt func(int) uint8) error {
	img := image.NewGray(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := valueAt(y*width + x)
			img.SetGray(x, y, color.Gray{Y: v})
		}
	}
	return writeFile(path, func(f *os.File) error {
		return png.Encode(f, img)
	})
}

func writeFile(path string, fn func(*os.File) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return fn(f)
}

func limitedRails(i int) uint8 {
	switch {
	case i%5 == 0:
		return 16
	case i%7 == 0:
		return 235
	default:
		return uint8(16 + (i*37)%220)
	}
}

func ambiguousInside(i int) uint8 {
	return uint8(20 + (i*37)%201)
}

func fullLifted(i int) uint8 {
	return uint8(32 + (i*53)%224)
}

func fullLowKey(i int) uint8 {
	if i%97 == 0 {
		return 245
	}
	return uint8(16 + (i*17)%83)
}
