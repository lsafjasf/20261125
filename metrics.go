package main

import "math"

const (
	blockSize       = 8
	darkLumaMax     = 64
	smoothGradMax   = 2
	noiseFloorDelta = 1.0
	blockAlignMin   = 1.8
	bandStepMin     = 2
	bandFlatMax     = 1
	bandRateMin     = 0.004
	chromaDriftMin  = 1.5
	darkSmoothMin   = 0.25
	tileSize        = 64
	classNone       = "none"
	classCoding     = "coding"
	classConversion = "conversion"
	classMixed      = "mixed"
)

type tileStats struct {
	Frame          string  `json:"frame"`
	Set            string  `json:"set"`
	Col            int     `json:"col"`
	Row            int     `json:"row"`
	X0, Y0, X1, Y1 int     `json:"-"`
	MSE            float64 `json:"mse"`
	PSNR           float64 `json:"psnr"`
	MeanAbsDelta   float64 `json:"meanAbsDelta"`
	BlockScore     float64 `json:"blockScore"`
	BandScore      float64 `json:"bandScore"`
	ChromaScore    float64 `json:"chromaScore"`
	DarkSmoothFrac float64 `json:"darkSmoothFrac"`
	Class          string  `json:"class"`
}

type frameStats struct {
	Frame        string         `json:"frame"`
	PSNRY        float64        `json:"psnrY"`
	RMSECb       float64        `json:"rmseCb"`
	RMSECr       float64        `json:"rmseCr"`
	BlockScore   float64        `json:"blockScore"`
	BandSteps    int            `json:"bandSteps"`
	DarkSmoothPx int            `json:"darkSmoothPx"`
	TileCounts   map[string]int `json:"tileCounts"`
	Tiles        []tileStats    `json:"tiles"`
}

// analyzeFrame compares one coded frame against its reference and tiles the
// residual spatially so artifact types can be localized per frame.
func analyzeFrame(ref, coded *Frame, setName string) *frameStats {
	if ref.W != coded.W || ref.H != coded.H {
		panic("dimension mismatch for frame " + ref.Name)
	}
	w, h := ref.W, ref.H
	gx, gy := lumaGradients(ref)
	mask := darkSmoothMask(ref, gx, gy)

	fs := &frameStats{Frame: ref.Name, TileCounts: map[string]int{}}
	var seY, seCb, seCr float64
	for ty := 0; ty < h; ty += tileSize {
		for tx := 0; tx < w; tx += tileSize {
			t := analyzeTile(ref, coded, mask, tx, ty, setName)
			fs.Tiles = append(fs.Tiles, t)
			fs.TileCounts[t.Class]++
		}
	}
	n := float64(w * h)
	for i := 0; i < w*h; i++ {
		dY := float64(coded.Y[i]) - float64(ref.Y[i])
		dCb := float64(coded.Cb[i]) - float64(ref.Cb[i])
		dCr := float64(coded.Cr[i]) - float64(ref.Cr[i])
		seY += dY * dY
		seCb += dCb * dCb
		seCr += dCr * dCr
	}
	fs.PSNRY = psnr(seY / n)
	fs.RMSECb = math.Sqrt(seCb / n)
	fs.RMSECr = math.Sqrt(seCr / n)
	fs.BlockScore = globalBlockScore(ref, coded)
	fs.BandSteps, fs.DarkSmoothPx = countBanding(coded, mask)
	return fs
}

func analyzeTile(ref, coded *Frame, mask []bool, tx, ty int, setName string) tileStats {
	w, h := ref.W, ref.H
	x1 := min(tx+tileSize, w)
	y1 := min(ty+tileSize, h)
	t := tileStats{
		Frame: ref.Name, Set: setName,
		Col: tx / tileSize, Row: ty / tileSize,
		X0: tx, Y0: ty, X1: x1, Y1: y1,
	}
	var se, absSum, bSum, bCnt, iSum, iCnt float64
	var maskCnt, bandSteps int
	var chromaSum float64
	for y := ty; y < y1; y++ {
		for x := tx; x < x1; x++ {
			idx := y*w + x
			dY := float64(coded.Y[idx]) - float64(ref.Y[idx])
			ad := math.Abs(dY)
			se += dY * dY
			absSum += ad
			if x > 0 && y > 0 && (x%blockSize == 0 || y%blockSize == 0) {
				bSum += ad
				bCnt++
			} else if x > 0 && y > 0 {
				iSum += ad
				iCnt++
			}
			if mask[idx] {
				maskCnt++
				chromaSum += math.Abs(float64(coded.Cb[idx])-float64(ref.Cb[idx])) +
					math.Abs(float64(coded.Cr[idx])-float64(ref.Cr[idx]))
			}
		}
	}
	px := float64((x1 - tx) * (y1 - ty))
	t.MSE = se / px
	t.PSNR = psnr(t.MSE)
	t.MeanAbsDelta = absSum / px
	t.BlockScore = ratioOrNeutral(bSum, bCnt, iSum, iCnt)
	bandSteps, maskPixels := countBanding(coded, mask, tx, ty, x1, y1)
	t.BandScore = float64(bandSteps) / max(1, float64(maskPixels))
	t.ChromaScore = chromaSum / max(1, float64(maskCnt))
	t.DarkSmoothFrac = float64(maskCnt) / px
	t.Class = classify(t)
	return t
}

