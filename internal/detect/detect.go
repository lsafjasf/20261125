package detect

import (
	"image"
	"sort"
)

type Settings struct {
	BandRows               int `json:"band_rows"`
	MaxShiftPX             int `json:"max_shift_px"`
	MinShiftPX             int `json:"min_shift_px"`
	MinTextureMean         int `json:"min_texture_mean"`
	MinPrefilterMismatchBP int `json:"min_prefilter_mismatch_bp"`
	MinMatchScoreBP        int `json:"min_match_score_bp"`
	MinShiftGainBP         int `json:"min_shift_gain_bp"`
	MaxBiasRatioBP         int `json:"max_bias_ratio_bp"`
	MinStructuredDiff      int `json:"min_structured_diff"`
	MinPhotometricStep     int `json:"min_photometric_step"`
	MinLightBiasBP         int `json:"min_light_bias_bp"`
	MaxLightStructure      int `json:"max_light_structure"`
	CandidateRowSeparation int `json:"candidate_row_separation_px"`
	MaxCandidatesPerFrame  int `json:"max_candidates_per_frame"`
	MaxSeamsPerFrame       int `json:"max_seams_per_frame"`
	MaxLightEdgesPerFrame  int `json:"max_light_edges_per_frame"`
	MaxEventGapFrames      int `json:"max_event_gap_frames"`
	MaxRowJumpPX           int `json:"max_row_jump_px"`
	MaxShiftJumpPX         int `json:"max_shift_jump_px"`
	MinContinuousFrames    int `json:"min_continuous_frames"`
	BandPairTolerancePX    int `json:"band_pair_tolerance_px"`
	MinBandHeightPX        int `json:"min_band_height_px"`
}

type Seam struct {
	Row            int `json:"row"`
	OffsetPX       int `json:"offset_px"`
	WidthPX        int `json:"width_px"`
	MatchScoreBP   int `json:"match_score_bp"`
	ZeroScoreBP    int `json:"zero_score_bp"`
	ShiftGainBP    int `json:"shift_gain_bp"`
	BiasRatioBP    int `json:"bias_ratio_bp"`
	LumaStep       int `json:"luma_step"`
	StructuredDiff int `json:"structured_diff"`
	TextureMean    int `json:"texture_mean"`
}

type Band struct {
	TopRow         int `json:"top_row"`
	BottomRow      int `json:"bottom_row"`
	HeightPX       int `json:"height_px"`
	TopOffsetPX    int `json:"top_offset_px"`
	BottomOffsetPX int `json:"bottom_offset_px"`
	WidthPX        int `json:"width_px"`
}

type LightEdge struct {
	Row            int    `json:"row"`
	LumaStep       int    `json:"luma_step"`
	BiasRatioBP    int    `json:"bias_ratio_bp"`
	StructuredDiff int    `json:"structured_diff"`
	Reason         string `json:"reason"`
}

type FrameEvidence struct {
	Seams       []Seam
	Bands       []Band
	LightEdges  []LightEdge
	TextureMean int
}

func DefaultSettings() Settings {
	return Settings{
		BandRows:               6,
		MaxShiftPX:             48,
		MinShiftPX:             2,
		MinTextureMean:         3,
		MinPrefilterMismatchBP: 1200,
		MinMatchScoreBP:        6800,
		MinShiftGainBP:         2200,
		MaxBiasRatioBP:         7600,
		MinStructuredDiff:      2,
		MinPhotometricStep:     6,
		MinLightBiasBP:         6000,
		MaxLightStructure:      20,
		CandidateRowSeparation: 3,
		MaxCandidatesPerFrame:  24,
		MaxSeamsPerFrame:       3,
		MaxLightEdgesPerFrame:  3,
		MaxEventGapFrames:      2,
		MaxRowJumpPX:           16,
		MaxShiftJumpPX:         6,
		MinContinuousFrames:    3,
		BandPairTolerancePX:    4,
		MinBandHeightPX:        4,
	}
}

