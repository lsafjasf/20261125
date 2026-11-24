package frameio

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ManifestSchema = "tear-manifest/v1"

type Manifest struct {
	Schema string          `json:"schema,omitempty"`
	FPSNum int             `json:"fps_num,omitempty"`
	FPSDen int             `json:"fps_den,omitempty"`
	Frames []ManifestFrame `json:"frames"`
}

type ManifestFrame struct {
	ID          string `json:"id,omitempty"`
	File        string `json:"file"`
	PTSUS       int64  `json:"pts_us"`
	DurationUS  int64  `json:"duration_us,omitempty"`
	ptsSet      bool
	durationSet bool
}

func (frame *ManifestFrame) UnmarshalJSON(raw []byte) error {
	var decoded struct {
		ID         string `json:"id"`
		File       string `json:"file"`
		PTSUS      *int64 `json:"pts_us"`
		PTSMS      *int64 `json:"pts_ms"`
		DurationUS *int64 `json:"duration_us"`
		DurationMS *int64 `json:"duration_ms"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	if decoded.PTSUS != nil && decoded.PTSMS != nil {
		return fmt.Errorf("frame specifies both pts_us and pts_ms")
	}
	if decoded.DurationUS != nil && decoded.DurationMS != nil {
		return fmt.Errorf("frame specifies both duration_us and duration_ms")
	}
	frame.ID = decoded.ID
	frame.File = decoded.File
	if decoded.PTSUS != nil {
		frame.PTSUS = *decoded.PTSUS
		frame.ptsSet = true
	} else if decoded.PTSMS != nil {
		if *decoded.PTSMS > maxInt64/1000 || *decoded.PTSMS < minInt64/1000 {
			return fmt.Errorf("pts_ms value is out of range")
		}
		frame.PTSUS = *decoded.PTSMS * 1000
		frame.ptsSet = true
	}
	if decoded.DurationUS != nil {
		frame.DurationUS = *decoded.DurationUS
		frame.durationSet = true
	} else if decoded.DurationMS != nil {
		if *decoded.DurationMS > maxInt64/1000 || *decoded.DurationMS < minInt64/1000 {
			return fmt.Errorf("duration_ms value is out of range")
		}
		frame.DurationUS = *decoded.DurationMS * 1000
		frame.durationSet = true
	}
	return nil
}

const (
	maxInt64 = int64(^uint64(0) >> 1)
	minInt64 = -maxInt64 - 1
)

type Plan struct {
	RootDir         string
	ManifestPath    string
	ManifestSHA256  string
	TimestampSource string
	FrameRate       string
	Frames          []FrameSpec
}

type FrameSpec struct {
	ID         string
	File       string
	AbsPath    string
	PTSUS      int64
	DurationUS int64
}

type Frame struct {
	Spec   FrameSpec
	Gray   *image.Gray
	Hash   string
	Width  int
	Height int
}

type Duplicate struct {
	IsDuplicate bool
	OfID        string
	Run         int
}

func LoadPlan(rootDir, manifestPath string, fpsNum, fpsDen int) (*Plan, error) {
	explicitManifest := manifestPath != ""
	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve input directory: %w", err)
	}
	info, err := os.Stat(rootDir)
	if err != nil {
		return nil, fmt.Errorf("stat input directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("input path is not a directory: %s", rootDir)
	}
	if manifestPath == "" {
		manifestPath = filepath.Join(rootDir, "frames.json")
	} else if !filepath.IsAbs(manifestPath) {
		manifestPath = filepath.Join(rootDir, manifestPath)
	}

	if _, err := os.Stat(manifestPath); err == nil {
		return loadManifestPlan(rootDir, manifestPath)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat manifest: %w", err)
	} else if explicitManifest {
		return nil, fmt.Errorf("manifest does not exist: %s", manifestPath)
	}

	if fpsNum <= 0 || fpsDen <= 0 {
		return nil, fmt.Errorf("fps_num and fps_den must be positive when no frames.json exists")
	}
	return scanDirectoryPlan(rootDir, fpsNum, fpsDen)
}

func loadManifestPlan(rootDir, manifestPath string) (*Plan, error) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.Schema != "" && manifest.Schema != ManifestSchema {
		return nil, fmt.Errorf("unsupported manifest schema %q", manifest.Schema)
	}
	if len(manifest.Frames) == 0 {
		return nil, fmt.Errorf("manifest contains no frames")
	}

	ids := make(map[string]int, len(manifest.Frames))
	plan := &Plan{
		RootDir:         rootDir,
		ManifestPath:    manifestPath,
		ManifestSHA256:  sha256Hex(raw),
		TimestampSource: "manifest",
	}
	if (manifest.FPSNum > 0) != (manifest.FPSDen > 0) {
		return nil, fmt.Errorf("manifest fps_num and fps_den must both be positive")
	}
	fpsValid := manifest.FPSNum > 0 && manifest.FPSDen > 0
	if fpsValid {
		plan.FrameRate = fmt.Sprintf("%d/%d", manifest.FPSNum, manifest.FPSDen)
	}

	anyPTS, allPTS := false, true
	for _, spec := range manifest.Frames {
		if spec.ptsSet {
			anyPTS = true
		} else {
			allPTS = false
		}
	}
	if anyPTS && !allPTS {
		return nil, fmt.Errorf("manifest must provide pts_us for every frame or for none of them")
	}
	if !anyPTS && !fpsValid {
		return nil, fmt.Errorf("manifest needs pts_us per frame or fps_num/fps_den")
	}
	frameDurationUS := int64(0)
	if fpsValid {
		frameDurationUS = int64(1_000_000) * int64(manifest.FPSDen) / int64(manifest.FPSNum)
		if frameDurationUS <= 0 {
			frameDurationUS = 1
		}
	}

	var previousPTS int64 = -1
	for index, spec := range manifest.Frames {
		if spec.File == "" {
			return nil, fmt.Errorf("manifest frame %d has no file", index)
		}
		relative := filepath.Clean(filepath.FromSlash(spec.File))
		if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("manifest frame %d file %q must be relative to the input directory", index, spec.File)
		}
		ptsUS := spec.PTSUS
		durationUS := spec.DurationUS
		if !anyPTS {
			ptsUS = int64(index) * int64(1_000_000) * int64(manifest.FPSDen) / int64(manifest.FPSNum)
		}
		if !spec.durationSet && fpsValid {
			durationUS = frameDurationUS
		}
		if ptsUS < 0 {
			return nil, fmt.Errorf("manifest frame %d has negative pts_us", index)
		}
		if ptsUS < previousPTS {
			return nil, fmt.Errorf("manifest frame %d pts_us %d is earlier than previous pts_us %d", index, ptsUS, previousPTS)
		}
		if durationUS < 0 {
			return nil, fmt.Errorf("manifest frame %d has negative duration_us", index)
		}
		previousPTS = ptsUS

		id := strings.TrimSpace(spec.ID)
		if id == "" {
			id = idFromFile(relative, index)
		}
		if prior, exists := ids[id]; exists {
			return nil, fmt.Errorf("duplicate frame id %q at indexes %d and %d", id, prior, index)
		}
		ids[id] = index

		absPath := filepath.Join(rootDir, relative)
		plan.Frames = append(plan.Frames, FrameSpec{
			ID:         id,
			File:       filepath.ToSlash(relative),
			AbsPath:    absPath,
			PTSUS:      ptsUS,
			DurationUS: durationUS,
		})
	}
	return plan, nil
}

func scanDirectoryPlan(rootDir string, fpsNum, fpsDen int) (*Plan, error) {
	var files []string
	err := filepath.WalkDir(rootDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".png", ".jpg", ".jpeg":
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan input directory: %w", err)
	}
	sort.Slice(files, func(i, j int) bool {
		left, _ := filepath.Rel(rootDir, files[i])
		right, _ := filepath.Rel(rootDir, files[j])
		return filepath.ToSlash(left) < filepath.ToSlash(right)
	})
	if len(files) == 0 {
		return nil, fmt.Errorf("no .png, .jpg, or .jpeg frames found in %s and no frames.json", rootDir)
	}

	plan := &Plan{
		RootDir:         rootDir,
		TimestampSource: "directory_order_fps",
		FrameRate:       fmt.Sprintf("%d/%d", fpsNum, fpsDen),
	}
	frameDurationUS := int64(1_000_000) * int64(fpsDen) / int64(fpsNum)
	if frameDurationUS <= 0 {
		frameDurationUS = 1
	}
	for index, path := range files {
		rel, err := filepath.Rel(rootDir, path)
		if err != nil {
			return nil, fmt.Errorf("relativize frame path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		plan.Frames = append(plan.Frames, FrameSpec{
			ID:         idFromFile(rel, index),
			File:       rel,
			AbsPath:    path,
			PTSUS:      int64(index) * int64(1_000_000) * int64(fpsDen) / int64(fpsNum),
			DurationUS: frameDurationUS,
		})
	}
	return plan, nil
}

func LoadFrame(spec FrameSpec) (Frame, error) {
	file, err := os.Open(spec.AbsPath)
	if err != nil {
		return Frame{}, fmt.Errorf("open frame %q: %w", spec.File, err)
	}
	defer file.Close()

	decoded, format, err := image.Decode(file)
	if err != nil {
		return Frame{}, fmt.Errorf("decode frame %q: %w", spec.File, err)
	}
	if format != "png" && format != "jpeg" {
		return Frame{}, fmt.Errorf("frame %q uses unsupported format %q", spec.File, format)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return Frame{}, fmt.Errorf("frame %q has empty bounds", spec.File)
	}
	gray := image.NewGray(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	drawGray(gray, decoded)
	return Frame{
		Spec:   spec,
		Gray:   gray,
		Hash:   grayHash(gray),
		Width:  bounds.Dx(),
		Height: bounds.Dy(),
	}, nil
}

func drawGray(dst *image.Gray, src image.Image) {
	bounds := src.Bounds()
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			r, g, b, _ := src.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			r8, g8, b8 := int(r>>8), int(g>>8), int(b>>8)
			luma := (19595*r8 + 38470*g8 + 7471*b8 + 32768) >> 16
			dst.Pix[y*dst.Stride+x] = uint8(luma)
		}
	}
}

func grayHash(gray *image.Gray) string {
	hash := sha256.New()
	var header [16]byte
	putUint64(header[0:8], uint64(gray.Rect.Dx()))
	putUint64(header[8:16], uint64(gray.Rect.Dy()))
	_, _ = hash.Write(header[:])
	for y := 0; y < gray.Rect.Dy(); y++ {
		_, _ = hash.Write(gray.Pix[y*gray.Stride : y*gray.Stride+gray.Rect.Dx()])
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func putUint64(dst []byte, value uint64) {
	for i := 0; i < 8; i++ {
		dst[i] = byte(value >> (8 * (7 - i)))
	}
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func idFromFile(name string, index int) string {
	base := filepath.ToSlash(name)
	base = strings.TrimPrefix(base, "./")
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.TrimSpace(base)
	if base == "" {
		return fmt.Sprintf("frame-%06d", index+1)
	}
	base = strings.ReplaceAll(base, "/", "_")
	return base
}
