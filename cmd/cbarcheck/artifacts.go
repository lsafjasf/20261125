package main

import (
	"fmt"
	"os"
	"path/filepath"

	"cbarcheck"
)

type artifactPaths struct {
	JSON string
	MD   string
}

func writeArtifacts(dir, prefix string, report *cbarcheck.Report, title string) (artifactPaths, error) {
	var paths artifactPaths
	if dir == "" {
		return paths, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return paths, err
	}
	paths.JSON = filepath.Join(dir, prefix+"-report.json")
	paths.MD = filepath.Join(dir, prefix+"-report.md")
	jf, err := os.Create(paths.JSON)
	if err != nil {
		return paths, err
	}
	if err := cbarcheck.WriteJSON(jf, report); err != nil {
		jf.Close()
		return paths, err
	}
	if err := jf.Close(); err != nil {
		return paths, err
	}
	if err := os.WriteFile(paths.MD, []byte(cbarcheck.Markdown(report, title)), 0o644); err != nil {
		return paths, err
	}
	return paths, nil
}

func writeYUVFile(path string, frame *cbarcheck.Frame) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := cbarcheck.WriteRaw444(f, frame); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func printFirstViolation(label string, report *cbarcheck.Report) {
	v := report.FirstViolation
	if v == nil {
		fmt.Printf("%s: PASS; no violating pixel\n", label)
		return
	}
	fmt.Printf("%s: FAIL; first violating pixel (x=%d,y=%d) region=%s reasons=%v\n",
		label, v.X, v.Y, v.Region, v.Reasons)
	fmt.Printf("  observed Y/Cb/Cr = %.0f/%.0f/%.0f\n", v.Observed.Y, v.Observed.Cb, v.Observed.Cr)
	fmt.Printf("  expected Y/Cb/Cr = %.0f/%.0f/%.0f\n", v.Expected.Y, v.Expected.Cb, v.Expected.Cr)
	fmt.Printf("  error (8-bit)    = %.2f/%.2f/%.2f\n", v.ErrorCode8.Y, v.ErrorCode8.Cb, v.ErrorCode8.Cr)
	fmt.Printf("  legal bounds     = Y[%.0f,%.0f] Cb[%.0f,%.0f] Cr[%.0f,%.0f]\n",
		v.Bounds.YMin, v.Bounds.YMax, v.Bounds.CbMin, v.Bounds.CbMax, v.Bounds.CrMin, v.Bounds.CrMax)
	if c := report.FirstChromaViolation; c != nil {
		fmt.Printf("  first chroma violation: (x=%d,y=%d) region=%s Cb/Cr observed=%.0f/%.0f expected=%.0f/%.0f\n",
			c.X, c.Y, c.Region, c.Observed.Cb, c.Observed.Cr, c.Expected.Cb, c.Expected.Cr)
	}
}
