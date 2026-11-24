package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// VideoMeta holds the container/stream parameters reported by ffprobe.
type VideoMeta struct {
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	Codec      string   `json:"codec"`
	Profile    string   `json:"profile"`
	PixFmt     string   `json:"pix_fmt"`
	FrameRate  float64  `json:"frame_rate"`
	Duration   float64  `json:"duration_sec"`
	NbFrames   int      `json:"nb_frames"`
	BitRate    int64    `json:"bit_rate_bps"`
	HasBFrames bool     `json:"has_b_frames"`
	Keyframes  []int    `json:"keyframes"`
	ColorTags  []string `json:"color_tags"`
}

type ffprobeOut struct {
	Streams []struct {
		CodecName  string `json:"codec_name"`
		Profile    string `json:"profile"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		PixFmt     string `json:"pix_fmt"`
		AvgFR      string `json:"avg_frame_rate"`
		RFR        string `json:"r_frame_rate"`
		NbFrames   string `json:"nb_frames"`
		BitRate    string `json:"bit_rate"`
		HasBFrames int    `json:"has_b_frames"`
		Pix        []struct {
			Left int    `json:"left"`
			Top  int    `json:"top"`
			Key  int    `json:"key"`
			Val  string `json:"value"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
		Tags     map[string]string
	} `json:"format"`
}

// probeMeta runs ffprobe and parses the first video stream.
func probeMeta(video string) (*VideoMeta, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_streams",
		"-show_format", "-print_format", "json", video)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}
	var pr ffprobeOut
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, err
	}
	idx := -1
	for i, s := range pr.Streams {
		if s.Width > 0 && s.Height > 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("no video stream in %s", video)
	}
	s := pr.Streams[idx]
	meta := &VideoMeta{
		Width: s.Width, Height: s.Height, Codec: s.CodecName,
		Profile: s.Profile, PixFmt: s.PixFmt,
		FrameRate:  parseFraction(firstNonEmpty(s.AvgFR, s.RFR)),
		Duration:   parseFloat(pr.Format.Duration),
		NbFrames:   int(parseFloat(s.NbFrames)),
		BitRate:    int64(parseFloat(firstNonEmpty(s.BitRate, pr.Format.BitRate))),
		HasBFrames: s.HasBFrames > 0,
	}
	return meta, nil
}

// probeKeyframes returns 0-based frame indices flagged as keyframes.
func probeKeyframes(video string) ([]int, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "frame=key_frame,pkt_pts_time,pts_time",
		"-of", "csv=p=0", video)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var keys []int
	idx := 0
	for _, line := range splitLines(string(out)) {
		if line == "" {
			continue
		}
		// Each row: key_flag,pts_time
		if line[0] == '1' {
			keys = append(keys, idx)
		}
		idx++
	}
	return keys, nil
}

// extractFrames decodes the video to PNG frames; reuses cache when present.
func extractFrames(video, cacheDir string, fps float64) ([]*Frame, error) {
	if pngs, err := loadPNGSequence(cacheDir, fps); err == nil && len(pngs) > 0 {
		return pngs, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, err
	}
	pattern := filepath.Join(cacheDir, "f_%06d.png")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-y", "-i", video, "-vsync", "0", "-pix_fmt", "gray", pattern)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg decode failed: %w\n%s", err, out)
	}
	return loadPNGSequence(cacheDir, fps)
}

// extractRawYUV decodes the video to a single planar I420 file for reference diff.
func extractRawYUV(video, rawPath string) error {
	if _, err := os.Stat(rawPath); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(rawPath), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-y", "-i", video, "-pix_fmt", "yuv420p", "-f", "rawvideo", rawPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg raw decode failed: %w\n%s", err, out)
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" && v != "0/0" {
			return v
		}
	}
	return "0/1"
}

func parseFraction(s string) float64 {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return parseFloat(s[:i]) / parseFloat(s[i+1:])
		}
	}
	return parseFloat(s)
}

func parseFloat(s string) float64 {
	v := 0.0
	neg := false
	i := 0
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	scale := 1.0
	frac := false
	for ; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			frac = true
			continue
		}
		if c < '0' || c > '9' {
			break
		}
		d := float64(c - '0')
		if frac {
			scale *= 0.1
			v += d * scale
		} else {
			v = v*10 + d
		}
	}
	if neg {
		return -v
	}
	return v
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, trimCR(s[start:i]))
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, trimCR(s[start:]))
	}
	return lines
}

func trimCR(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\r' {
		return s[:len(s)-1]
	}
	return s
}
