package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type videoMeta struct {
	Width  int
	Height int
	FPS    float64
}

type ffprobeReport struct {
	Streams []struct {
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
		RFrameRate   string `json:"r_frame_rate"`
	} `json:"streams"`
}

func isVideoPath(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mp4", ".mkv", ".mov", ".avi", ".webm", ".ts", ".m2ts", ".flv", ".wmv", ".m4v":
		return true
	default:
		return false
	}
}

func isRawPath(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".yuv", ".i420", ".raw", ".rgb", ".rgb24", ".gray", ".y8":
		return true
	default:
		return false
	}
}

func probeVideo(path string) (videoMeta, error) {
	bin := envOr("FFPROBE_BIN", "ffprobe")
	cmd := exec.Command(bin, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height,avg_frame_rate,r_frame_rate", "-of", "json", path)
	out, err := cmd.Output()
	if err != nil {
		return videoMeta{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}
	var report ffprobeReport
	if err := json.Unmarshal(out, &report); err != nil {
		return videoMeta{}, err
	}
	if len(report.Streams) == 0 {
		return videoMeta{}, fmt.Errorf("ffprobe found no video stream in %s", path)
	}
	s := report.Streams[0]
	fps := parseRate(s.AvgFrameRate)
	if fps <= 0 {
		fps = parseRate(s.RFrameRate)
	}
	return videoMeta{Width: s.Width, Height: s.Height, FPS: fps}, nil
}

func parseRate(v string) float64 {
	parts := strings.Split(strings.TrimSpace(v), "/")
	if len(parts) == 1 {
		f, _ := strconv.ParseFloat(parts[0], 64)
		return f
	}
	a, err1 := strconv.ParseFloat(parts[0], 64)
	b, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || b == 0 {
		return 0
	}
	return a / b
}

func startVideoReader(path string) (io.ReadCloser, func() error, error) {
	bin := envOr("FFMPEG_BIN", "ffmpeg")
	cmd := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-i", path,
		"-f", "rawvideo", "-pix_fmt", "rgb24", "pipe:1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start %s: %w (set FFMPEG_BIN or decode to PNG/raw)", bin, err)
	}
	cleanup := func() error {
		_ = stdout.Close()
		return cmd.Wait()
	}
	return stdout, cleanup, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
