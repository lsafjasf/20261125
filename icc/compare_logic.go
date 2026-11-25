package icc

import "math"

// Compare produces a full matrix/TRC comparison of two parsed profiles.
func Compare(left, right *Profile) *Report {
	rep := &Report{}
	rep.LeftSummary = summarize(left)
	rep.RightSummary = summarize(right)
	rep.WhitePoint = compareWhitePoint(left, right)

	for _, col := range []string{"r", "g", "b"} {
		rep.Matrix = append(rep.Matrix, compareMatrixCol(left, right, col))
	}
	for _, ch := range []string{"r", "g", "b"} {
		rep.Curves = append(rep.Curves, compareCurve(left, right, ch))
	}

	rep.Unsupported.Left = listUnsupported(left)
	rep.Unsupported.Right = listUnsupported(right)
	return rep
}

func summarize(p *Profile) ProfileSummary {
	s := ProfileSummary{
		Path:       p.Path,
		Size:       p.Size,
		Version:    p.Version.String(),
		Class:      p.Class,
		ColorSpace: p.ColorSpace,
		PCS:        p.PCS,
		TagCount:   p.TagCount(),
		Issues:     append([]string{}, p.Issues...),
		TagIssues:  map[string][]string{},
	}
	for _, t := range p.Tags {
		if len(t.Issues) > 0 {
			s.TagIssues[t.Signature] = append([]string{}, t.Issues...)
		}
	}
	return s
}

func compareWhitePoint(left, right *Profile) WhitePointReport {
	rep := WhitePointReport{
		LeftHeaderWP:  left.HeaderWP,
		RightHeaderWP: right.HeaderWP,
	}
	if tag, ok := left.FindTag("wtpt"); ok && tag.Loaded && tag.XYZ != nil {
		rep.LeftXYZ, rep.LeftSource = *tag.XYZ, "wtpt"
	} else {
		rep.LeftXYZ, rep.LeftSource = left.HeaderWP, "header illuminant"
	}
	if tag, ok := right.FindTag("wtpt"); ok && tag.Loaded && tag.XYZ != nil {
		rep.RightXYZ, rep.RightSource = *tag.XYZ, "wtpt"
	} else {
		rep.RightXYZ, rep.RightSource = right.HeaderWP, "header illuminant"
	}

	labL := xyzToLab(rep.LeftXYZ)
	labR := xyzToLab(rep.RightXYZ)
	rep.DeltaE76 = math.Sqrt(sq(labL[0]-labR[0]) + sq(labL[1]-labR[1]) + sq(labL[2]-labR[2]))
	return rep
}

// d50 is the PCS/adopted-white reference used for XYZ->Lab.
var d50 = XYZ{X: 0.9642, Y: 1.0, Z: 0.8249}

func xyzToLab(c XYZ) [3]float64 {
	f := func(t float64) float64 {
		const e = 216.0 / 24389.0
		const k = 24389.0 / 27.0
		if t > e {
			return math.Cbrt(t)
		}
		return (k*t + 16) / 116
	}
	fx := f(c.X / d50.X)
	fy := f(c.Y / d50.Y)
	fz := f(c.Z / d50.Z)
	return [3]float64{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}

func compareMatrixCol(left, right *Profile, ch string) MatrixCol {
	out := MatrixCol{Channel: ch}
	sig := ch + "XYZ"
	if tag, ok := left.FindTag(sig); ok && tag.Loaded && tag.XYZ != nil {
		out.LeftXYZ, out.LeftOK = *tag.XYZ, true
	}
	if tag, ok := right.FindTag(sig); ok && tag.Loaded && tag.XYZ != nil {
		out.RightXYZ, out.RightOK = *tag.XYZ, true
	}
	if out.LeftOK && out.RightOK {
		out.MaxAbsDiff = max3(
			math.Abs(out.LeftXYZ.X-out.RightXYZ.X),
			math.Abs(out.LeftXYZ.Y-out.RightXYZ.Y),
			math.Abs(out.LeftXYZ.Z-out.RightXYZ.Z))
	}
	return out
}

func compareCurve(left, right *Profile, ch string) CurveStats {
	sig := ch + "TRC"
	out := CurveStats{Channel: ch, LeftForm: "missing", RightForm: "missing"}

	lt, lok := usableTRC(left, sig)
	rt, rok := usableTRC(right, sig)
	if lok {
		out.LeftForm = lt.Describe()
	}
	if rok {
		out.RightForm = rt.Describe()
	}
	// Quantization is meaningful only for an encoded curv LUT; an analytic
	// partner curve (gamma/para) from either profile supplies ideal nodes.
	if lok && lt.Kind == TRCLUT {
		out.LeftQuant = lutQuantization(lt, analyticPartner(lt, rt))
	}
	if rok && rt.Kind == TRCLUT {
		out.RightQuant = lutQuantization(rt, analyticPartner(rt, lt))
	}

	if !lok || !rok {
		out.Comparable = false
		switch {
		case !lok && !rok:
			out.Reason = "both profiles lack a valid " + sig
		case !lok:
			out.Reason = "left profile lacks a valid " + sig
		default:
			out.Reason = "right profile lacks a valid " + sig
		}
		return out
	}

	out.Comparable = true
	out.Stats = sampleDiff(lt, rt)
	return out
}

// analyticPartner returns other when it is analytic (not a LUT), so it can
// serve as the ideal reference for quantizing the LUT; nil otherwise.
func analyticPartner(lut, other *TRC) *TRC {
	if other != nil && other.Kind != TRCLUT {
		return other
	}
	return nil
}

func usableTRC(p *Profile, sig string) (*TRC, bool) {
	tag, ok := p.FindTag(sig)
	if !ok || !tag.Loaded || tag.TRC == nil {
		return nil, false
	}
	return tag.TRC, true
}

func sampleDiff(a, b *TRC) Stats {
	var sumSq, sumAbs, maxAbs float64
	for i := 0; i < SamplePoints; i++ {
		x := float64(i) / float64(SamplePoints-1)
		va, _ := a.Eval(x)
		vb, _ := b.Eval(x)
		d := va - vb
		sumSq += sq(d)
		sumAbs += math.Abs(d)
		if math.Abs(d) > maxAbs {
			maxAbs = math.Abs(d)
		}
	}
	n := float64(SamplePoints)
	return Stats{MaxAbs: maxAbs, RMS: math.Sqrt(sumSq / n), MeanAbs: sumAbs / n, Samples: SamplePoints}
}

func listUnsupported(p *Profile) []UnsupportedTag {
	var out []UnsupportedTag
	for _, t := range p.Tags {
		if t.Kind == TagUnsupported {
			out = append(out, UnsupportedTag{Signature: t.Signature, Offset: t.Offset, Size: t.Size, Type: t.Type})
		}
	}
	return out
}

func sq(v float64) float64 { return v * v }

func max3(a, b, c float64) float64 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}
