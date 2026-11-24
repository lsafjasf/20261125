package preview

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"

	"surrounddiag/internal/wav"
)

type Item struct {
	Channel string  `json:"channel"`
	Index   int     `json:"index"`
	File    string  `json:"file"`
	Start   float64 `json:"start_seconds"`
	End     float64 `json:"end_seconds"`
	RMSDB   float64 `json:"rms_dbfs"`
}

type Manifest struct {
	Source      string `json:"source"`
	SampleRate  int    `json:"sample_rate"`
	BitDepth    int    `json:"preview_bit_depth"`
	ChannelMode string `json:"channel_mode"`
	Items       []Item `json:"items"`
	Note        string `json:"note"`
}

func Export(file *wav.File, outDir string, startSeconds, endSeconds float64) (*Manifest, error) {
	if endSeconds <= 0 || endSeconds > file.Duration.Seconds() {
		endSeconds = file.Duration.Seconds()
	}
	if startSeconds < 0 {
		startSeconds = 0
	}
	if endSeconds <= startSeconds {
		return nil, fmt.Errorf("empty audition range %.3f–%.3f s", startSeconds, endSeconds)
	}
	start := int(math.Round(startSeconds * float64(file.SampleRate)))
	end := int(math.Round(endSeconds * float64(file.SampleRate)))
	if end > file.Frames {
		end = file.Frames
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	manifest := &Manifest{
		Source: file.Path, SampleRate: file.SampleRate, BitDepth: 24,
		ChannelMode: "mono extract; source PCM is not modified",
		Note:        "每个文件只包含一个声道，无降噪、无增益、无下混。",
	}
	for index, name := range file.Layout {
		samples := make([]float64, end-start)
		for frame := start; frame < end; frame++ {
			samples[frame-start] = file.Data[frame*file.Channels+index]
		}
		filename := fmt.Sprintf("%02d-%s.wav", index+1, safeName(name))
		target := filepath.Join(outDir, filename)
		if err := wav.WriteMono24(target, file.SampleRate, samples); err != nil {
			return nil, err
		}
		manifest.Items = append(manifest.Items, Item{
			Channel: name, Index: index, File: target,
			Start: startSeconds, End: endSeconds, RMSDB: rmsDB(samples),
		})
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	return manifest, nil
}

func Play(manifest *Manifest, player string) error {
	if player == "" {
		return nil
	}
	for _, item := range manifest.Items {
		fmt.Printf("PLAY %s: %s\n", item.Channel, item.File)
		command := exec.Command(player, item.File)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("player failed for %s: %w", item.Channel, err)
		}
	}
	return nil
}

func safeName(name string) string {
	out := make([]rune, 0, len(name))
	for _, char := range name {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9', char == '-', char == '_':
			out = append(out, char)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "channel"
	}
	return string(out)
}

func rmsDB(samples []float64) float64 {
	if len(samples) == 0 {
		return -120
	}
	var sum float64
	for _, sample := range samples {
		sum += sample * sample
	}
	rms := math.Sqrt(sum / float64(len(samples)))
	if rms < 1e-6 {
		return -120
	}
	return 20 * math.Log10(rms)
}
