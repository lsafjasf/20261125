package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mp4diag"
)

const (
	containerFile  = "01-container-first-bad-box.mp4"
	metadataFile   = "02-sample-table-metadata-only.mp4"
	compressedFile = "03-compressed-sample2-display-2-4.mp4"
)

func main() {
	cleanPath := flag.String("clean", "testdata/clean.mp4", "known-good minimal MP4 baseline")
	outDir := flag.String("out", "injections", "directory for three injection files and manifest")
	flag.Parse()
	if err := run(*cleanPath, *outDir); err != nil {
		fmt.Fprintln(os.Stderr, "mkinject:", err)
		os.Exit(1)
	}
}

func run(cleanPath, outDir string) error {
	clean, err := os.ReadFile(cleanPath)
	if err != nil {
		return err
	}
	report, err := mp4diag.Analyze(cleanPath)
	if err != nil {
		return fmt.Errorf("analyze clean baseline: %w", err)
	}
	sample2 := findSample(report, 2)
	sample5 := findSample(report, 5)
	stsz := findBox(report, "stsz")
	mdat := findBox(report, "mdat")
	if sample2 == nil || sample5 == nil || stsz == nil || mdat == nil || len(report.Samples) < 8 {
		return fmt.Errorf("clean baseline must contain 8 parsed samples, stsz and mdat")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	containerEnd := sample5.Offset + int64(sample5.Size)
	if err := os.WriteFile(filepath.Join(outDir, containerFile), clean[:containerEnd], 0o644); err != nil {
		return err
	}

	metadata := append([]byte(nil), clean...)
	entry2SizeOffset := stsz.Offset + stsz.HeaderSize + 12 + 4
	if got := binary.BigEndian.Uint32(metadata[entry2SizeOffset : entry2SizeOffset+4]); got != sample2.Size {
		return fmt.Errorf("stsz entry 2 size=%d, sample size=%d", got, sample2.Size)
	}
	binary.BigEndian.PutUint32(metadata[entry2SizeOffset:entry2SizeOffset+4], sample2.Size+1)
	if err := os.WriteFile(filepath.Join(outDir, metadataFile), metadata, 0o644); err != nil {
		return err
	}

	compressed := append([]byte(nil), clean...)
	pixelByteOffset := sample2.Offset + 8
	compressed[pixelByteOffset] ^= 0x01
	if err := os.WriteFile(filepath.Join(outDir, compressedFile), compressed, 0o644); err != nil {
		return err
	}

	manifest := buildManifest(report, stsz, mdat)
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), data, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote 3 injections and manifest to %s\n", outDir)
	return nil
}

func buildManifest(report *mp4diag.Report, stsz, mdat *mp4diag.BoxInfo) mp4diag.Manifest {
	return mp4diag.Manifest{
		SchemaVersion: mp4diag.SchemaVersion,
		Reference:     "../testdata/clean.mp4",
		Fixtures: []mp4diag.FixtureExpectation{
			{
				File: containerFile, Title: "mdat 后三个 sample 被物理截断，moov 目录仍完整",
				DominantLayer:           mp4diag.LayerContainer,
				FirstBadBox:             &mp4diag.BoxLocator{Path: mdat.Path, Type: mdat.Type, Offset: mdat.Offset},
				AffectedSamples:         []int{6, 7, 8},
				AffectedDisplayFrames:   []int{6, 7, 8},
				CleanSampleRanges:       []mp4diag.Span{{First: 1, Last: 5}},
				CleanDisplayFrameRanges: []mp4diag.Span{{First: 1, Last: 5}},
				ReferencePixelDamage:    mp4diag.PixelNotAssessed,
				DecoderVerification:     "ffmpeg 可解码前 5 帧；sample 6-8 因 EOF 无 packet",
			},
			{
				File: metadataFile, Title: "stsz 第 2 项 +1；样本负载字节未变",
				DominantLayer: mp4diag.LayerSampleTable,
				MetadataBox: &mp4diag.BoxLocator{
					Path: stsz.Path, Type: stsz.Type, Offset: stsz.Offset, Field: "sample_size", Entry: 2,
				},
				FaultSample: 2, ChangedSamples: []int{},
				CleanSampleRanges:       []mp4diag.Span{{First: 1, Last: 8}},
				CleanDisplayFrameRanges: []mp4diag.Span{{First: 1, Last: 8}},
				ReferencePixelDamage:    mp4diag.PixelRuledOut,
				DecoderVerification:     "ffmpeg 的 NAL/demux 报错来自错误 stsz；不是像素哈希差异",
			},
			{
				File: compressedFile, Title: "sample 2 切片负载单字节翻转",
				DominantLayer: mp4diag.LayerCompressed,
				FaultSample:   2, ChangedSamples: []int{2},
				AffectedSamples:         []int{2, 3, 4},
				AffectedDisplayFrames:   []int{2, 3, 4},
				CleanSampleRanges:       []mp4diag.Span{{First: 1, Last: 1}, {First: 5, Last: 8}},
				CleanDisplayFrameRanges: []mp4diag.Span{{First: 1, Last: 1}, {First: 5, Last: 8}},
				ReferencePixelDamage:    mp4diag.PixelSuspected,
				DecoderVerification:     "ffmpeg framemd5 实测显示帧 2,3,4 与干净基线不同，帧 5 起 IDR 恢复",
			},
		},
	}
}

func findSample(report *mp4diag.Report, number int) *mp4diag.SampleInfo {
	for i := range report.Samples {
		if report.Samples[i].Number == number {
			return &report.Samples[i]
		}
	}
	return nil
}

func findBox(report *mp4diag.Report, typ string) *mp4diag.BoxInfo {
	for i := range report.Boxes {
		if report.Boxes[i].Type == typ {
			return &report.Boxes[i]
		}
	}
	return nil
}
