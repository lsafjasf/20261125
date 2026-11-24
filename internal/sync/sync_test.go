package sync

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func testOptions() Options {
	return Options{
		FPS:           25,
		Tolerance:     time.Millisecond,
		JumpThreshold: 20 * time.Millisecond,
		DriftBudget:   40 * time.Millisecond,
		MinSamples:    8,
		Context:       4,
	}
}

func TestLocalFrameSkipIsLocatedAtFirstJump(t *testing.T) {
	records := DemoRecords(DemoSpec{
		FPS:         25,
		TotalFrames: 1200,
		ChangeFrame: 500,
		Offset:      240 * time.Millisecond,
		Pattern:     KindFrameSkip,
	})

	report, err := Analyze(records, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.Classification != KindFrameSkip {
		t.Fatalf("classification = %q, want %q; summary=%s",
			report.Classification, KindFrameSkip, report.Summary)
	}
	if report.FirstAbnormalFrame == nil {
		t.Fatal("first abnormal frame is nil")
	}
	if report.FirstAbnormalFrame.Frame != 500 {
		t.Fatalf("first frame = %d, want 500", report.FirstAbnormalFrame.Frame)
	}
	if report.FirstAbnormalFrame.Stage != StageEdit {
		t.Fatalf("stage = %q, want edit", report.FirstAbnormalFrame.Stage)
	}
	if report.EstimatedSkippedFrames != 6 {
		t.Fatalf("skipped frames = %d, want 6", report.EstimatedSkippedFrames)
	}
	if report.FirstPlaybackAnomalyFrame == nil ||
		report.FirstPlaybackAnomalyFrame.Frame != 500 {
		t.Fatalf("playback anomaly = %#v, want frame 500",
			report.FirstPlaybackAnomalyFrame)
	}
	if got := report.ObservedFinalOffsetNS; got != 240_000_000 {
		t.Fatalf("final offset = %dns, want 240ms", got)
	}
}

func TestLinearDriftReportsEarlyThresholdFrame(t *testing.T) {
	records := DemoRecords(DemoSpec{
		FPS:         25,
		TotalFrames: 1000,
		ChangeFrame: 1,
		Offset:      240 * time.Millisecond,
		Pattern:     KindLinearDrift,
	})

	report, err := Analyze(records, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.Classification != KindLinearDrift {
		t.Fatalf("classification = %q, want %q", report.Classification, KindLinearDrift)
	}
	if report.FirstAbnormalFrame == nil {
		t.Fatal("first abnormal frame is nil")
	}
	if report.FirstAbnormalFrame.Frame > 20 {
		t.Fatalf("first drift threshold frame = %d, expected early crossing",
			report.FirstAbnormalFrame.Frame)
	}
	if report.ObservedFinalOffsetNS < 230_000_000 {
		t.Fatalf("final offset = %dns, want about 240ms",
			report.ObservedFinalOffsetNS)
	}
}

func TestDelayedLinearDriftFindsOnset(t *testing.T) {
	records := DemoRecords(DemoSpec{
		FPS:         25,
		TotalFrames: 2000,
		ChangeFrame: 500,
		Offset:      240 * time.Millisecond,
		Pattern:     KindLinearDrift,
	})

	report, err := Analyze(records, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.Classification != KindLinearDrift {
		t.Fatalf("classification = %q, want %q", report.Classification, KindLinearDrift)
	}
	if report.FirstAbnormalFrame == nil {
		t.Fatal("first abnormal frame is nil")
	}
	if report.FirstAbnormalFrame.Frame < 500 || report.FirstAbnormalFrame.Frame > 520 {
		t.Fatalf("first frame = %d, want threshold crossing near 500",
			report.FirstAbnormalFrame.Frame)
	}
	if report.EstimatedOnsetFrame == nil {
		t.Fatal("estimated onset is nil")
	}
	if delta := *report.EstimatedOnsetFrame - 500; delta < -2 || delta > 2 {
		t.Fatalf("onset = %d, want near 500", *report.EstimatedOnsetFrame)
	}
}

func TestFixedOffset(t *testing.T) {
	records := DemoRecords(DemoSpec{
		FPS:         25,
		TotalFrames: 120,
		Offset:      120 * time.Millisecond,
		Pattern:     KindFixedOffset,
	})

	report, err := Analyze(records, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.Classification != KindFixedOffset {
		t.Fatalf("classification = %q, want %q", report.Classification, KindFixedOffset)
	}
	if report.FirstAbnormalFrame == nil || report.FirstAbnormalFrame.Frame != 0 {
		t.Fatalf("first abnormal = %#v, want frame 0", report.FirstAbnormalFrame)
	}
}

func TestParseJSONLAndCSV(t *testing.T) {
	frame := int64(42)
	video := int64(1_680_000_000)
	audio := int64(1_920_000_000)
	jsonRecords, err := ParseJSON([]byte(`
{"stage":"playback","frame":42,"video_pos_ns":1680000000,"audio_pos_ns":1920000000}
{"stage":"render","frame":42,"position_ns":1680000000}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(jsonRecords) != 2 || jsonRecords[0].Frame == nil || *jsonRecords[0].Frame != frame {
		t.Fatalf("bad JSON records: %#v", jsonRecords)
	}

	csvInput := "stage,frame,video_pos_ms,audio_pos_ms\n" +
		"playback,42,1680,1920\n"
	csvRecords, err := ParseCSV(strings.NewReader(csvInput))
	if err != nil {
		t.Fatal(err)
	}
	points, err := Normalize(csvRecords)
	if err != nil {
		t.Fatal(err)
	}
	if points[0].VideoNS == nil || *points[0].VideoNS != video {
		t.Fatalf("video = %#v, want %d", points[0].VideoNS, video)
	}
	if points[0].AudioNS == nil || *points[0].AudioNS != audio {
		t.Fatalf("audio = %#v, want %d", points[0].AudioNS, audio)
	}
}

func TestWritePositionCSVContainsAllStages(t *testing.T) {
	records := DemoRecords(DemoSpec{TotalFrames: 4, ChangeFrame: 2})
	points, err := Normalize(records)
	if err != nil {
		t.Fatal(err)
	}
	tracks, err := BuildTracks(points)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := WritePositionCSV(&buffer, BuildPositionRows(tracks)); err != nil {
		t.Fatal(err)
	}
	output := buffer.String()
	for _, stage := range Stages {
		if !strings.Contains(output, string(stage)+"_position_ns") {
			t.Fatalf("CSV missing stage %q header:\n%s", stage, output)
		}
	}
}

func TestWidePositionCSVRoundTrip(t *testing.T) {
	records := DemoRecords(DemoSpec{TotalFrames: 30, ChangeFrame: 15})
	points, err := Normalize(records)
	if err != nil {
		t.Fatal(err)
	}
	tracks, err := BuildTracks(points)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := WritePositionCSV(&buffer, BuildPositionRows(tracks)); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseCSV(strings.NewReader(buffer.String()))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(parsed, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.Classification != KindFrameSkip {
		t.Fatalf("classification = %q, want %q", report.Classification, KindFrameSkip)
	}
	if report.FirstAbnormalFrame == nil || report.FirstAbnormalFrame.Frame != 15 {
		t.Fatalf("first abnormal = %#v, want frame 15", report.FirstAbnormalFrame)
	}
}

func TestFrameIndexGapDetectedWhenPTSIsContinuous(t *testing.T) {
	records := []Record{}
	for _, frame := range []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 16, 17, 18, 19} {
		frameValue := frame
		position := frame * 40_000_000
		records = append(records, Record{
			Stage:      StageRender,
			Frame:      &frameValue,
			PositionNS: &position,
		})
	}

	report, err := Analyze(records, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.FirstAbnormalFrame == nil {
		t.Fatal("first abnormal frame is nil")
	}
	if report.FirstAbnormalFrame.Frame != 16 {
		t.Fatalf("first abnormal = %d, want 16", report.FirstAbnormalFrame.Frame)
	}
	if report.Classification != KindFrameSkip {
		t.Fatalf("classification = %q, want %q", report.Classification, KindFrameSkip)
	}
	if report.EstimatedSkippedFrames != 6 {
		t.Fatalf("skipped = %d, want 6", report.EstimatedSkippedFrames)
	}
}
