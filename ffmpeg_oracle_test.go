package mp4diag_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestFFmpegFrameHashOracle(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available; static GOP range is still tested by manifest")
	}

	clean := frameMD5Hashes(t, "testdata/clean.mp4")
	compressed := frameMD5Hashes(t, "injections/03-compressed-sample2-display-2-4.mp4")
	if len(clean) != 8 || len(compressed) != 8 {
		t.Fatalf("frame counts clean=%d compressed=%d, want 8/8", len(clean), len(compressed))
	}
	var diff []int
	for i := range clean {
		if clean[i] != compressed[i] {
			diff = append(diff, i+1)
		}
	}
	assertInts(t, "ffmpeg compressed display diff", diff, []int{2, 3, 4})

	truncated := frameMD5Hashes(t, "injections/01-container-first-bad-box.mp4")
	if len(truncated) != 5 {
		t.Fatalf("truncated decoded frames = %d, want 5", len(truncated))
	}
	for i := range truncated {
		if truncated[i] != clean[i] {
			t.Fatalf("truncated frame %d hash differs from clean prefix", i+1)
		}
	}
}

func frameMD5Hashes(t *testing.T, path string) []string {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", path, "-f", "framemd5", "-")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Logf("ffmpeg stderr: %s", stderr.String())
	}
	var hashes []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] < '0' || line[0] > '9' {
			continue
		}
		parts := strings.Split(line, ",")
		hashes = append(hashes, strings.TrimSpace(parts[len(parts)-1]))
	}
	if len(hashes) == 0 {
		t.Fatalf("no framemd5 hashes for %s: %s", path, stderr.String())
	}
	return hashes
}

func assertInts(t *testing.T, name string, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
}
