package analyze

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"tearcheck/internal/detect"
	"tearcheck/internal/frameio"
	"tearcheck/internal/review"
)

const (
	testWidth   = 160
	testHeight  = 96
	testTearRow = 32
	testShift   = 10
)

func TestAnalyzeSeparatesTearFromHorizontalLight(t *testing.T) {
	dir := t.TempDir()
	writeSyntheticSequence(t, dir)
	plan, err := frameio.LoadPlan(dir, "", 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	settings := detect.DefaultSettings()
	settings.MaxShiftPX = 16
	settings.BandRows = 6
	settings.MinContinuousFrames = 3

	first, err := Analyze(plan, Options{Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Analyze(plan, Options{Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.MarshalIndent(first, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.MarshalIndent(second, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("same input produced different reports")
	}

	if got := first.Frames[2].Machine.Classification; got != "light_effect" {
		t.Fatalf("light frame classification = %q, want light_effect", got)
	}
	if got := first.Frames[2].Final; got != "not_tear" {
		t.Fatalf("light frame final = %q, want not_tear", got)
	}
	for _, index := range []int{4, 5, 6} {
		frame := first.Frames[index]
		if frame.Machine.Classification != "tear" {
			t.Fatalf("frame %d classification = %q, want tear; seams=%+v bands=%+v", index, frame.Machine.Classification, frame.Machine.Seams, frame.Machine.Bands)
		}
		if frame.Machine.WidthPX != testShift {
			t.Fatalf("frame %d width = %d, want %d", index, frame.Machine.WidthPX, testShift)
		}
		if frame.Machine.Continuity != "continuous" {
			t.Fatalf("frame %d continuity = %q, want continuous", index, frame.Machine.Continuity)
		}
	}
	if !first.Frames[7].Duplicate {
		t.Fatal("frame 7 should be marked as a duplicate")
	}
	if first.Frames[7].DuplicateOf != "f06" {
		t.Fatalf("duplicate_of = %q, want f06", first.Frames[7].DuplicateOf)
	}
	if len(first.Events) != 1 {
		t.Fatalf("event count = %d, want 1", len(first.Events))
	}
	event := first.Events[0]
	if len(event.FrameIDs) != 3 {
		t.Fatalf("event frame count = %d, want 3 (duplicates must not inflate continuity)", len(event.FrameIDs))
	}
	if event.WidthPX != testShift || event.Continuity != "continuous" {
		t.Fatalf("event width/continuity = %d/%q, want %d/continuous", event.WidthPX, event.Continuity, testShift)
	}
	if event.StartPTSUS != 160_000 || event.EndPTSUS != 240_000 {
		t.Fatalf("event timestamps = %d..%d, want 160000..240000", event.StartPTSUS, event.EndPTSUS)
	}
	if first.Frames[4].PTSMS != 160 || first.Frames[4].Timecode != "00:00:00.160" {
		t.Fatalf("timestamp evidence = %d/%q", first.Frames[4].PTSMS, first.Frames[4].Timecode)
	}
	if first.Summary.MachineLightEffectFrames != 1 || first.Summary.FinalTearFrames != 3 {
		t.Fatalf("summary = %+v", first.Summary)
	}
}

func TestReviewClearsFramesIndividually(t *testing.T) {
	dir := t.TempDir()
	writeSyntheticSequence(t, dir)
	plan, err := frameio.LoadPlan(dir, "", 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	settings := detect.DefaultSettings()
	settings.MaxShiftPX = 16

	reviewFile := review.NewFile()
	reviewFile, err = review.Set(reviewFile, review.Entry{
		FrameID:  "f04",
		Decision: review.DecisionClear,
		Note:     "intentional horizontal wipe",
		Reviewer: "alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(plan, Options{Settings: settings, ReviewFile: reviewFile})
	if err != nil {
		t.Fatal(err)
	}
	if report.Frames[4].Final != "not_tear" {
		t.Fatalf("reviewed frame final = %q, want not_tear", report.Frames[4].Final)
	}
	if report.Frames[4].Review == nil || report.Frames[4].Review.Note != "intentional horizontal wipe" {
		t.Fatalf("review evidence missing: %+v", report.Frames[4].Review)
	}
	if report.Frames[5].Final != "tear" || report.Frames[6].Final != "tear" {
		t.Fatal("unreviewed frames must stay torn")
	}
	if report.Events[0].FinalClassification != "tear" {
		t.Fatalf("event final = %q, want tear", report.Events[0].FinalClassification)
	}

	for _, frameID := range []string{"f04", "f05", "f06"} {
		reviewFile, err = review.Set(reviewFile, review.Entry{
			FrameID:  frameID,
			Decision: review.DecisionClear,
			Reviewer: "alice",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	cleared, err := Analyze(plan, Options{Settings: settings, ReviewFile: reviewFile})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Events[0].FinalClassification != "not_tear" {
		t.Fatalf("cleared event final = %q, want not_tear", cleared.Events[0].FinalClassification)
	}
	if cleared.Summary.FinalTearFrames != 0 || cleared.Summary.ReviewedFrames != 3 {
		t.Fatalf("cleared summary = %+v", cleared.Summary)
	}
}

func TestReviewUnknownFrameFails(t *testing.T) {
	dir := t.TempDir()
	writeSyntheticSequence(t, dir)
	plan, err := frameio.LoadPlan(dir, "", 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	file := review.NewFile()
	file, err = review.Set(file, review.Entry{FrameID: "missing", Decision: review.DecisionClear})
	if err != nil {
		t.Fatal(err)
	}
	settings := detect.DefaultSettings()
	settings.MaxShiftPX = 16
	_, err = Analyze(plan, Options{Settings: settings, ReviewFile: file})
	if err == nil {
		t.Fatal("expected unknown frame id to fail")
	}
}

func TestIsolatedBandIsStrongEvidence(t *testing.T) {
	dir := t.TempDir()
	img := syntheticBandFrame(30, 62, testShift)
	writeGrayPNG(t, filepath.Join(dir, "band.png"), img)
	writeManifest(t, dir, []frameio.ManifestFrame{{
		ID: "band", File: "band.png", PTSUS: 0, DurationUS: 40_000,
	}})
	plan, err := frameio.LoadPlan(dir, "", 25, 1)
	if err != nil {
		t.Fatal(err)
	}
	settings := detect.DefaultSettings()
	settings.MaxShiftPX = 16
	report, err := Analyze(plan, Options{Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	frame := report.Frames[0]
	if frame.Machine.Classification != "tear" {
		t.Fatalf("band classification = %q, want tear; seams=%+v bands=%+v", frame.Machine.Classification, frame.Machine.Seams, frame.Machine.Bands)
	}
	if frame.Machine.Continuity != "isolated" {
		t.Fatalf("band continuity = %q, want isolated", frame.Machine.Continuity)
	}
	if len(frame.Machine.Bands) != 1 {
		t.Fatalf("bands = %+v, want one", frame.Machine.Bands)
	}
	band := frame.Machine.Bands[0]
	if band.HeightPX != 32 || band.TopOffsetPX != testShift || band.BottomOffsetPX != -testShift {
		t.Fatalf("band evidence = %+v", band)
	}
	if report.Events[0].FinalClassification != "tear" {
		t.Fatalf("band event final = %q", report.Events[0].FinalClassification)
	}
}

func TestSingleSeamIsolatedIsUncertain(t *testing.T) {
	dir := t.TempDir()
	img := syntheticFrame(false, true, 0)
	writeGrayPNG(t, filepath.Join(dir, "single.png"), img)
	writeManifest(t, dir, []frameio.ManifestFrame{{
		ID: "single", File: "single.png", PTSUS: 0, DurationUS: 40_000,
	}})
	plan, err := frameio.LoadPlan(dir, "", 25, 1)
	if err != nil {
		t.Fatal(err)
	}
	settings := detect.DefaultSettings()
	settings.MaxShiftPX = 16
	report, err := Analyze(plan, Options{Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	if report.Frames[0].Machine.Classification != "uncertain" {
		t.Fatalf("classification = %q, want uncertain", report.Frames[0].Machine.Classification)
	}
	if report.Events[0].FinalClassification != "uncertain" {
		t.Fatalf("event final = %q, want uncertain", report.Events[0].FinalClassification)
	}
}

func TestDefaultSettingsFitSmallFrames(t *testing.T) {
	dir := t.TempDir()
	img := image.NewGray(image.Rect(0, 0, 64, 48))
	const shift = 5
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			sourceX := x
			if y >= 15 && y < 33 {
				sourceX = (x - shift + 64) % 64
			}
			img.SetGray(x, y, color.Gray{Y: texture(sourceX)})
		}
	}
	writeGrayPNG(t, filepath.Join(dir, "small.png"), img)
	writeManifest(t, dir, []frameio.ManifestFrame{{
		ID: "small", File: "small.png", PTSUS: 0,
	}})
	plan, err := frameio.LoadPlan(dir, "", 25, 1)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(plan, Options{Settings: detect.DefaultSettings()})
	if err != nil {
		t.Fatal(err)
	}
	if report.Settings.MaxShiftPX != 24 {
		t.Fatalf("effective max shift = %d, want 24", report.Settings.MaxShiftPX)
	}
	if len(report.Frames[0].Machine.Seams) == 0 {
		t.Fatal("small frame did not produce seams")
	}
}

func TestIntermittentSeamAcrossOneCleanFrame(t *testing.T) {
	dir := t.TempDir()
	for index := 0; index < 5; index++ {
		torn := index == 1 || index == 3
		img := syntheticFrame(false, torn, index)
		writeGrayPNG(t, filepath.Join(dir, fmt.Sprintf("i%02d.png", index)), img)
	}
	frames := make([]frameio.ManifestFrame, 0, 5)
	for index := 0; index < 5; index++ {
		frames = append(frames, frameio.ManifestFrame{
			ID: fmt.Sprintf("i%02d", index), File: fmt.Sprintf("i%02d.png", index),
			PTSUS: int64(index) * 40_000,
		})
	}
	writeManifest(t, dir, frames)
	plan, err := frameio.LoadPlan(dir, "", 25, 1)
	if err != nil {
		t.Fatal(err)
	}
	settings := detect.DefaultSettings()
	settings.MaxShiftPX = 16
	report, err := Analyze(plan, Options{Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Events) != 1 || report.Events[0].Continuity != "intermittent" {
		t.Fatalf("events = %+v, want one intermittent event", report.Events)
	}
	if report.Frames[1].Final != "tear" || report.Frames[3].Final != "tear" {
		t.Fatal("intermittent tear frames should be final tear")
	}
	if report.Frames[2].Final != "not_tear" {
		t.Fatalf("clean gap final = %q, want not_tear", report.Frames[2].Final)
	}
}

func writeSyntheticSequence(t *testing.T, dir string) {
	t.Helper()
	for index := 0; index < 8; index++ {
		var img *image.Gray
		switch index {
		case 2:
			img = syntheticFrame(true, false, index)
		case 4, 5, 6:
			img = syntheticFrame(false, true, index)
		case 7:
			img = syntheticFrame(false, true, 6)
		default:
			img = syntheticFrame(false, false, index)
		}
		path := filepath.Join(dir, fmt.Sprintf("f%02d.png", index))
		writeGrayPNG(t, path, img)
	}
	frames := make([]frameio.ManifestFrame, 0, 8)
	for index := 0; index < 8; index++ {
		frames = append(frames, frameio.ManifestFrame{
			ID:         fmt.Sprintf("f%02d", index),
			File:       fmt.Sprintf("f%02d.png", index),
			PTSUS:      int64(index) * 40_000,
			DurationUS: 40_000,
		})
	}
	writeManifest(t, dir, frames)
}

func writeManifest(t *testing.T, dir string, frames []frameio.ManifestFrame) {
	t.Helper()
	raw, err := json.MarshalIndent(frameio.Manifest{
		Schema: frameio.ManifestSchema,
		FPSNum: 25,
		FPSDen: 1,
		Frames: frames,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "frames.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func syntheticFrame(lightEffect, torn bool, variant int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, testWidth, testHeight))
	for y := 0; y < testHeight; y++ {
		step := 0
		if lightEffect && y >= 20 && y < 29 {
			step = 80
		}
		for x := 0; x < testWidth; x++ {
			sourceX := x
			if torn && y >= testTearRow {
				sourceX = (x - testShift + testWidth) % testWidth
			}
			value := int(texture(sourceX+variant*17)) + step
			if value > 255 {
				value = 255
			}
			img.SetGray(x, y, color.Gray{Y: uint8(value)})
		}
	}
	return img
}

func syntheticBandFrame(top, bottom, shift int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, testWidth, testHeight))
	for y := 0; y < testHeight; y++ {
		for x := 0; x < testWidth; x++ {
			sourceX := x
			if y >= top && y < bottom {
				sourceX = (x - shift + testWidth) % testWidth
			}
			img.SetGray(x, y, color.Gray{Y: texture(sourceX)})
		}
	}
	return img
}

func texture(x int) uint8 {
	value := (x*37 + (x/5)*71 + (x*x)%251) % 128
	return uint8(value)
}

func writeGrayPNG(t *testing.T, path string, img *image.Gray) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