type rowStat struct {
	row            int
	mismatchBP     int
	lumaStep       int
	biasRatioBP    int
	structuredDiff int
	textureMean    int
}

type rowCandidate struct {
	row   int
	score int
}

// Detect finds structural horizontal discontinuities. It deliberately treats
// a brightness step that is uniform across the row as a photometric edge, not
// as tearing, because that is the signature of horizontal flares and light
// bands.
func Detect(gray *image.Gray, settings Settings) FrameEvidence {
	width, height := gray.Rect.Dx(), gray.Rect.Dy()
	if width <= 0 || height <= 0 {
		return FrameEvidence{}
	}
	pad := settings.MaxShiftPX + 1
	band := settings.BandRows
	if band < 1 || width <= 2*pad || height <= 2*band {
		return FrameEvidence{}
	}

	edges := horizontalEdges(gray)
	prefix := edgePrefix(edges, width, height)
	order := shiftOrder(settings.MaxShiftPX)

	var stats []rowStat
	var candidates []rowCandidate
	var lightCandidates []rowCandidate
	for row := band; row <= height-band; row++ {
		stat := measureRow(gray, prefix, width, row, pad, band)
		stats = append(stats, stat)

		if stat.textureMean >= settings.MinTextureMean &&
			stat.structuredDiff >= settings.MinStructuredDiff &&
			stat.biasRatioBP < settings.MaxBiasRatioBP &&
			stat.mismatchBP >= settings.MinPrefilterMismatchBP {
			candidates = append(candidates, rowCandidate{row: row, score: stat.score()})
		}
		if abs(stat.lumaStep) >= settings.MinPhotometricStep &&
			stat.biasRatioBP >= settings.MinLightBiasBP &&
			stat.structuredDiff <= settings.MaxLightStructure {
			lightCandidates = append(lightCandidates, rowCandidate{row: row, score: abs(stat.lumaStep)})
		}
	}

	chosenRows := selectRows(candidates, settings.CandidateRowSeparation, settings.MaxCandidatesPerFrame)
	byRow := make(map[int]rowStat, len(stats))
	for _, stat := range stats {
		byRow[stat.row] = stat
	}

	aProfile := make([]int64, width)
	bProfile := make([]int64, width)
	var seams []Seam
	maxTexture := 0
	for _, row := range chosenRows {
		stat := byRow[row]
		fillBandProfile(prefix, width, row-band, row, aProfile)
		fillBandProfile(prefix, width, row, row+band, bProfile)
		seam, ok := matchSeam(stat, aProfile, bProfile, width, pad, band, order, settings)
		if !ok {
			continue
		}
		seams = append(seams, seam)
		if seam.TextureMean > maxTexture {
			maxTexture = seam.TextureMean
		}
	}
	sort.Slice(seams, func(i, j int) bool {
		if seams[i].MatchScoreBP != seams[j].MatchScoreBP {
			return seams[i].MatchScoreBP > seams[j].MatchScoreBP
		}
		if seams[i].Row != seams[j].Row {
			return seams[i].Row < seams[j].Row
		}
		return seams[i].OffsetPX < seams[j].OffsetPX
	})
	if len(seams) > settings.MaxSeamsPerFrame {
		seams = seams[:settings.MaxSeamsPerFrame]
	}

	bands := pairBands(seams, settings)
	lightRows := selectRows(lightCandidates, 2, settings.MaxLightEdgesPerFrame)
	lightEdges := make([]LightEdge, 0, len(lightRows))
	for _, row := range lightRows {
		stat := byRow[row]
		lightEdges = append(lightEdges, LightEdge{
			Row:            stat.row,
			LumaStep:       stat.lumaStep,
			BiasRatioBP:    stat.biasRatioBP,
			StructuredDiff: stat.structuredDiff,
			Reason:         "photometric_step_uniform_across_row",
		})
	}
	sort.Slice(lightEdges, func(i, j int) bool {
		left, right := abs(lightEdges[i].LumaStep), abs(lightEdges[j].LumaStep)
		if left != right {
			return left > right
		}
		return lightEdges[i].Row < lightEdges[j].Row
	})

	return FrameEvidence{
		Seams:       seams,
		Bands:       bands,
		LightEdges:  lightEdges,
		TextureMean: maxTexture,
	}
}

