package transparentvideo

import "fmt"

func VerifyVideo(video *Video) error {
	if err := validateVideoShape(video); err != nil {
		return err
	}

	expectedSource := expectedSourceColor(video.Mode)
	black := video.Frames[1]
	white := video.Frames[2]
	expectedBlack := Color{R: 127, G: 0, B: 0, A: 255}
	expectedWhite := Color{R: 255, G: 128, B: 128, A: 255}

	for index := 0; index < len(video.Frames[0].Pix); index += 4 {
		source := video.Frames[0]
		if got := colorAt(source.Pix, index); got != expectedSource {
			return fmt.Errorf("source pixel %d is %v, want %v for mode %q", index/4, got, expectedSource, video.Mode)
		}
		if got := colorAt(black.Pix, index); got != expectedBlack {
			return fmt.Errorf("black-background pixel %d is %v, want %v", index/4, got, expectedBlack)
		}
		if got := colorAt(white.Pix, index); got != expectedWhite {
			return fmt.Errorf("white-background pixel %d is %v, want %v", index/4, got, expectedWhite)
		}
	}
	return nil
}

func expectedSourceColor(mode AlphaMode) Color {
	if mode == AlphaPremultiplied {
		return Color{R: 127, G: 0, B: 0, A: 127}
	}
	return Color{R: 255, G: 0, B: 0, A: 127}
}

func colorAt(pix []byte, offset int) Color {
	return Color{
		R: pix[offset],
		G: pix[offset+1],
		B: pix[offset+2],
		A: pix[offset+3],
	}
}
