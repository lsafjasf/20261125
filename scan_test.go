package matroska

import (
	"bytes"
	"testing"
)

func encodeVINT(value uint64) []byte {
	for width := 1; width <= 8; width++ {
		limit := (uint64(1) << uint(7*width)) - 2
		if value > limit {
			continue
		}
		out := make([]byte, width)
		v := value
		for i := width - 1; i >= 0; i-- {
			out[i] = byte(v)
			v >>= 8
		}
		out[0] |= byte(1) << uint(8-width)
		return out
	}
	panic("value does not fit VINT")
}

func encodeID(id uint64) []byte {
	switch {
	case id <= 0xFF:
		return []byte{byte(id)}
	case id <= 0xFFFF:
		return []byte{byte(id >> 8), byte(id)}
	case id <= 0xFFFFFF:
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	default:
		return []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
	}
}

func element(id uint64, payload ...byte) []byte {
	out := encodeID(id)
	out = append(out, encodeVINT(uint64(len(payload)))...)
	return append(out, payload...)
}

func uintBytes(value uint64) []byte {
	if value == 0 {
		return []byte{0}
	}
	var out []byte
	for value > 0 {
		out = append([]byte{byte(value)}, out...)
		value >>= 8
	}
	return out
}

func trackElements(defaultDuration uint64) []byte {
	track := element(idTrackEntry,
		append(element(idTrackNumber, 1), element(idDefaultDuration, uintBytes(defaultDuration)...)...)...)
	return element(idTracks, track...)
}

func infoElement(scale uint64) []byte {
	return element(idInfo, element(idTimecodeScale, uintBytes(scale)...)...)
}

func clusterElement(timecode uint64, children ...[]byte) []byte {
	payload := element(idClusterTimecode, uintBytes(timecode)...)
	for _, child := range children {
		payload = append(payload, child...)
	}
	return element(idCluster, payload...)
}

func matroskaFile(segmentChildren ...[]byte) []byte {
	segment := element(idSegment, bytes.Join(segmentChildren, nil)...)
	return append(element(idEBML, 0x42, 0x86, 0x81, 0x01), segment...)
}

func TestScanFixedLacingWithTimestamps(t *testing.T) {
	block := element(idSimpleBlock, blockPayload(0x84, []byte{2}, []byte("AAAABBBBCCCC"), 0)...)
	data := matroskaFile(
		infoElement(1_000_000),
		trackElements(20_000_000),
		clusterElement(1000, block),
	)
	result := Scan(data, ScanOptions{})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", result.Issues)
	}
	if len(result.Blocks) != 1 || len(result.Frames) != 3 {
		t.Fatalf("blocks=%d frames=%d", len(result.Blocks), len(result.Frames))
	}
	wantData := []string{"AAAA", "BBBB", "CCCC"}
	for i, frame := range result.Frames {
		if int64(len(data)) < frame.End {
			t.Fatalf("frame %d end %d outside file", i, frame.End)
		}
		if got := string(data[frame.Start:frame.End]); got != wantData[i] {
			t.Errorf("frame %d data=%q want %q", i, got, wantData[i])
		}
		if want := int64(1000 + i*20); frame.Timecode != want {
			t.Errorf("frame %d timecode=%d want %d", i, frame.Timecode, want)
		}
	}
}

func TestScanUnknownElementsAndResync(t *testing.T) {
	block := element(idSimpleBlock, blockPayload(0x84, []byte{0}, []byte("voice"), 2)...)
	knownUnknown := element(0x81, 'x', 'y')
	unknownSized := []byte{0x82, 0xFF}
	data := matroskaFile(
		infoElement(1_000_000),
		trackElements(20_000_000),
		knownUnknown,
		unknownSized,
		clusterElement(10, block),
	)
	result := Scan(data, ScanOptions{ReportUnknown: true})
	if len(result.Blocks) != 1 || len(result.Frames) != 1 {
		t.Fatalf("blocks=%d frames=%d issues=%+v", len(result.Blocks), len(result.Frames), result.Issues)
	}
	var unknown, recovered int
	for _, issue := range result.Issues {
		if issue.Kind != ErrUnknownElement {
			t.Errorf("unexpected issue: %+v", issue)
			continue
		}
		unknown++
		if issue.Recovered {
			recovered++
		}
	}
	if unknown != 2 || recovered != 2 {
		t.Fatalf("unknown=%d recovered=%d issues=%+v", unknown, recovered, result.Issues)
	}
}

