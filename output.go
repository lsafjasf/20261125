package sdrquant

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// SavePNG writes a normalized diagnostic or processed float image as 8-bit
// PNG. It is intended for visualization, not as a second processing step.
func SavePNG(path string, im *FloatImage) error {
	if im.Channels == 1 {
		dst := image.NewGray(image.Rect(0, 0, im.Width, im.Height))
		for y := 0; y < im.Height; y++ {
			for x := 0; x < im.Width; x++ {
				v := uint8(math.Floor(Clamp01(float64(im.At(x, y, 0)))*255 + 0.5))
				dst.SetGray(x, y, color.Gray{Y: v})
			}
		}
		return writePNG(path, dst)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, im.Width, im.Height))
	for y := 0; y < im.Height; y++ {
		for x := 0; x < im.Width; x++ {
			r := uint8(math.Floor(Clamp01(float64(im.At(x, y, 0)))*255 + 0.5))
			g := uint8(math.Floor(Clamp01(float64(im.At(x, y, 1)))*255 + 0.5))
			b := uint8(math.Floor(Clamp01(float64(im.At(x, y, 2)))*255 + 0.5))
			dst.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return writePNG(path, dst)
}

// SaveResultPNG writes the quantized integer output without any color
// management or blur.
func SaveResultPNG(path string, res *Result) error {
	if res == nil {
		return fmt.Errorf("nil result")
	}
	if res.Channels == 1 {
		dst := image.NewGray(image.Rect(0, 0, res.Width, res.Height))
		copy(dst.Pix, res.Quant)
		return writePNG(path, dst)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, res.Width, res.Height))
	for i := 0; i < res.Width*res.Height; i++ {
		dst.Pix[i*4] = res.Quant[i*3]
		dst.Pix[i*4+1] = res.Quant[i*3+1]
		dst.Pix[i*4+2] = res.Quant[i*3+2]
		dst.Pix[i*4+3] = 255
	}
	return writePNG(path, dst)
}

func writePNG(path string, im image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(f, im); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return f.Close()
}

// WriteDiagnostics creates the four maps, metrics.json, and a compact radial
// spectrum.csv. Existing unrelated files in dir are left untouched.
func WriteDiagnostics(dir string, diag *Diagnostics) error {
	if diag == nil {
		return fmt.Errorf("nil diagnostics")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := []struct {
		name string
		im   *FloatImage
	}{
		{"residual.png", diag.ResidualMap},
		{"spectrum.png", diag.SpectrumMap},
		{"diffusion.png", diag.DiffusionMap},
		{"classification.png", diag.ClassificationMap},
	}
	for _, file := range files {
		if err := SavePNG(filepath.Join(dir, file.name), file.im); err != nil {
			return err
		}
	}
	if err := writeMetrics(filepath.Join(dir, "metrics.json"), diag.Metrics); err != nil {
		return err
	}
	return writeRadialSpectrum(filepath.Join(dir, "spectrum.csv"), diag.SpectrumDB, diag.SpectrumSize)
}

func writeMetrics(path string, metrics Metrics) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(metrics); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return f.Close()
}

func writeRadialSpectrum(path string, spectrumDB []float64, size int) error {
	const bins = 128
	if len(spectrumDB) != size*size || size <= 0 {
		return fmt.Errorf("invalid spectrum grid")
	}
	sum := make([]float64, bins)
	count := make([]int, bins)
	for y := 0; y < size; y++ {
		fy := float64(y-size/2) / float64(size)
		for x := 0; x < size; x++ {
			fx := float64(x-size/2) / float64(size)
			radius := math.Hypot(fx, fy)
			bin := int(radius / math.Sqrt(0.5) * float64(bins))
			if bin >= bins {
				bin = bins - 1
			}
			sum[bin] += math.Pow(10, spectrumDB[y*size+x]/10)
			count[bin]++
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, "radius_cycles_per_pixel,mean_power_db"); err != nil {
		return err
	}
	for i := 0; i < bins; i++ {
		power := 0.0
		if count[i] > 0 {
			power = sum[i] / float64(count[i])
		}
		radius := (float64(i) + 0.5) / float64(bins) * math.Sqrt(0.5)
		db := 10 * math.Log10(power+1e-15)
		if _, err := fmt.Fprintf(f, "%.8f,%.6f\n", radius, db); err != nil {
			return err
		}
	}
	return f.Close()
}
