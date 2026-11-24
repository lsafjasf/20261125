package media

import (
	"math"
	"path/filepath"
	"testing"

	"rec2020-to-rec709-shoulder/internal/pipeline"
)

func TestPFMRoundTrip(t *testing.T) {
	im := pipeline.NewFloatImage(3, 2)
	copy(im.Pix, []float64{0, .25, 1.5, 2, .5, 0, .1, .2, .3, 3, 2, 1, .4, .5, .6, 0, 0, 0})
	path := filepath.Join(t.TempDir(), "frame.pfm")
	if err := WritePFM(path, im); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPFM(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 3 || got.Height != 2 {
		t.Fatalf("bad dimensions %dx%d", got.Width, got.Height)
	}
	for i := range im.Pix {
		if math.Abs(got.Pix[i]-im.Pix[i]) > 1e-6 {
			t.Fatalf("pixel %d = %v, want %v", i, got.Pix[i], im.Pix[i])
		}
	}
}
