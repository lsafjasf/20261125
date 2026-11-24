package main

import (
	"fmt"
	"io"
)

type wavFormat struct {
	tag           uint16
	channels      int
	sampleRate    uint32
	blockAlign    int
	bitsPerSample int
}

func buildWAVManifest(m *Manifest, r io.ReaderAt, size int64, opts Options) error {
	var riff [12]byte
	if err := readFullAt(r, riff[:], 0); err != nil {
		return err
	}
	if string(riff[0:4]) != "RIFF" || string(riff[8:12]) != "WAVE" {
		return fmt.Errorf("not a WAVE file")
	}

	pos := int64(12)
	counts := map[string]int{}
	var fmtBox *BoxHash
	var dataBox *BoxHash
	var format wavFormat

	for pos+8 <= size {
		var header [8]byte
		if err := readFullAt(r, header[:], pos); err != nil {
			return fmt.Errorf("read RIFF chunk header at %d: %w", pos, err)
		}
		id := string(header[0:4])
		chunkSize := int64(leUint32(header[:], 4))
		payloadStart := pos + 8
		payloadEnd := payloadStart + chunkSize
		if payloadEnd > size {
			return fmt.Errorf("RIFF chunk %q at %d extends to %d beyond %d", id, pos, payloadEnd, size)
		}

		counts[id]++
		path := fmt.Sprintf("/%s[%d]", id, counts[id])
		sum, err := hashRange(r, pos, payloadEnd)
		if err != nil {
			return err
		}
		box := BoxHash{
			Path:      path,
			Type:      id,
			Index:     counts[id],
			Start:     pos,
			HeaderEnd: payloadStart,
			End:       payloadEnd,
			Size:      payloadEnd - pos,
			SHA256:    sum,
			Ignorable: id != "fmt " && id != "data",
		}
		m.Boxes = append(m.Boxes, box)

		switch id {
		case "fmt ":
			fmtBox = &box
			format, err = parseWAVFormat(r, box)
			if err != nil {
				m.Warnings = append(m.Warnings, err.Error())
			}
		case "data":
			if dataBox == nil {
				dataBox = &box
			} else {
				m.Warnings = append(m.Warnings, "multiple RIFF data chunks; only first is hashed")
			}
		}

		pos = payloadEnd + chunkSize%2
	}

	if fmtBox == nil {
		return fmt.Errorf("WAVE missing fmt chunk")
	}
	if dataBox == nil {
		return fmt.Errorf("WAVE missing data chunk")
	}
	if format.channels <= 0 || format.blockAlign <= 0 {
		return fmt.Errorf("invalid WAVE format")
	}

	frameBytes := int64(format.blockAlign)
	dataPayloadSize := dataBox.End - dataBox.HeaderEnd
	frameCount := dataPayloadSize / frameBytes
	if rem := dataPayloadSize % frameBytes; rem != 0 {
		m.Warnings = append(m.Warnings, fmt.Sprintf("data chunk has %d trailing byte(s) outside whole PCM frames", rem))
	}
	m.Tracks = append(m.Tracks, TrackInfo{
		TrackID:     0,
		Handler:     "WAVE",
		Kind:        "audio",
		Timescale:   format.sampleRate,
		SampleCount: int(frameCount),
		Audio: &AudioFormat{
			FormatTag:     format.tag,
			Channels:      format.channels,
			SampleRate:    format.sampleRate,
			BitsPerSample: format.bitsPerSample,
			BlockAlign:    format.blockAlign,
		},
	})
	m.AudioIntervals = buildWAVIntervals(r, *dataBox, format.sampleRate, frameBytes, frameCount, opts)
	return nil
}

func parseWAVFormat(r io.ReaderAt, box BoxHash) (wavFormat, error) {
	n := box.End - box.HeaderEnd
	if n < 16 {
		return wavFormat{}, fmt.Errorf("fmt chunk too short: %d", n)
	}
	data := make([]byte, n)
	if _, err := r.ReadAt(data, box.HeaderEnd); err != nil && err != io.EOF {
		return wavFormat{}, err
	}

	f := wavFormat{
		tag:           leUint16(data, 0),
		channels:      int(leUint16(data, 2)),
		sampleRate:    leUint32(data, 4),
		blockAlign:    int(leUint16(data, 12)),
		bitsPerSample: int(leUint16(data, 14)),
	}
	if f.tag == 65534 && n >= 26 && leUint16(data, 24) == 1 {
		f.tag = 1
	}
	if f.tag != 1 {
		return f, fmt.Errorf("unsupported compressed WAVE format tag %d; hashing data ranges only", f.tag)
	}
	expected := f.channels * (f.bitsPerSample / 8)
	if f.bitsPerSample%8 != 0 || f.blockAlign != expected {
		if f.blockAlign <= 0 {
			f.blockAlign = expected
		}
		return f, fmt.Errorf("unsafe PCM frame layout: block_align=%d expected=%d", f.blockAlign, expected)
	}
	return f, nil
}

func buildWAVIntervals(r io.ReaderAt, data BoxHash, sampleRate uint32, frameBytes int64, frameCount int64, opts Options) []AudioIntervalHash {
	window := int64(opts.AudioIntervalSamples)
	if window <= 0 {
		window = int64(DefaultOptions().AudioIntervalSamples)
	}
	count := int((frameCount + window - 1) / window)
	out := make([]AudioIntervalHash, 0, count)

	for start := int64(0); start < frameCount; start += window {
		end := start + window
		if end > frameCount {
			end = frameCount
		}
		byteStart := data.HeaderEnd + start*frameBytes
		byteEnd := data.HeaderEnd + end*frameBytes
		sum, err := hashRange(r, byteStart, byteEnd)
		if err != nil {
			continue
		}
		interval := AudioIntervalHash{
			Key:         fmt.Sprintf("wav/audio-interval/%d", len(out)+1),
			Source:      FormatWAV,
			Index:       len(out) + 1,
			StartSample: start,
			EndSample:   end,
			StartTicks:  start,
			EndTicks:    end,
			Offset:      byteStart,
			Size:        byteEnd - byteStart,
			SHA256:      sum,
		}
		if sampleRate > 0 {
			interval.StartSeconds = float64(start) / float64(sampleRate)
			interval.EndSeconds = float64(end) / float64(sampleRate)
		}
		out = append(out, interval)
	}
	return out
}
