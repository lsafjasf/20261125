package aes3

import "math"

type candidate struct {
	subframe  Subframe
	startEdge int
	score     float64
}

type decoder struct {
	edges []Edge
	wf    *waveform
	h     float64
	opt   Options
}

func (d *decoder) findCandidates() []candidate {
	var out []candidate
	for i, e := range d.edges {
		if !validTime(e.Time) {
			continue
		}
		// Slot 31 starts at half-index 62 and its boundary always creates an
		// edge. Earlier capture ends cannot contain a complete subframe.
		lastTime := d.edges[len(d.edges)-1].Time
		if lastTime < e.Time+62*d.h {
			continue
		}

		var states [8]Level
		for k := 0; k < 8; k++ {
			states[k] = d.wf.levelAt(e.Time + (float64(k)+0.5)*d.h)
		}
		p := matchPreamble(states)
		if p == nil {
			continue
		}
		c, ok := d.decodeAt(i, p)
		if ok {
			out = append(out, c)
		}
	}
	return out
}

func (d *decoder) decodeAt(startEdge int, p *preamble) (candidate, bool) {
	t0 := d.edges[startEdge].Time
	tol := d.opt.JitterTolerance * d.h
	slotBits := make([]int, 28)
	expected := []float64{t0}

	for k := 1; k < 8; k++ {
		if p.states[k] != p.states[k-1] {
			expected = append(expected, t0+float64(k)*d.h)
		}
	}

	for j := 0; j < 28; j++ {
		slot := 4 + j
		half := 2 * slot
		boundary := t0 + float64(half)*d.h
		if _, dist := d.wf.nearestEdge(boundary); dist <= tol {
			expected = append(expected, boundary)
		} else {
			// Keep the missing expectation in the list so score reports it.
			expected = append(expected, boundary)
		}

		first := d.wf.levelAt(t0 + (float64(half)+0.5)*d.h)
		second := d.wf.levelAt(t0 + (float64(half)+1.5)*d.h)
		bit := 0
		if first != second {
			bit = 1
			expected = append(expected, t0+float64(half+1)*d.h)
		}
		slotBits[j] = bit
	}

	matched := 0
	usedEdge := make(map[int]bool, len(expected))
	for _, at := range expected {
		if idx, dist := d.wf.nearestEdge(at); dist <= tol && idx >= 0 {
			matched++
			usedEdge[idx] = true
		}
	}

	end := t0 + 64*d.h
	extra := 0
	// Edges exactly at end belong to the following preamble and are excluded.
	for i := startEdge; i < len(d.edges); i++ {
		at := d.edges[i].Time
		if at >= end-tol {
			break
		}
		if at < t0-d.h*1e-9 {
			continue
		}
		if !usedEdge[i] {
			extra++
		}
	}

	score := float64(matched) / float64(len(expected)+extra)
	errs := []string{}
	if matched != len(expected) {
		errs = append(errs, "missing or jittered BMC boundary/midpoint edges")
	}
	if extra != 0 {
		errs = append(errs, "unexpected edges inside subframe")
	}

	parityOnes := 0
	for _, b := range slotBits {
		parityOnes += b
	}
	parityOK := parityOnes%2 == 0
	if !parityOK {
		errs = append(errs, "AES3 even-parity check failed")
	}
	if d.opt.RequireParity && !parityOK {
		return candidate{}, false
	}
	if score+1e-12 < d.opt.MinSubframeScore {
		return candidate{}, false
	}

	audio := decodeAudio(slotBits[:24])
	validityBit := intToBool(slotBits[24])
	subframeNumber, label := 1, "L"
	if p.channel == "right" {
		subframeNumber, label = 2, "R"
	}
	sf := Subframe{
		SubframeNumber:   subframeNumber,
		ChannelLabel:     label,
		Channel:          p.channel,
		Code:             p.code,
		BlockStart:       p.block,
		StartTime:        t0,
		EndTime:          end,
		StartEdge:        startEdge,
		HalfUnit:         d.h,
		Score:            roundScore(score),
		SlotBits:         slotBits,
		Audio:            audio,
		ValidityBit:      validityBit,
		AudioValid:       !validityBit,
		UserBit:          intToBool(slotBits[25]),
		ChannelStatusBit: intToBool(slotBits[26]),
		ParityBit:        intToBool(slotBits[27]),
		ParityOK:         parityOK,
		Trusted:          score >= 1-1e-12 && parityOK,
		Errors:           errs,
	}
	return candidate{subframe: sf, startEdge: startEdge, score: score}, true
}

func decodeAudio(bits []int) Audio {
	raw := int32(0)
	for i, b := range bits {
		if b != 0 {
			raw |= 1 << i
		}
	}
	sample := raw
	if sample&(1<<23) != 0 {
		sample -= 1 << 24
	}

	aux := 0
	for i := 0; i < 4; i++ {
		aux |= bits[i] << i
	}
	raw20 := int32(0)
	for i := 4; i < 24; i++ {
		if bits[i] != 0 {
			raw20 |= 1 << (i - 4)
		}
	}
	sample20 := raw20
	if sample20&(1<<19) != 0 {
		sample20 -= 1 << 20
	}

	return Audio{
		Bits:       append([]int(nil), bits...),
		Raw24:      raw,
		Sample24:   sample,
		Sample20:   sample20,
		Auxiliary4: aux,
	}
}

func roundScore(v float64) float64 {
	return math.Round(v*1e9) / 1e9
}
