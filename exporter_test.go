package transparentvideo

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSetModeConvertsPixels(t *testing.T) {
	exporter, err := New(Config{Width: 2, Height: 2, Mode: AlphaStraight})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	want := expectedPix(2, 2, expectedSourceColor(AlphaStraight))
	if got := exporter.SourceFrame().Pix; !bytes.Equal(got, want) {
		t.Fatalf("straight source = %v, want %v", got, want)
	}

	if err := exporter.SetMode(AlphaPremultiplied); err != nil {
		t.Fatalf("SetMode returned error: %v", err)
	}
	if exporter.Mode() != AlphaPremultiplied {
		t.Fatalf("Mode = %q, want %q", exporter.Mode(), AlphaPremultiplied)
	}
	if got := exporter.SourceFrame().Pix; !bytes.Equal(got, expectedPix(2, 2, expectedSourceColor(AlphaPremultiplied))) {
		t.Fatalf("premultiplied source = %v, want %v", got, expectedPix(2, 2, expectedSourceColor(AlphaPremultiplied)))
	}

	if err := exporter.SetMode(AlphaStraight); err != nil {
		t.Fatalf("SetMode returned error: %v", err)
	}
	if exporter.Mode() != AlphaStraight {
		t.Fatalf("Mode = %q, want %q", exporter.Mode(), AlphaStraight)
	}
	if got := exporter.SourceFrame().Pix; !bytes.Equal(got, expectedPix(2, 2, expectedSourceColor(AlphaStraight))) {
		t.Fatalf("restored source = %v, want %v", got, expectedPix(2, 2, expectedSourceColor(AlphaStraight)))
	}

	video, err := exporter.BuildVideo()
	if err != nil {
		t.Fatalf("BuildVideo returned error: %v", err)
	}
	if video.Mode != AlphaStraight {
		t.Fatalf("restored video mode = %q, want %q", video.Mode, AlphaStraight)
	}
	wantBlack := expectedPix(2, 2, Color{R: 127, G: 0, B: 0, A: 255})
	wantWhite := expectedPix(2, 2, Color{R: 255, G: 128, B: 128, A: 255})
	if !bytes.Equal(video.Frames[1].Pix, wantBlack) {
		t.Fatalf("restored black frame = %v, want %v", video.Frames[1].Pix, wantBlack)
	}
	if !bytes.Equal(video.Frames[2].Pix, wantWhite) {
		t.Fatalf("restored white frame = %v, want %v", video.Frames[2].Pix, wantWhite)
	}
	if err := VerifyVideo(video); err != nil {
		t.Fatalf("VerifyVideo returned error: %v", err)
	}
}

func TestRenderBlackAndWhiteFrames(t *testing.T) {
	exporter, err := New(Config{Width: 3, Height: 1, Mode: AlphaPremultiplied})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	black, err := exporter.RenderTestFrame(Black)
	if err != nil {
		t.Fatalf("RenderTestFrame(black) returned error: %v", err)
	}
	white, err := exporter.RenderTestFrame(White)
	if err != nil {
		t.Fatalf("RenderTestFrame(white) returned error: %v", err)
	}

	wantBlack := expectedPix(3, 1, Color{R: 127, G: 0, B: 0, A: 255})
	wantWhite := expectedPix(3, 1, Color{R: 255, G: 128, B: 128, A: 255})
	if !bytes.Equal(black.Pix, wantBlack) {
		t.Fatalf("black frame = %v, want %v", black.Pix, wantBlack)
	}
	if !bytes.Equal(white.Pix, wantWhite) {
		t.Fatalf("white frame = %v, want %v", white.Pix, wantWhite)
	}
	if black.Purpose != PurposeBlack || white.Purpose != PurposeWhite {
		t.Fatalf("purposes = %q, %q; want %q, %q", black.Purpose, white.Purpose, PurposeBlack, PurposeWhite)
	}
}

func TestRenderTestFrameRejectsUnsupportedBackground(t *testing.T) {
	exporter, err := New(Config{Width: 1, Height: 1})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := exporter.RenderTestFrame(Color{R: 1, G: 255, B: 255, A: 255}); err == nil {
		t.Fatal("RenderTestFrame accepted a background that is neither black nor white")
	}
}