func (stat rowStat) score() int {
	return stat.mismatchBP*2 + stat.structuredDiff*32
}

func horizontalEdges(gray *image.Gray) []uint8 {
	width, height := gray.Rect.Dx(), gray.Rect.Dy()
	edges := make([]uint8, width*height)
	for y := 0; y < height; y++ {
		base := y * gray.Stride
		out := y * width
		for x := 1; x < width-1; x++ {
			edges[out+x] = uint8(abs(int(gray.Pix[base+x+1]) - int(gray.Pix[base+x-1])))
		}
	}
	return edges
}

func edgePrefix(edges []uint8, width, height int) []int32 {
	prefix := make([]int32, (height+1)*width)
	for y := 0; y < height; y++ {
		current := y * width
		next := (y + 1) * width
		for x := 0; x < width; x++ {
			prefix[next+x] = prefix[current+x] + int32(edges[current+x])
		}
	}
	return prefix
}

func measureRow(gray *image.Gray, prefix []int32, width, row, pad, band int) rowStat {
	stat := rowStat{row: row}
	var sumAbove, sumBelow, sumMin int64
	for x := pad; x < width-pad; x++ {
		above := int64(prefix[row*width+x] - prefix[(row-band)*width+x])
		below := int64(prefix[(row+band)*width+x] - prefix[row*width+x])
		sumAbove += above
		sumBelow += below
		sumMin += min64(above, below)
	}
	if denom := sumAbove + sumBelow; denom > 0 {
		stat.mismatchBP = int(10000 * (denom - 2*sumMin) / denom)
	}
	samples := int64(band * (width - 2*pad))
	if samples > 0 {
		stat.textureMean = int(sumAbove / samples)
	}

	var sumSigned, sumAbs int64
	for x := pad; x < width-pad; x++ {
		diff := int64(gray.Pix[row*gray.Stride+x]) - int64(gray.Pix[(row-1)*gray.Stride+x])
		sumSigned += diff
		sumAbs += abs64(diff)
	}
	count := int64(width - 2*pad)
	if count <= 0 {
		return stat
	}
	mean := sumSigned / count
	stat.lumaStep = int(mean)
	var sumDeviation int64
	for x := pad; x < width-pad; x++ {
		diff := int64(gray.Pix[row*gray.Stride+x]) - int64(gray.Pix[(row-1)*gray.Stride+x])
		sumDeviation += abs64(diff - mean)
	}
	stat.structuredDiff = int(sumDeviation / count)
	meanAbs := sumAbs / count
	if denom := meanAbs + int64(stat.structuredDiff); denom > 0 {
		stat.biasRatioBP = int(10000 * meanAbs / denom)
	}
	return stat
}

func fillBandProfile(prefix []int32, width, startRow, endRow int, profile []int64) {
	for x := 0; x < width; x++ {
		profile[x] = int64(prefix[endRow*width+x] - prefix[startRow*width+x])
	}
}

