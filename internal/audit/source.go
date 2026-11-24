package audit

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const DefaultMaxSamples = uint64(2_000_000)

func LoadClip(spec ClipSpec, baseDir string, defaultMax uint64) (LoadInfo, *Accumulator, error) {
	if spec.Path == "" {
		return LoadInfo{}, nil, errors.New("clip path is empty")
	}
	path := spec.Path
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return LoadInfo{}, nil, err
	}
	if info.IsDir() {
		return LoadInfo{}, nil, fmt.Errorf("%s is a directory", path)
	}

	maxSamples := spec.MaxSamples
	if maxSamples == 0 {
		maxSamples = defaultMax
	}
	if maxSamples == 0 {
		maxSamples = DefaultMaxSamples
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif":
		return loadImage(path, info.Size(), maxSamples)
	case ".y4m":
		return loadY4M(path, info.Size(), maxSamples)
	case ".yuv", ".raw", ".gray":
		return loadRaw(path, info.Size(), spec, maxSamples)
	default:
		return LoadInfo{}, nil, fmt.Errorf("unsupported input extension %q; use png, jpg, gif, y4m, or raw yuv with width/height/pixelFormat", ext)
	}
}

func loadImage(path string, fileBytes int64, maxSamples uint64) (LoadInfo, *Accumulator, error) {
	f, err := os.Open(path)
	if err != nil {
		return LoadInfo{}, nil, err
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return LoadInfo{}, nil, fmt.Errorf("decode image: %w", err)
	}
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()
	if width <= 0 || height <= 0 {
		return LoadInfo{}, nil, errors.New("image has empty bounds")
	}
	total := uint64(width) * uint64(height)
	stride := sampleStride(total, maxSamples)
	acc := NewAccumulator()
	var seen uint64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if seen%stride == 0 {
				acc.Add(luma8(img.At(x, y)))
			}
			seen++
		}
	}
	return LoadInfo{
		Path:      path,
		Format:    format,
		Width:     width,
		Height:    height,
		Frames:    1,
		Sampled:   acc.Total(),
		FileBytes: fileBytes,
	}, acc, nil
}

func luma8(c color.Color) uint8 {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	r := uint32(n.R)
	g := uint32(n.G)
	b := uint32(n.B)
	return uint8((13933*r + 46871*g + 4732*b + 32768) >> 16)
}

func sampleStride(total, maxSamples uint64) uint64 {
	if maxSamples == 0 || total <= maxSamples {
		return 1
	}
	return uint64(math.Ceil(float64(total) / float64(maxSamples)))
}

type planeLayout struct {
	lumaBytes   int64
	chromaBytes int64
}

func rawLayout(width, height int, pixelFormat string) (planeLayout, error) {
	if width <= 0 || height <= 0 {
		return planeLayout{}, errors.New("raw input requires positive width and height")
	}
	pixels := int64(width) * int64(height)
	switch strings.ToLower(strings.TrimSpace(pixelFormat)) {
	case "gray", "grey", "y", "y8":
		return planeLayout{lumaBytes: pixels}, nil
	case "yuv420p", "i420", "i420p", "420":
		if width%2 != 0 || height%2 != 0 {
			return planeLayout{}, errors.New("yuv420p requires even width and height")
		}
		return planeLayout{lumaBytes: pixels, chromaBytes: pixels / 2}, nil
	case "yuv422p", "i422", "422":
		if width%2 != 0 {
			return planeLayout{}, errors.New("yuv422p requires even width")
		}
		return planeLayout{lumaBytes: pixels, chromaBytes: pixels}, nil
	case "yuv444p", "i444", "444":
		return planeLayout{lumaBytes: pixels, chromaBytes: pixels * 2}, nil
	default:
		return planeLayout{}, fmt.Errorf("unsupported raw pixel format %q", pixelFormat)
	}
}

func loadRaw(path string, fileBytes int64, spec ClipSpec, maxSamples uint64) (LoadInfo, *Accumulator, error) {
	layout, err := rawLayout(spec.Width, spec.Height, spec.PixelFormat)
	if err != nil {
		return LoadInfo{}, nil, err
	}
	frameBytes := layout.lumaBytes + layout.chromaBytes
	if frameBytes <= 0 {
		return LoadInfo{}, nil, errors.New("invalid raw frame size")
	}
	frames := int(fileBytes / frameBytes)
	if frames == 0 {
		return LoadInfo{}, nil, errors.New("raw input is shorter than one frame")
	}
	totalLuma := uint64(frames) * uint64(layout.lumaBytes)
	stride := sampleStride(totalLuma, maxSamples)

	f, err := os.Open(path)
	if err != nil {
		return LoadInfo{}, nil, err
	}
	defer f.Close()

	acc := NewAccumulator()
	luma := make([]byte, layout.lumaBytes)
	for frame := 0; frame < frames; frame++ {
		if _, err := io.ReadFull(f, luma); err != nil {
			return LoadInfo{}, nil, fmt.Errorf("read luma frame %d: %w", frame, err)
		}
		base := uint64(frame) * uint64(layout.lumaBytes)
		for i, v := range luma {
			if (base+uint64(i))%stride == 0 {
				acc.Add(v)
			}
		}
		if layout.chromaBytes > 0 {
			if _, err := io.CopyN(io.Discard, f, layout.chromaBytes); err != nil {
				return LoadInfo{}, nil, fmt.Errorf("read chroma frame %d: %w", frame, err)
			}
		}
	}
	return LoadInfo{
		Path:      path,
		Format:    strings.ToLower(spec.PixelFormat),
		Width:     spec.Width,
		Height:    spec.Height,
		Frames:    frames,
		Sampled:   acc.Total(),
		FileBytes: fileBytes,
	}, acc, nil
}
