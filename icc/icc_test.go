package icc

import (
	"math"
	"strings"
	"testing"

	"icccmp/icc/fixture"
)

func buildProfile(t *testing.T, b *fixture.Builder) *Profile {
	t.Helper()
	return Parse(b.Build(), "synthetic")
}

func baseBuilder(ver uint8) *fixture.Builder {
	return fixture.New(ver, 4).
		AddXYZ("wtpt", 0.9642, 1.0, 0.8249).
		AddXYZ("rXYZ", 0.43, 0.22, 0.01).
		AddXYZ("gXYZ", 0.38, 0.71, 0.09).
		AddXYZ("bXYZ", 0.14, 0.06, 0.71).
		AddGamma("rTRC", 2.2).
		AddGamma("gTRC", 2.2).
		AddGamma("bTRC", 2.2)
}

func TestParseVersionAndHeader(t *testing.T) {
	p := buildProfile(t, fixture.New(4, 3).
		AddXYZ("wtpt", 0.9642, 1.0, 0.8249).
		AddGamma("rTRC", 2.2))
	if p.Version.Major != 4 || p.Version.Minor != 3 {
		t.Fatalf("version = %s, want 4.3", p.Version)
	}
	if math.Abs(p.HeaderWP.X-0.9642) > 1e-4 {
		t.Fatalf("header white point X = %v", p.HeaderWP.X)
	}
	if len(p.Issues) != 0 {
		t.Fatalf("unexpected issues: %v", p.Issues)
	}
}

func TestGammaAndLUTCurve(t *testing.T) {
	lut := make([]float64, 256)
	for i := range lut {
		lut[i] = math.Pow(float64(i)/255.0, 2.2)
	}
	p := buildProfile(t, baseBuilder(2).AddLUT("bTRC", lut))

	rTag, _ := p.FindTag("rTRC")
	if !rTag.Loaded || rTag.TRC.Kind != TRCGamma {
		t.Fatalf("rTRC not parsed as gamma: %+v", rTag)
	}
	// The gamma itself is stored as u8Fixed8, so compare against the decoded value.
	wantGamma := math.Round(2.2*256) / 256
	if got, _ := rTag.TRC.Eval(0.5); math.Abs(got-math.Pow(0.5, wantGamma)) > 1e-12 {
		t.Fatalf("gamma eval = %v, want %v", got, math.Pow(0.5, wantGamma))
	}

	bTag, _ := p.FindTag("bTRC")
	if bTag.TRC.Kind != TRCLUT {
		t.Fatalf("bTRC kind = %v", bTag.TRC.Kind)
	}
	if got, _ := bTag.TRC.Eval(0.0); math.Abs(got) > 1e-12 {
		t.Fatalf("LUT at 0 = %v, want 0", got)
	}
	if got, _ := bTag.TRC.Eval(1.0); math.Abs(got-1.0) > 1e-12 {
		t.Fatalf("LUT at 1 = %v, want 1", got)
	}
}

func TestParaType3Srgb(t *testing.T) {
	params := []float64{2.4, 1.0 / 1.055, 0.055 / 1.055, 1.0 / 12.92, 0.04045}
	p := buildProfile(t, baseBuilder(4).AddPara("rTRC", 3, params))
	tag, _ := p.FindTag("rTRC")
	if tag.TRC.Kind != TRCPara || tag.TRC.ParaType != 3 {
		t.Fatalf("rTRC = %+v", tag.TRC)
	}
	// Below the linear-section threshold d=0.04045 the curve is c*x.
	got, _ := tag.TRC.Eval(0.01)
	want := params[3] * 0.01
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("linear branch = %v, want %v", got, want)
	}
	// Above the threshold the power branch dominates.
	hi, _ := tag.TRC.Eval(0.5)
	wantHi := math.Pow(params[1]*0.5+params[2], params[0])
	if math.Abs(hi-wantHi) > 1e-5 {
		t.Fatalf("power branch = %v, want %v", hi, wantHi)
	}
}

func TestDuplicateTagFlagged(t *testing.T) {
	p := buildProfile(t, baseBuilder(2).Duplicate("rTRC"))
	dups := p.DuplicateTags()
	if dups["rTRC"] != 2 {
		t.Fatalf("duplicate count = %v, want 2", dups)
	}
	found := false
	for _, issue := range p.Issues {
		if strings.Contains(issue, "duplicate tag") {
			found = true
		}
	}
	if !found {
		t.Fatalf("duplicate not flagged: %v", p.Issues)
	}
}

