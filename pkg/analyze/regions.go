package analyze

// findRegions extracts 4-connected components of pixels whose luminance is
// strictly above thresholdNits. Components below minAreaRatio of the frame
// are kept (callers need them for false-positive review, e.g. subtitles or
// specular sparks) and flagged Qualified=false.
func findRegions(f *Frame, lum []float64, thresholdNits, minAreaRatio float64) []Region {
	w, h := f.Width, f.Height
	n := w * h
	mask := make([]bool, n)
	for i, y := range lum {
		mask[i] = y > thresholdNits
	}

	visited := make([]bool, n)
	queue := make([]int, 0, 1024)
	var regions []Region

	for start := 0; start < n; start++ {
		if !mask[start] || visited[start] {
			continue
		}
		visited[start] = true
		queue = append(queue[:0], start)

		reg := Region{
			ID:   len(regions) + 1,
			MinX: w,
			MinY: h,
			MaxX: -1,
			MaxY: -1,
		}
		sum := 0.0

		for head := 0; head < len(queue); head++ {
			idx := queue[head]
			x, y := idx%w, idx/w
			reg.Size++
			sum += lum[idx]
			if lum[idx] > reg.PeakNits {
				reg.PeakNits = lum[idx]
				reg.PeakCode = maxCode(f, idx)
			}
			if x < reg.MinX {
				reg.MinX = x
			}
			if x > reg.MaxX {
				reg.MaxX = x
			}
			if y < reg.MinY {
				reg.MinY = y
			}
			if y > reg.MaxY {
				reg.MaxY = y
			}

			if x > 0 {
				if j := idx - 1; mask[j] && !visited[j] {
					visited[j] = true
					queue = append(queue, j)
				}
			}
			if x < w-1 {
				if j := idx + 1; mask[j] && !visited[j] {
					visited[j] = true
					queue = append(queue, j)
				}
			}
			if y > 0 {
				if j := idx - w; mask[j] && !visited[j] {
					visited[j] = true
					queue = append(queue, j)
				}
			}
			if y < h-1 {
				if j := idx + w; mask[j] && !visited[j] {
					visited[j] = true
					queue = append(queue, j)
				}
			}
		}

		reg.AreaRatio = float64(reg.Size) / float64(n)
		reg.Qualified = reg.AreaRatio >= minAreaRatio
		reg.MeanNits = sum / float64(reg.Size)
		regions = append(regions, reg)
	}
	return regions
}