func TestEncodeDecodeAndVerifyVideo(t *testing.T) {
	for _, mode := range []AlphaMode{AlphaStraight, AlphaPremultiplied} {
		t.Run(string(mode), func(t *testing.T) {
			exporter, err := New(Config{Width: 2, Height: 3, Mode: mode})
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			video, err := exporter.BuildVideo()
			if err != nil {
				t.Fatalf("BuildVideo returned error: %v", err)
			}

			buffer := bytes.NewBuffer(nil)
			if _, err := Encode(buffer, video); err != nil {
				t.Fatalf("Encode returned error: %v", err)
			}
			decoded, err := Decode(buffer)
			if err != nil {
				t.Fatalf("Decode returned error: %v", err)
			}
			if decoded.Mode != mode {
				t.Fatalf("decoded mode = %q, want %q", decoded.Mode, mode)
			}
			if len(decoded.Frames) != 3 {
				t.Fatalf("frame count = %d, want 3", len(decoded.Frames))
			}
			if err := VerifyVideo(decoded); err != nil {
				t.Fatalf("VerifyVideo returned error: %v", err)
			}
		})
	}
}

func TestDecodeRejectsLabelWithoutPixelConversion(t *testing.T) {
	tests := []struct {
		name       string
		actualMode AlphaMode
		labelMode  AlphaMode
	}{
		{name: "straight pixels labeled premultiplied", actualMode: AlphaStraight, labelMode: AlphaPremultiplied},
		{name: "premultiplied pixels labeled straight", actualMode: AlphaPremultiplied, labelMode: AlphaStraight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter, err := New(Config{Width: 1, Height: 1, Mode: tt.actualMode})
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			video, err := exporter.BuildVideo()
			if err != nil {
				t.Fatalf("BuildVideo returned error: %v", err)
			}
			video.Mode = tt.labelMode

			buffer := bytes.NewBuffer(nil)
			if _, err := Encode(buffer, video); err != nil {
				t.Fatalf("Encode returned error: %v", err)
			}
			if _, err := Decode(buffer); err == nil {
				t.Fatal("Decode accepted changed mode label with unchanged pixels")
			}
		})
	}
}

func TestExportKeepsPreviousFileWhenVerificationFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "video.tvid")
	previous := []byte("previous successful export")
	if err := os.WriteFile(path, previous, 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	exporter, err := New(Config{Width: 1, Height: 1})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	exporter.verify = func(*Video) error {
		return bytes.ErrTooLarge
	}

	if err := exporter.Export(path); !errors.Is(err, bytes.ErrTooLarge) {
		t.Fatalf("Export error = %v, want %v", err, bytes.ErrTooLarge)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !bytes.Equal(current, previous) {
		t.Fatalf("current file = %q, want preserved previous file %q", current, previous)
	}
}

func TestExportReplacesPreviousFileAfterSuccessfulVerification(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "video.tvid")
	previous := []byte("previous successful export")
	if err := os.WriteFile(path, previous, 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	exporter, err := New(Config{Width: 2, Height: 2, Mode: AlphaPremultiplied})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := exporter.Export(path); err != nil {
		t.Fatalf("Export returned error: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer file.Close()
	decoded, err := Decode(file)
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if decoded.Mode != AlphaPremultiplied {
		t.Fatalf("decoded mode = %q, want %q", decoded.Mode, AlphaPremultiplied)
	}
	if err := VerifyVideo(decoded); err != nil {
		t.Fatalf("VerifyVideo returned error: %v", err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Fatalf("temporary files remain, entries = %v, err = %v", entries, err)
	}
}

func expectedPix(width, height int, color Color) []byte {
	pix := make([]byte, width*height*4)
	for index := 0; index < len(pix); index += 4 {
		pix[index] = color.R
		pix[index+1] = color.G
		pix[index+2] = color.B
		pix[index+3] = color.A
	}
	return pix
}
