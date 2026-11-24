package media

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rec2020-to-rec709-shoulder/internal/colorconv"
	"rec2020-to-rec709-shoulder/internal/pipeline"
)

func ListFrames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".pfm":
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no PNG/JPEG/PFM frames in %s", dir)
	}
	return files, nil
}

func ReadFrame(path, transfer, signalRange string) (*pipeline.FloatImage, error) {
	if strings.EqualFold(filepath.Ext(path), ".pfm") {
		return ReadPFM(path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	return imageToLinear(img, transfer, signalRange), nil
}

func imageToLinear(img image.Image, transfer, signalRange string) *pipeline.FloatImage {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	out := pipeline.NewFloatImage(w, h)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			rf, gf, bf := unpremultiply16(r, g, b, a)
			i := ((y-bounds.Min.Y)*w + (x - bounds.Min.X)) * 3
			out.Pix[i] = colorconv.DecodeSignal(normalizeSignal(rf, signalRange), transfer)
			out.Pix[i+1] = colorconv.DecodeSignal(normalizeSignal(gf, signalRange), transfer)
			out.Pix[i+2] = colorconv.DecodeSignal(normalizeSignal(bf, signalRange), transfer)
		}
	}
	return out
}

func unpremultiply16(r, g, b, a uint32) (float64, float64, float64) {
	const maxV = 65535.0
	if a == 0 {
		return 0, 0, 0
	}
	rf := float64(r) / maxV
	gf := float64(g) / maxV
	bf := float64(b) / maxV
	af := float64(a) / maxV
	if af > 0 && af < 1 {
		rf /= af
		gf /= af
		bf /= af
	}
	return clamp(rf), clamp(gf), clamp(bf)
}

func normalizeSignal(v float64, signalRange string) float64 {
	if signalRange == "limited" {
		v = (v - 16.0/255.0) / (219.0 / 255.0)
	}
	return math.Max(0, v)
}

func WritePNG(path string, img *pipeline.FloatImage, transfer string, bit16 bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	if bit16 {
		rgba := image.NewNRGBA64(image.Rect(0, 0, img.Width, img.Height))
		for y := 0; y < img.Height; y++ {
			for x := 0; x < img.Width; x++ {
				i := (y*img.Width + x) * 3
				r := encode16(img.Pix[i], transfer)
				g := encode16(img.Pix[i+1], transfer)
				b := encode16(img.Pix[i+2], transfer)
				idx := rgba.PixOffset(x, y)
				rgba.Pix[idx+0] = uint8(r >> 8)
				rgba.Pix[idx+1] = uint8(r)
				rgba.Pix[idx+2] = uint8(g >> 8)
				rgba.Pix[idx+3] = uint8(g)
				rgba.Pix[idx+4] = uint8(b >> 8)
				rgba.Pix[idx+5] = uint8(b)
				rgba.Pix[idx+6] = 255
				rgba.Pix[idx+7] = 255
			}
		}
		return enc.Encode(file, rgba)
	}
	rgba := image.NewNRGBA(image.Rect(0, 0, img.Width, img.Height))
	for y := 0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			i := (y*img.Width + x) * 3
			rgba.SetNRGBA(x, y, color.NRGBA{
				R: encode8(img.Pix[i], transfer),
				G: encode8(img.Pix[i+1], transfer),
				B: encode8(img.Pix[i+2], transfer),
				A: 255,
			})
		}
	}
	return enc.Encode(file, rgba)
}

func encode16(v float64, transfer string) uint16 {
	v = clamp(colorconv.EncodeSignal(clamp(v), transfer))
	return uint16(math.Round(v * 65535))
}

func encode8(v float64, transfer string) uint8 {
	v = clamp(colorconv.EncodeSignal(clamp(v), transfer))
	return uint8(math.Round(v * 255))
}

func clamp(v float64) float64 { return math.Max(0, math.Min(1, v)) }
