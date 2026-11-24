package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func mp4Box(typ string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(8+len(payload)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}

func mp4FullBox(typ string, version byte, payload []byte) []byte {
	full := make([]byte, 4+len(payload))
	full[0] = version
	copy(full[4:], payload)
	return mp4Box(typ, full)
}

func buildTestMP4(t *testing.T, video, audio [][]byte, moovFirst bool) []byte {
	t.Helper()
	ftyp := mp4Box("ftyp", append([]byte("isom"), make([]byte, 4)...))
	videoBytes := bytes.Join(video, nil)
	audioBytes := bytes.Join(audio, nil)

	buildMoov := func(videoOffset, audioOffset int64) []byte {
		var moov bytes.Buffer
		moov.Write(buildTestVideoTrak(video, videoOffset))
		moov.Write(buildTestAudioTrak(audio, audioOffset))
		return mp4Box("moov", moov.Bytes())
	}

	mdatPayload := append(append([]byte(nil), videoBytes...), audioBytes...)
	mdat := mp4Box("mdat", mdatPayload)

	var out bytes.Buffer
	out.Write(ftyp)
	if moovFirst {
		placeholder := buildMoov(0, 0)
		videoOffset := int64(len(ftyp) + len(placeholder) + 8)
		audioOffset := videoOffset + int64(len(videoBytes))
		moov := buildMoov(videoOffset, audioOffset)
		if len(moov) != len(placeholder) {
			t.Fatal("moov placeholder size changed")
		}
		out.Write(moov)
		out.Write(mdat)
	} else {
		videoOffset := int64(len(ftyp) + 8)
		audioOffset := videoOffset + int64(len(videoBytes))
		out.Write(mdat)
		out.Write(buildMoov(videoOffset, audioOffset))
	}
	out.Write(mp4Box("free", make([]byte, 16)))
	return out.Bytes()
}

func buildTestVideoTrak(samples [][]byte, offset int64) []byte {
	var tkhdBody bytes.Buffer
	tkhdBody.Write(make([]byte, 8)) // creation/modification
	binary.Write(&tkhdBody, binary.BigEndian, uint32(1))
	tkhdBody.Write(make([]byte, 4+4))
	binary.Write(&tkhdBody, binary.BigEndian, uint32(1))
	tkhdBody.Write(make([]byte, 60))
	tkhd := mp4FullBox("tkhd", 0, tkhdBody.Bytes())

	mdhdBody := make([]byte, 20)
	binary.BigEndian.PutUint32(mdhdBody[8:12], 30000)
	binary.BigEndian.PutUint32(mdhdBody[12:16], 9000)
	mdhd := mp4FullBox("mdhd", 0, mdhdBody)
	hdlrBody := append(make([]byte, 4), []byte("vide")...)
	hdlrBody = append(hdlrBody, make([]byte, 16)...)
	hdlr := mp4FullBox("hdlr", 0, hdlrBody)

	var sttsBody bytes.Buffer
	binary.Write(&sttsBody, binary.BigEndian, uint32(1))
	binary.Write(&sttsBody, binary.BigEndian, uint32(len(samples)))
	binary.Write(&sttsBody, binary.BigEndian, uint32(3000))
	stts := mp4FullBox("stts", 0, sttsBody.Bytes())

	var stscBody bytes.Buffer
	binary.Write(&stscBody, binary.BigEndian, uint32(1))
	binary.Write(&stscBody, binary.BigEndian, uint32(1))
	binary.Write(&stscBody, binary.BigEndian, uint32(len(samples)))
	binary.Write(&stscBody, binary.BigEndian, uint32(1))
	stsc := mp4FullBox("stsc", 0, stscBody.Bytes())

	var stszBody bytes.Buffer
	binary.Write(&stszBody, binary.BigEndian, uint32(0))
	binary.Write(&stszBody, binary.BigEndian, uint32(len(samples)))
	for _, sample := range samples {
		binary.Write(&stszBody, binary.BigEndian, uint32(len(sample)))
	}
	stsz := mp4FullBox("stsz", 0, stszBody.Bytes())

	var stcoBody bytes.Buffer
	binary.Write(&stcoBody, binary.BigEndian, uint32(1))
	binary.Write(&stcoBody, binary.BigEndian, uint32(offset))
	stco := mp4FullBox("stco", 0, stcoBody.Bytes())

	var stssBody bytes.Buffer
	binary.Write(&stssBody, binary.BigEndian, uint32(2))
	binary.Write(&stssBody, binary.BigEndian, uint32(1))
	binary.Write(&stssBody, binary.BigEndian, uint32(3))
	stss := mp4FullBox("stss", 0, stssBody.Bytes())

	stbl := mp4Box("stbl", bytes.Join([][]byte{stts, stsc, stsz, stco, stss}, nil))
	minf := mp4Box("minf", stbl)
	mdia := mp4Box("mdia", bytes.Join([][]byte{mdhd, hdlr, minf}, nil))
	return mp4Box("trak", bytes.Join([][]byte{tkhd, mdia}, nil))
}

func buildTestAudioTrak(samples [][]byte, offset int64) []byte {
	var tkhdBody bytes.Buffer
	tkhdBody.Write(make([]byte, 8))
	binary.Write(&tkhdBody, binary.BigEndian, uint32(2))
	tkhdBody.Write(make([]byte, 68))
	tkhd := mp4FullBox("tkhd", 0, tkhdBody.Bytes())

	mdhdBody := make([]byte, 20)
	binary.BigEndian.PutUint32(mdhdBody[8:12], 48000)
	mdhd := mp4FullBox("mdhd", 0, mdhdBody)
	hdlrBody := append(make([]byte, 4), []byte("soun")...)
	hdlrBody = append(hdlrBody, make([]byte, 16)...)
	hdlr := mp4FullBox("hdlr", 0, hdlrBody)

	var sttsBody bytes.Buffer
	binary.Write(&sttsBody, binary.BigEndian, uint32(1))
	binary.Write(&sttsBody, binary.BigEndian, uint32(len(samples)))
	binary.Write(&sttsBody, binary.BigEndian, uint32(1024))
	stts := mp4FullBox("stts", 0, sttsBody.Bytes())

	var stscBody bytes.Buffer
	binary.Write(&stscBody, binary.BigEndian, uint32(1))
	binary.Write(&stscBody, binary.BigEndian, uint32(1))
	binary.Write(&stscBody, binary.BigEndian, uint32(len(samples)))
	binary.Write(&stscBody, binary.BigEndian, uint32(1))
	stsc := mp4FullBox("stsc", 0, stscBody.Bytes())

	var stszBody bytes.Buffer
	binary.Write(&stszBody, binary.BigEndian, uint32(0))
	binary.Write(&stszBody, binary.BigEndian, uint32(len(samples)))
	for _, sample := range samples {
		binary.Write(&stszBody, binary.BigEndian, uint32(len(sample)))
	}
	stsz := mp4FullBox("stsz", 0, stszBody.Bytes())

	var stcoBody bytes.Buffer
	binary.Write(&stcoBody, binary.BigEndian, uint32(1))
	binary.Write(&stcoBody, binary.BigEndian, uint32(offset))
	stco := mp4FullBox("stco", 0, stcoBody.Bytes())

	stbl := mp4Box("stbl", bytes.Join([][]byte{stts, stsc, stsz, stco}, nil))
	minf := mp4Box("minf", stbl)
	mdia := mp4Box("mdia", bytes.Join([][]byte{mdhd, hdlr, minf}, nil))
	return mp4Box("trak", bytes.Join([][]byte{tkhd, mdia}, nil))
}

func TestMP4RemuxAndVideoByteLocalization(t *testing.T) {
	video := [][]byte{[]byte("frame-one!"), []byte("FRAME-TWO-bytes"), []byte("third-picture")}
	audio := [][]byte{bytes.Repeat([]byte{0xA0}, 12), bytes.Repeat([]byte{0xB1}, 14)}

	originalBytes := buildTestMP4(t, video, audio, false)
	remuxBytes := buildTestMP4(t, video, audio, true)
	opts := Options{BlockSize: 128, AudioIntervalSamples: 1}

	original := testArchive(t, originalBytes, opts)
	remux := testArchive(t, remuxBytes, opts)
	if len(original.Manifest.Samples) != 5 {
		t.Fatalf("sample count = %d, want 5; warnings=%v", len(original.Manifest.Samples), original.Manifest.Warnings)
	}
	if len(original.Manifest.Chunks) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(original.Manifest.Chunks))
	}

	normal, err := CompareArchives(original, remux)
	if err != nil {
		t.Fatal(err)
	}
	if !normal.MediaIdentical {
		t.Fatalf("normal moov/mdat rearrangement caused media mismatch: changes=%+v warnings=%v", normal.Changes, normal.Warnings)
	}

	var target SampleHash
	for _, sample := range remux.Manifest.Samples {
		if sample.TrackKind == "video" && sample.Index == 2 {
			target = sample
		}
	}
	if target.Key == "" {
		t.Fatal("missing video sample 2 in remux manifest")
	}

	mutatedBytes := append([]byte(nil), remuxBytes...)
	mutatedBytes[target.Offset+3] ^= 0xFF
	mutated := testArchive(t, mutatedBytes, opts)

	report, err := CompareArchives(original, mutated)
	if err != nil {
		t.Fatal(err)
	}
	if report.MediaIdentical {
		t.Fatal("expected video sample mutation to be detected")
	}
	if report.EarliestMediaChange == nil {
		t.Fatal("missing earliest media change")
	}
	change := report.EarliestMediaChange
	if change.Layer != LayerSample {
		t.Fatalf("earliest layer = %q, want media-sample", change.Layer)
	}
	if change.RelativeOffset != 3 {
		t.Fatalf("relative offset = %d, want 3", change.RelativeOffset)
	}
	if change.CopyOffset != target.Offset+3 {
		t.Fatalf("copy offset = %d, want %d", change.CopyOffset, target.Offset+3)
	}
	if change.Media == nil || change.Media.SampleIndex != 2 || change.Media.TrackKind != "video" {
		t.Fatalf("bad affected picture: %+v", change.Media)
	}
	if change.Media.StartTime < 0.099 || change.Media.StartTime > 0.101 {
		t.Fatalf("start time = %v, want about 0.1s", change.Media.StartTime)
	}
}
