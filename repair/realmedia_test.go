package repair_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mp4index/fixture"
	"mp4index/movie"
	"mp4index/repair"
)

func ffmpegPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available; skipping real media validation")
	}
	return p
}

func runFFmpeg(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func assertDecodes(t *testing.T, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("repaired file does not decode: %v\n%s", err, out)
	}
}

func generateRealSources(t *testing.T) (fast, late string) {
	dir := t.TempDir()
	fast = filepath.Join(dir, "fast.mp4")
	late = filepath.Join(dir, "late.mp4")
	src := "testsrc=duration=2:size=320x240:rate=15"
	runFFmpeg(t, "-f", "lavfi", "-i", src, "-c:v", "libx264", "-g", "15",
		"-bf", "2", "-pix_fmt", "yuv420p", "-movflags", "+faststart", fast)
	runFFmpeg(t, "-f", "lavfi", "-i", src, "-c:v", "libx264", "-g", "15",
		"-bf", "2", "-pix_fmt", "yuv420p", late)
	return fast, late
}

// Healthy real file must repair to frame identical output, B frames included.
func TestRealHealthyFrameIdentical(t *testing.T) {
	ffmpegPath(t)
	_, late := generateRealSources(t)

	rep, dst := repairTo(t, late, repair.Options{})
	if rep.ScannedSamples != 30 {
		t.Fatalf("scanned %d samples, want 30", rep.ScannedSamples)
	}
	assertDecodes(t, dst)

	origMD5 := filepath.Join(t.TempDir(), "o.fmd5")
	repMD5 := filepath.Join(t.TempDir(), "r.fmd5")
	runFFmpeg(t, "-i", late, "-an", "-f", "framemd5", origMD5)
	runFFmpeg(t, "-i", dst, "-an", "-f", "framemd5", repMD5)
	if hashFile(t, origMD5) == "" {
		t.Fatal("empty framemd5")
	}
	// Compare only the per-frame hash columns.
	if frameHashColumns(t, origMD5) != frameHashColumns(t, repMD5) {
		t.Fatal("decoded frame hash sequence differs after repair")
	}
}

func frameHashColumns(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c",
		`grep -v '^#' "$1" | awk -F, '{print $NF}'`, "sh", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// Truncation group on a real faststart MP4.
func TestRealTruncatedGroup(t *testing.T) {
	ffmpegPath(t)
	fast, _ := generateRealSources(t)

	f, info, err := movie.OpenAndParse(fast)
	if err != nil {
		t.Fatal(err)
	}
	sec := info.Mdats[0]
	f.Close()

	cut := sec.PayloadOffset + sec.PayloadLength*7/10
	damaged := filepath.Join(t.TempDir(), "truncated.mp4")
	if err := fixture.TruncateCopy(fast, damaged, cut); err != nil {
		t.Fatal(err)
	}
	rep, dst := repairTo(t, damaged, repair.Options{})
	if !rep.Damage.Truncated || rep.ScannedSamples == 0 {
		t.Fatalf("expected truncation with samples, %+v", rep.Damage)
	}
	if len(rep.Intervals) != 1 {
		t.Fatalf("expected one playable interval, got %+v", rep.Intervals)
	}
	assertDecodes(t, dst)
}

// Moov-less truncation recovered via a same-recorder reference file.
func TestRealMissingMoovWithReference(t *testing.T) {
	ffmpegPath(t)
	_, late := generateRealSources(t)

	f, info, err := movie.OpenAndParse(late)
	if err != nil {
		t.Fatal(err)
	}
	sec := info.Mdats[0]
	f.Close()
	cut := sec.PayloadOffset + sec.PayloadLength/2
	damaged := filepath.Join(t.TempDir(), "moov-gone.mp4")
	if err := fixture.TruncateCopy(late, damaged, cut); err != nil {
		t.Fatal(err)
	}
	if _, err := repair.Analyze(damaged, repair.Options{}); err == nil {
		t.Fatal("expected analysis without reference to fail on missing stsd")
	}
	dst := filepath.Join(t.TempDir(), "ref-repaired.mp4")
	rep, err := repair.RepairFile(damaged, dst, repair.Options{Reference: late})
	if err != nil {
		t.Fatalf("repair with reference: %v", err)
	}
	if !rep.Damage.MissingMoov || !rep.Damage.UsedReference {
		t.Fatalf("report flags wrong: %+v", rep.Damage)
	}
	assertDecodes(t, dst)
}

// Wrong stco offsets group on real media.
func TestRealBadOffsetsGroup(t *testing.T) {
	ffmpegPath(t)
	_, late := generateRealSources(t)
	bad := filepath.Join(t.TempDir(), "badoff.mp4")
	if n, err := fixture.OffsetAllSTCO(late, bad, 8192); err != nil || n == 0 {
		t.Fatalf("offset corruption: n=%d err=%v", n, err)
	}
	rep, dst := repairTo(t, bad, repair.Options{})
	if rep.Damage.BadOffsets == 0 {
		t.Fatalf("expected bad offsets detected, %+v", rep.Damage)
	}
	assertDecodes(t, dst)
}

// Duplicate sample records group on real media, using a moov-last one sample
// per chunk intermediate so duplicated offsets stay pure duplicates.
func TestRealDuplicateGroup(t *testing.T) {
	ffmpegPath(t)
	_, late := generateRealSources(t)

	baseLast := filepath.Join(t.TempDir(), "one-per-chunk-moovlast.mp4")
	if _, err := repair.RepairFile(late, baseLast, repair.Options{MoovLast: true}); err != nil {
		t.Fatal(err)
	}
	dup := filepath.Join(t.TempDir(), "duplicate.mp4")
	if err := fixture.DuplicateIndexRecords(baseLast, dup, []int{3, 8, 14, 21}); err != nil {
		t.Fatal(err)
	}
	plan, err := repair.Analyze(dup, repair.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Report.Damage.OldIndexOffsetCount != 34 {
		t.Fatalf("duplicated stco must parse with 34 entries, got %d", plan.Report.Damage.OldIndexOffsetCount)
	}
	if plan.Report.Damage.IndexDuplicateRecords < 4 {
		t.Fatalf("expected >=4 duplicate records, %+v", plan.Report.Damage)
	}
	if plan.Report.Damage.BadOffsets != 0 {
		t.Fatalf("pure duplication must not create bad offsets, got %d", plan.Report.Damage.BadOffsets)
	}
	if plan.Report.Damage.Truncated || len(plan.Report.Unrecovered) != 0 {
		t.Fatalf("fully explained duplicates must not be truncation: trunc=%v unrec=%+v",
			plan.Report.Damage.Truncated, plan.Report.Unrecovered)
	}
	if plan.Report.ScannedSamples != 30 {
		t.Fatalf("scanned %d want 30", plan.Report.ScannedSamples)
	}
	_, dst := repairTo(t, dup, repair.Options{})
	assertDecodes(t, dst)
}

// Non-video tracks (audio) are preserved verbatim apart from chunk offset
// remapping, so "index only" repair does not silently drop them.
func TestRealAudioTrackPreserved(t *testing.T) {
	ffmpegPath(t)
	path := filepath.Join(t.TempDir(), "av.mp4")
	runFFmpeg(t,
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=15",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:v", "libx264", "-bf", "0", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-shortest", path)

	_, dst := repairTo(t, path, repair.Options{})
	assertDecodes(t, dst)

	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries",
		"stream=codec_type", "-of", "csv=p=0", dst).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "audio") || !strings.Contains(string(out), "video") {
		t.Fatalf("repaired file missing tracks: %q", out)
	}
}