func TestWrongLengthXYZ(t *testing.T) {
	p := buildProfile(t, baseBuilder(4).SetWrongLength("rXYZ", 18))
	tag, _ := p.FindTag("rXYZ")
	if tag.Loaded {
		t.Fatal("rXYZ should not be loaded with wrong length")
	}
	if !issueContains(p, "XYZ length 18 is not 20") {
		t.Fatalf("wrong-length issue missing: %v", p.Issues)
	}
}

func TestIllegalNonMonotonicLUT(t *testing.T) {
	lut := make([]float64, 16)
	for i := range lut {
		lut[i] = math.Pow(float64(i)/15.0, 2.2)
	}
	lut[10], lut[9] = lut[9], lut[10]
	p := buildProfile(t, baseBuilder(2).AddLUT("rTRC", lut))
	tag, _ := p.FindTag("rTRC")
	if tag.Loaded {
		t.Fatal("illegal LUT should not be loaded")
	}
	if !issueContains(p, "not monotonic") {
		t.Fatalf("illegal-curve issue missing: %v", p.Issues)
	}
}

func TestIllegalParaType(t *testing.T) {
	// Function type 5 sits outside the supported 0..4 range.
	raw := make([]byte, 12+8*4)
	copy(raw[0:4], "para")
	raw[8] = 0
	raw[9] = 5 // function type 5
	p2 := buildProfile(t, baseBuilder(4).AddRaw("rTRC", raw))
	tag, _ := p2.FindTag("rTRC")
	if tag.Loaded {
		t.Fatal("para type 5 must be rejected")
	}
	if !issueContains(p2, "function type 5") {
		t.Fatalf("illegal para issue missing: %v", p2.Issues)
	}
}

func TestOutOfRangeTag(t *testing.T) {
	p := buildProfile(t, baseBuilder(4).
		AddRaw("cprt", []byte("text    ")).
		SetOffset("cprt", 1<<20))
	if !issueContains(p, "exceeds file length") {
		t.Fatalf("out-of-range issue missing: %v", p.Issues)
	}
}

func TestUnsupportedTagMarked(t *testing.T) {
	p := buildProfile(t, baseBuilder(4).AddRaw("cprt",
		append([]byte("text"), make([]byte, 8)...)))
	rep := Compare(p, p)
	if len(rep.Unsupported.Left) != 1 || rep.Unsupported.Left[0].Signature != "cprt" {
		t.Fatalf("unsupported list = %+v", rep.Unsupported.Left)
	}
}

func TestWhitePointDeltaE(t *testing.T) {
	a := buildProfile(t, baseBuilder(2))
	b := buildProfile(t, fixture.New(4, 3).
		AddXYZ("wtpt", 0.95, 1.0, 0.82).
		AddGamma("rTRC", 2.2).
		AddGamma("gTRC", 2.2).
		AddGamma("bTRC", 2.2))
	rep := Compare(a, b)
	if rep.WhitePoint.DeltaE76 <= 0 {
		t.Fatalf("expected positive delta E, got %v", rep.WhitePoint.DeltaE76)
	}
}

func TestLUTQuantizationAgainstAnalytic(t *testing.T) {
	decodedGamma := math.Round(2.2*256) / 256 // what the analytic gamma tag actually stores
	lut := make([]float64, 32)
	for i := range lut {
		lut[i] = math.Pow(float64(i)/31.0, decodedGamma)
	}
	a := buildProfile(t, baseBuilder(2).AddLUT("bTRC", lut))
	b := buildProfile(t, baseBuilder(4))
	rep := Compare(a, b)
	var bCurve *CurveStats
	for i := range rep.Curves {
		if rep.Curves[i].Channel == "b" {
			bCurve = &rep.Curves[i]
		}
	}
	if bCurve.LeftQuant == nil || !bCurve.LeftQuant.Present {
		t.Fatal("missing LUT quantization stats")
	}
	// LUT and analytic partner now describe the same gamma, so the residual
	// is purely u16Fixed16 rounding and must stay within one code step.
	if bCurve.LeftQuant.MaxAbs > 4.0/65535.0 {
		t.Fatalf("quantization max %v far exceeds u16 rounding scale", bCurve.LeftQuant.MaxAbs)
	}
}

func issueContains(p *Profile, sub string) bool {
	for _, issue := range p.Issues {
		if strings.Contains(issue, sub) {
			return true
		}
	}
	return false
}