// classify separates block-aligned coding artifacts from color-conversion
// banding/chroma drift in dark smooth areas; mixed means both mechanisms fire.
func classify(t tileStats) string {
	if t.MeanAbsDelta < noiseFloorDelta {
		return classNone
	}
	coding := t.BlockScore >= blockAlignMin
	conversion := t.DarkSmoothFrac >= darkSmoothMin &&
		(t.BandScore >= bandRateMin || t.ChromaScore >= chromaDriftMin)
	switch {
	case coding && conversion:
		return classMixed
	case coding:
		return classCoding
	case conversion:
		return classConversion
	default:
		return classNone
	}
}

func ratioOrNeutral(bSum, bCnt, iSum, iCnt float64) float64 {
	if bCnt == 0 || iCnt == 0 {
		return 1
	}
	interior := iSum / iCnt
	if interior == 0 {
		if bSum == 0 {
			return 1
		}
		return bSum / bCnt * 100
	}
	return (bSum / bCnt) / interior
}

// globalBlockScore measures residual energy concentration on 8px transform
// block boundaries, the signature of block-based coding artifacts.
func globalBlockScore(ref, coded *Frame) float64 {
	w, h := ref.W, ref.H
	var bSum, bCnt, iSum, iCnt float64
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			ad := math.Abs(float64(coded.Y[y*w+x]) - float64(ref.Y[y*w+x]))
			if x%blockSize == 0 || y%blockSize == 0 {
				bSum += ad
				bCnt++
			} else {
				iSum += ad
				iCnt++
			}
		}
	}
	return ratioOrNeutral(bSum, bCnt, iSum, iCnt)
}

func lumaGradients(f *Frame) ([]int, []int) {
	w, h := f.W, f.H
	gx := make([]int, w*h)
	gy := make([]int, w*h)
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			i := y*w + x
			gx[i] = absInt(int(f.Y[i+1]) - int(f.Y[i-1]))
			gy[i] = absInt(int(f.Y[i+w]) - int(f.Y[i-w]))
		}
	}
	return gx, gy
}

func darkSmoothMask(f *Frame, gx, gy []int) []bool {
	mask := make([]bool, f.W*f.H)
	for i := range mask {
		mask[i] = f.Y[i] <= darkLumaMax && gx[i] <= smoothGradMax && gy[i] <= smoothGradMax
	}
	return mask
}

// countBanding counts luma step edges inside dark smooth regions. A band step
// is an adjacent jump >= bandStepMin with flat runs (gradient <= bandFlatMax)
// on both sides — quantization/contouring rather than texture.
func countBanding(f *Frame, mask []bool, bounds ...int) (int, int) {
	x0, y0, x1, y1 := 0, 0, f.W, f.H
	if len(bounds) == 4 {
		x0, y0, x1, y1 = bounds[0], bounds[1], bounds[2], bounds[3]
	}
	w := f.W
	steps, maskPx := 0, 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			i := y*w + x
			if !mask[i] {
				continue
			}
			maskPx++
			if x > x0 && x+1 < x1 && mask[i-1] && mask[i+1] {
				d := absInt(int(f.Y[i]) - int(f.Y[i-1]))
				flatL := x-2 < x0 || !mask[i-2] || absInt(int(f.Y[i-1])-int(f.Y[i-2])) <= bandFlatMax
				flatR := absInt(int(f.Y[i+1])-int(f.Y[i])) <= bandFlatMax
				if d >= bandStepMin && flatL && flatR {
					steps++
				}
			}
			if y > y0 && y+1 < y1 && mask[i-w] && mask[i+w] {
				d := absInt(int(f.Y[i]) - int(f.Y[i-w]))
				flatU := y-2 < y0 || !mask[i-2*w] || absInt(int(f.Y[i-w])-int(f.Y[i-2*w])) <= bandFlatMax
				flatD := absInt(int(f.Y[i+w])-int(f.Y[i])) <= bandFlatMax
				if d >= bandStepMin && flatU && flatD {
					steps++
				}
			}
		}
	}
	return steps, maskPx
}

func psnr(mse float64) float64 {
	if mse <= 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10((255*255)/mse)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