func matchSeam(stat rowStat, above, below []int64, width, pad, band int, order []int, settings Settings) (Seam, bool) {
	var sumAbove int64
	for x := pad; x < width-pad; x++ {
		sumAbove += above[x]
	}
	if sumAbove <= 0 {
		return Seam{}, false
	}

	bestScore := -1
	bestShift := 0
	zeroScore := 0
	for _, shift := range order {
		var sumShifted, overlap int64
		for x := pad; x < width-pad; x++ {
			value := below[x+shift]
			sumShifted += value
			overlap += min64(above[x], value)
		}
		denom := sumAbove + sumShifted
		if denom <= 0 {
			continue
		}
		score := int(20000 * overlap / denom)
		if shift == 0 {
			zeroScore = score
		}
		if score > bestScore {
			bestScore = score
			bestShift = shift
		}
	}
	if bestScore < 0 {
		return Seam{}, false
	}
	if abs(bestShift) < settings.MinShiftPX ||
		bestScore < settings.MinMatchScoreBP ||
		bestScore-zeroScore < settings.MinShiftGainBP ||
		stat.biasRatioBP >= settings.MaxBiasRatioBP ||
		stat.structuredDiff < settings.MinStructuredDiff ||
		stat.textureMean < settings.MinTextureMean {
		return Seam{}, false
	}

	return Seam{
		Row:            stat.row,
		OffsetPX:       bestShift,
		WidthPX:        abs(bestShift),
		MatchScoreBP:   bestScore,
		ZeroScoreBP:    zeroScore,
		ShiftGainBP:    bestScore - zeroScore,
		BiasRatioBP:    stat.biasRatioBP,
		LumaStep:       stat.lumaStep,
		StructuredDiff: stat.structuredDiff,
		TextureMean:    stat.textureMean,
	}, true
}

func selectRows(candidates []rowCandidate, separation, limit int) []int {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].row < candidates[j].row
	})
	if separation < 1 {
		separation = 1
	}
	if limit <= 0 {
		limit = len(candidates)
	}
	chosen := make([]int, 0, limit)
	for _, candidate := range candidates {
		if len(chosen) >= limit {
			break
		}
		tooClose := false
		for _, row := range chosen {
			if abs(candidate.row-row) < separation {
				tooClose = true
				break
			}
		}
		if !tooClose {
			chosen = append(chosen, candidate.row)
		}
	}
	sort.Ints(chosen)
	return chosen
}

func pairBands(seams []Seam, settings Settings) []Band {
	type pair struct {
		first, second int
		score         int
		gap           int
	}
	var pairs []pair
	for i := 0; i < len(seams); i++ {
		for j := i + 1; j < len(seams); j++ {
			left, right := seams[i], seams[j]
			if left.Row > right.Row {
				left, right = right, left
			}
			if right.Row-left.Row < settings.MinBandHeightPX {
				continue
			}
			if left.OffsetPX*right.OffsetPX >= 0 {
				continue
			}
			if abs(left.OffsetPX+right.OffsetPX) > settings.BandPairTolerancePX {
				continue
			}
			score := left.MatchScoreBP
			if right.MatchScoreBP < score {
				score = right.MatchScoreBP
			}
			pairs = append(pairs, pair{first: i, second: j, score: score, gap: right.Row - left.Row})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].score != pairs[j].score {
			return pairs[i].score > pairs[j].score
		}
		if pairs[i].gap != pairs[j].gap {
			return pairs[i].gap > pairs[j].gap
		}
		if pairs[i].first != pairs[j].first {
			return pairs[i].first < pairs[j].first
		}
		return pairs[i].second < pairs[j].second
	})

	used := make(map[int]bool, len(seams))
	var bands []Band
	for _, candidate := range pairs {
		if used[candidate.first] || used[candidate.second] {
			continue
		}
		left, right := seams[candidate.first], seams[candidate.second]
		if left.Row > right.Row {
			left, right = right, left
		}
		width := abs(left.OffsetPX)
		if abs(right.OffsetPX) > width {
			width = abs(right.OffsetPX)
		}
		bands = append(bands, Band{
			TopRow:         left.Row,
			BottomRow:      right.Row,
			HeightPX:       right.Row - left.Row,
			TopOffsetPX:    left.OffsetPX,
			BottomOffsetPX: right.OffsetPX,
			WidthPX:        width,
		})
		used[candidate.first] = true
		used[candidate.second] = true
	}
	return bands
}

func shiftOrder(maxShift int) []int {
	order := make([]int, 0, 2*maxShift+1)
	order = append(order, 0)
	for distance := 1; distance <= maxShift; distance++ {
		order = append(order, -distance, distance)
	}
	return order
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func min64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
