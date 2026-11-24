package repack

import (
	"fmt"
	"os"
	"path/filepath"
)

// GenerateSamplePacks writes one valid pack per language into dir. Every pack
// contains the same video frames and chapters, language-specific audio, and
// subtitle cues aligned to that pack's audio samples.
func GenerateSamplePacks(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create sample dir: %w", err)
	}
	languages := []string{"en", "fr", "de"}
	for _, lang := range languages {
		boxes := buildSampleBoxes(lang)
		raw, err := EncodePack(boxes)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, "pack-"+lang+".mcpack")
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// buildSampleBoxes creates deterministic content for one language.
func buildSampleBoxes(lang string) []Box {
	const (
		width  = 320
		height = 240
		step   = int64(1_000_000) // one frame per second
		frames = 4
	)
	boxes := make([]Box, 0, frames*3+2)
	for i := 0; i < frames; i++ {
		start := int64(i) * step
		end := start + step
		boxes = append(boxes, Box{
			Type:    TypeVideo,
			StartUs: start,
			EndUs:   end,
			Width:   width,
			Height:  height,
			Payload: []byte(fmt.Sprintf("frame-%d", i)),
		})
	}
	for i := 0; i < frames; i++ {
		start := int64(i) * step
		end := start + step
		boxes = append(boxes, Box{
			Type:     TypeAudio,
			Language: lang,
			StartUs:  start,
			EndUs:    end,
			Payload:  []byte(fmt.Sprintf("audio-%s-%d", lang, i)),
		})
	}
	for i := 0; i < frames; i++ {
		start := int64(i) * step
		end := start + step
		boxes = append(boxes, Box{
			Type:     TypeSubtitle,
			Language: lang,
			StartUs:  start,
			EndUs:    end,
			Payload:  []byte(fmt.Sprintf("sub-%s-%d", lang, i)),
		})
	}
	chapters := []struct {
		start int64
		end   int64
		name  string
	}{
		{0, 2 * step, "intro"},
		{2 * step, 4 * step, "main"},
	}
	for _, ch := range chapters {
		boxes = append(boxes, Box{
			Type:    TypeChapter,
			StartUs: ch.start,
			EndUs:   ch.end,
			Payload: []byte(ch.name),
		})
	}
	return boxes
}