func TestScanSkipsBadBlockAndFindsNext(t *testing.T) {
	bad := element(idSimpleBlock, blockPayload(0x06, []byte{1, 0x8A}, nil, 0)...)
	good := element(idSimpleBlock, blockPayload(0x84, []byte{2}, []byte("AAAABBBBCCCC"), 5)...)
	data := matroskaFile(
		infoElement(1_000_000),
		trackElements(20_000_000),
		clusterElement(100, bad, good),
	)
	result := Scan(data, ScanOptions{})
	if len(result.Blocks) != 1 || len(result.Frames) != 3 {
		t.Fatalf("blocks=%d frames=%d issues=%+v", len(result.Blocks), len(result.Frames), result.Issues)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues=%+v", result.Issues)
	}
	issue := result.Issues[0]
	if issue.Kind != ErrFirstFrameSize || !issue.Recovered {
		t.Fatalf("issue=%+v", issue)
	}
	if got := result.Frames[0].Timecode; got != 105 {
		t.Fatalf("recovered frame timecode=%d want 105", got)
	}
}

func TestScanBadBlockBoundaryDoesNotSwallowNext(t *testing.T) {
	good := element(idSimpleBlock, blockPayload(0x84, []byte{0}, []byte("next-audio"), 1)...)
	badPayload := blockPayload(0x06, []byte{1, 0x00}, nil, 0)
	badPayload = append(badPayload, good...)
	bad := element(idSimpleBlock, badPayload...)
	data := matroskaFile(
		infoElement(1_000_000),
		trackElements(20_000_000),
		clusterElement(10, bad),
	)
	result := Scan(data, ScanOptions{})
	if len(result.Blocks) != 1 || len(result.Frames) != 1 {
		t.Fatalf("blocks=%d frames=%d issues=%+v", len(result.Blocks), len(result.Frames), result.Issues)
	}
	if got := string(data[result.Frames[0].Start:result.Frames[0].End]); got != "next-audio" {
		t.Fatalf("recovered frame data=%q", got)
	}
	if len(result.Issues) == 0 || result.Issues[0].Kind != ErrFirstFrameSize || !result.Issues[0].Recovered {
		t.Fatalf("issues=%+v", result.Issues)
	}
}

func TestScanBlockGroup(t *testing.T) {
	block := element(idBlock, blockPayload(0x84, []byte{2}, []byte("AAAABBBBCCCC"), 5)...)
	group := element(idBlockGroup, block...)
	data := matroskaFile(
		infoElement(1_000_000),
		trackElements(10_000_000),
		clusterElement(100, group),
	)
	result := Scan(data, ScanOptions{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues=%+v", result.Issues)
	}
	if len(result.Blocks) != 1 || len(result.Frames) != 3 {
		t.Fatalf("blocks=%d frames=%d", len(result.Blocks), len(result.Frames))
	}
	for i, frame := range result.Frames {
		if want := int64(105 + i*10); frame.Timecode != want {
			t.Errorf("frame %d timecode=%d want %d", i, frame.Timecode, want)
		}
	}
}

func TestScanBlockGroupResyncsBadBlock(t *testing.T) {
	bad := element(idBlock, blockPayload(0x06, []byte{1, 0x00}, nil, 0)...)
	good := element(idBlock, blockPayload(0x84, []byte{0}, []byte("group-audio"), 2)...)
	group := element(idBlockGroup, append(bad, good...)...)
	data := matroskaFile(
		infoElement(1_000_000),
		trackElements(10_000_000),
		clusterElement(100, group),
	)
	result := Scan(data, ScanOptions{})
	if len(result.Blocks) != 1 || len(result.Frames) != 1 {
		t.Fatalf("blocks=%d frames=%d issues=%+v", len(result.Blocks), len(result.Frames), result.Issues)
	}
	if got := string(data[result.Frames[0].Start:result.Frames[0].End]); got != "group-audio" {
		t.Fatalf("recovered frame data=%q", got)
	}
	if len(result.Issues) == 0 || !result.Issues[0].Recovered {
		t.Fatalf("issues=%+v", result.Issues)
	}
}

func TestScanEOFTruncationIsTerminal(t *testing.T) {
	good := element(idSimpleBlock, blockPayload(0x84, []byte{0}, []byte("voice"), 0)...)
	fullBad := element(idSimpleBlock, blockPayload(0x84, []byte{0}, []byte("truncated-frame"), 1)...)
	truncatedBad := fullBad[:len(fullBad)-4]
	data := matroskaFile(
		infoElement(1_000_000),
		trackElements(20_000_000),
		clusterElement(50, good, truncatedBad),
	)
	result := Scan(data, ScanOptions{})
	if len(result.Blocks) != 1 || len(result.Frames) != 1 {
		t.Fatalf("blocks=%d frames=%d issues=%+v", len(result.Blocks), len(result.Frames), result.Issues)
	}
	if len(result.Issues) == 0 {
		t.Fatal("missing truncation issue")
	}
	last := result.Issues[len(result.Issues)-1]
	if last.Kind != ErrTruncated || last.Recovered {
		t.Fatalf("terminal issue=%+v", last)
	}
}
