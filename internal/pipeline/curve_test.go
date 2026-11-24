package pipeline

import (
	"math"
	"testing"

	"rec2020-to-rec709-shoulder/internal/colorconv"
)

func TestSoftShoulderProperties(t *testing.T) {
	knee := 0.72
	if got := SoftShoulder(0.5, knee); got != 0.5 {
		t.Fatalf("below knee must be identity, got %v", got)
	}
	if got := SoftShoulder(knee, knee); math.Abs(got-knee) > 1e-12 {
		t.Fatalf("shoulder must be continuous at knee, got %v", got)
	}
	prev := -1.0
	for x := 0.0; x <= 10; x += 0.01 {
		y := SoftShoulder(x, knee)
		if y < prev {
			t.Fatalf("shoulder not monotonic at x=%v", x)
		}
		if y >= 1 {
			t.Fatalf("asymptotic shoulder must stay below 1 at x=%v", x)
		}
		prev = y
	}
	if got := SoftShoulder(100, knee); got < 0.999 {
		t.Fatalf("shoulder should approach ceiling, got %v", got)
	}
}

func TestHardClipAndGamutCompress(t *testing.T) {
	got := MapWorkingColor(colorconv.RGB{R: 2, G: 0.2, B: 0.4}, RenderParams{Variant: VariantHard})
	if got.R != 1 || got.G != 0.2 || got.B != 0.4 {
		t.Fatalf("unexpected hard map: %+v", got)
	}

	in := colorconv.RGB{R: -0.4, G: 0.8, B: 0.2}
	lumaBefore := colorconv.Luminance709(in)
	compressed := GamutCompress(in)
	if compressed.R < -1e-12 {
		t.Fatalf("negative channel remains: %+v", compressed)
	}
	lumaAfter := colorconv.Luminance709(compressed)
	if math.Abs(lumaBefore-lumaAfter) > 1e-12 {
		t.Fatalf("gamut compression changed luma: %v -> %v", lumaBefore, lumaAfter)
	}
}

func TestRenderFrameBounds(t *testing.T) {
	src := NewFloatImage(2, 2)
	for i := range src.Pix {
		src.Pix[i] = []float64{0, 0.8, 2.5, -0.2}[i%4]
	}
	for _, variant := range []string{VariantHard, VariantFixed, VariantAdaptive} {
		out := RenderFrame(src, RenderParams{Variant: variant, Knee: .7})
		for _, v := range out.Pix {
			if v < 0 || v > 1 {
				t.Fatalf("%s produced out-of-bounds value %v", variant, v)
			}
		}
	}
}
