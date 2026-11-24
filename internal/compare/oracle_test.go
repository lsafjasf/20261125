package compare

// This optional test uses a locally installed ffmpeg only as an independent
// oracle. Production code and the diagnostic decision path never invoke an
// external command.

import (
	"os/exec"
	"path/filepath"
	"testing"

	"ffv1compare/internal/framemd5"
	"ffv1compare/internal/testutil"
)

func TestFFmpegOracle(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	cases := []struct {
		media    string
		manifest string
		pixFmt   string
	}{
		{"machine-a.mkv", "hashes-a.framemd5", ""},
		{"machine-a.mkv", "hashes-a-rgb.framemd5", "rgb24"},
		{"machine-b-slices.mkv", "hashes-b-slices.framemd5", ""},
		{"machine-b-reorder.mkv", "hashes-b-reorder.framemd5", ""},
		{"machine-b-444.mkv", "hashes-b-444.framemd5", ""},
		{"machine-b-shift.mkv", "hashes-b-shift.framemd5", ""},
	}
	for _, tc := range cases {
		t.Run(tc.media+"/"+tc.manifest, func(t *testing.T) {
			tmp := filepath.Join(t.TempDir(), "oracle.framemd5")
			args := []string{"-v", "error", "-i", testutil.DataPath(tc.media)}
			if tc.pixFmt != "" {
				args = append(args, "-pix_fmt", tc.pixFmt)
			}
			args = append(args, "-f", "framemd5", "-y", tmp)
			cmd := exec.Command(ffmpeg, args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("ffmpeg: %v\n%s", err, out)
			}
			got, err := framemd5.ParseFile(tmp)
			if err != nil {
				t.Fatal(err)
			}
			want, err := framemd5.ParseFile(testutil.DataPath(tc.manifest))
			if err != nil {
				t.Fatal(err)
			}
			gh, wh := got.Hashes(0), want.Hashes(0)
			if len(gh) != len(wh) {
				t.Fatalf("frame count oracle=%d manifest=%d", len(gh), len(wh))
			}
			for i := range gh {
				if gh[i] != wh[i] {
					t.Fatalf("frame %d hash oracle=%s manifest=%s", i, gh[i], wh[i])
				}
			}
		})
	}
}
