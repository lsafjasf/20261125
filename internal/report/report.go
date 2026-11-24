package report

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"ffv1compare/internal/compare"
)

var verdictZH = map[string]string{
	"identical":                              "一致",
	"no_difference":                          "无差异",
	"decoding_difference":                    "解码差异",
	"mixed_encoding_and_decoding_difference": "编码+解码差异",
	"encoding_difference":                    "编码差异",
	"encoding_slice_order":                   "编码差异(slice 顺序)",
	"encoding_slice_raster":                  "编码差异(slice 重切)",
	"encoding_pixel_format":                  "编码差异(像素格式)",
	"encoding_crc_setting":                   "编码差异(CRC 设置)",
	"container_timestamp_difference":         "容器时间戳差异",
	"container_config_difference":            "容器配置差异",
	"bitstream_identical":                    "码流一致",
	"undetermined":                           "证据不足",
}

func ZH(v string) string {
	if s, ok := verdictZH[v]; ok {
		return s
	}
	return v
}

func WriteText(w io.Writer, res *compare.Result) {
	a, b := res.A, res.B
	fmt.Fprintln(w, "FFV1 逐帧对照")
	fmt.Fprintln(w, "================")
	fmt.Fprintf(w, "A: %s\n", a.Path)
	fmt.Fprintf(w, "   codec=%s size=%dx%d config=%s\n", a.CodecID, a.Width, a.Height, a.Config.Descriptor())
	fmt.Fprintf(w, "   frames=%d timestamp_scale=%dns container_crc=%d ok/%d bad\n",
		len(a.Frames), a.TimestampScale, a.ContainerCRCOK, a.ContainerCRCBad)
	if a.HashPath != "" {
		fmt.Fprintf(w, "   decoded hashes: %s\n", a.HashPath)
	}
	fmt.Fprintf(w, "B: %s\n", b.Path)
	fmt.Fprintf(w, "   codec=%s size=%dx%d config=%s\n", b.CodecID, b.Width, b.Height, b.Config.Descriptor())
	fmt.Fprintf(w, "   frames=%d timestamp_scale=%dns container_crc=%d ok/%d bad\n",
		len(b.Frames), b.TimestampScale, b.ContainerCRCOK, b.ContainerCRCBad)
	if b.HashPath != "" {
		fmt.Fprintf(w, "   decoded hashes: %s\n", b.HashPath)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "结论: %s (%s)\n", ZH(res.Verdict), res.Confidence)
	for _, r := range res.Reasons {
		fmt.Fprintf(w, "  - %s\n", r)
	}
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tA pts\tB pts\tΔns\tA sl\tB sl\tslice rel\tA crc\tB crc\tA sample\tB sample\tA decode\tB decode\t判定")
	for _, f := range res.Frames {
		fmt.Fprintf(tw, "%d\t%.6f\t%.6f\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			f.Index,
			float64(f.A.PTSNS)/1e9, float64(f.B.PTSNS)/1e9, f.PTSDeltaNS,
			sliceCell(f.A), sliceCell(f.B),
			sliceRelation(f.A, f.B),
			crcCell(f.A), crcCell(f.B),
			short(f.A.EncodedSHA256), short(f.B.EncodedSHA256),
			short(f.A.DecodedHash), short(f.B.DecodedHash),
			ZH(f.Verdict),
		)
	}
	tw.Flush()

	var details []compare.FrameComparison
	for _, f := range res.Frames {
		if f.Verdict != "identical" && f.Verdict != "bitstream_identical" && len(f.Reasons) > 0 {
			details = append(details, f)
		}
	}
	if len(details) > 0 {
		fmt.Fprintln(w, "\n差异原因")
		for _, f := range details {
			fmt.Fprintf(w, "  frame %d: %s\n", f.Index, ZH(f.Verdict))
			for _, r := range f.Reasons {
				fmt.Fprintf(w, "    - %s\n", r)
			}
		}
	}
	fmt.Fprintln(w, "\n说明: 本工具不调用 ffmpeg；decoded 列来自 -hash-a/-hash-b 提供的 framemd5/framecrc 侧车文件。")
}

func sliceCell(f compare.Frame) string {
	if f.SliceCount == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", f.SliceCount)
}

func sliceRelation(a, b compare.Frame) string {
	if a.SliceCount == 0 || b.SliceCount == 0 {
		return "-"
	}
	if a.SliceCount != b.SliceCount {
		return "count"
	}
	if a.SliceSetSHA256 != "" && a.SliceSetSHA256 == b.SliceSetSHA256 {
		if a.SliceSequenceSHA256 != "" && a.SliceSequenceSHA256 == b.SliceSequenceSHA256 {
			return "same"
		}
		return "permuted"
	}
	return "different"
}

func crcCell(f compare.Frame) string {
	if f.SliceCount == 0 {
		return "-"
	}
	if !f.SliceCRCEnabled {
		return "off"
	}
	if f.SliceCRCOK {
		return "ok"
	}
	return "BAD"
}

func short(s string) string {
	if s == "" {
		return "-"
	}
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

type jsonFrame struct {
	Index        int      `json:"index"`
	KeyframeA    bool     `json:"keyframe_a"`
	KeyframeB    bool     `json:"keyframe_b"`
	PTSANs       int64    `json:"pts_a_ns"`
	PTSBNs       int64    `json:"pts_b_ns"`
	PTSDeltaNs   int64    `json:"pts_delta_ns"`
	SampleSizeA  int      `json:"sample_size_a"`
	SampleSizeB  int      `json:"sample_size_b"`
	EncodedA     string   `json:"encoded_sha256_a"`
	EncodedB     string   `json:"encoded_sha256_b"`
	SliceCountA  int      `json:"slice_count_a"`
	SliceCountB  int      `json:"slice_count_b"`
	SliceSetA    string   `json:"slice_set_sha256_a"`
	SliceSetB    string   `json:"slice_set_sha256_b"`
	SliceSeqA    string   `json:"slice_sequence_sha256_a"`
	SliceSeqB    string   `json:"slice_sequence_sha256_b"`
	SliceRel     string   `json:"slice_relation"`
	SliceHashesA []string `json:"slice_payload_sha256_a,omitempty"`
	SliceHashesB []string `json:"slice_payload_sha256_b,omitempty"`
	SliceCRCA    string   `json:"slice_crc_a"`
	SliceCRCB    string   `json:"slice_crc_b"`
	DecodedA     string   `json:"decoded_hash_a,omitempty"`
	DecodedB     string   `json:"decoded_hash_b,omitempty"`
	DecodedKind  string   `json:"decoded_hash_kind,omitempty"`
	Verdict      string   `json:"verdict"`
	Reasons      []string `json:"reasons,omitempty"`
}

type jsonCapture struct {
	Path             string    `json:"path"`
	CodecID          string    `json:"codec_id"`
	Width            uint64    `json:"width"`
	Height           uint64    `json:"height"`
	Frames           int       `json:"frames"`
	TimestampScaleNS uint64    `json:"timestamp_scale_ns"`
	PixelFormat      string    `json:"pixel_format"`
	Config           any       `json:"config"`
	ContainerCRCOK   int       `json:"container_crc_ok"`
	ContainerCRCBad  int       `json:"container_crc_bad"`
	ContainerCRCs    []jsonCRC `json:"container_crc_details,omitempty"`
	HashManifest     string    `json:"hash_manifest,omitempty"`
	Issues           []string  `json:"issues,omitempty"`
}

type jsonCRC struct {
	ParentID      string `json:"parent_id"`
	ParentOffset  int64  `json:"parent_offset"`
	ElementOffset int64  `json:"element_offset"`
	Stored        uint32 `json:"stored"`
	Calculated    uint32 `json:"calculated"`
	OK            bool   `json:"ok"`
}

type jsonResult struct {
	Verdict    string         `json:"verdict"`
	Confidence string         `json:"confidence"`
	Reasons    []string       `json:"reasons,omitempty"`
	Counts     map[string]int `json:"counts"`
	A          jsonCapture    `json:"a"`
	B          jsonCapture    `json:"b"`
	Frames     []jsonFrame    `json:"frames"`
}

func WriteJSON(w io.Writer, res *compare.Result) error {
	out := jsonResult{
		Verdict:    res.Verdict,
		Confidence: res.Confidence,
		Reasons:    res.Reasons,
		Counts:     res.Counts,
		A:          captureJSON(res.A),
		B:          captureJSON(res.B),
	}
	for _, f := range res.Frames {
		out.Frames = append(out.Frames, jsonFrame{
			Index:        f.Index,
			KeyframeA:    f.A.Keyframe,
			KeyframeB:    f.B.Keyframe,
			PTSANs:       f.A.PTSNS,
			PTSBNs:       f.B.PTSNS,
			PTSDeltaNs:   f.PTSDeltaNS,
			SampleSizeA:  f.A.SampleSize,
			SampleSizeB:  f.B.SampleSize,
			EncodedA:     f.A.EncodedSHA256,
			EncodedB:     f.B.EncodedSHA256,
			SliceCountA:  f.A.SliceCount,
			SliceCountB:  f.B.SliceCount,
			SliceSetA:    f.A.SliceSetSHA256,
			SliceSetB:    f.B.SliceSetSHA256,
			SliceSeqA:    f.A.SliceSequenceSHA256,
			SliceSeqB:    f.B.SliceSequenceSHA256,
			SliceRel:     sliceRelation(f.A, f.B),
			SliceHashesA: f.A.SliceHashes,
			SliceHashesB: f.B.SliceHashes,
			SliceCRCA:    crcCell(f.A),
			SliceCRCB:    crcCell(f.B),
			DecodedA:     f.A.DecodedHash,
			DecodedB:     f.B.DecodedHash,
			DecodedKind:  f.A.DecodedHashKind,
			Verdict:      f.Verdict,
			Reasons:      f.Reasons,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func captureJSON(c *compare.Capture) jsonCapture {
	out := jsonCapture{
		Path:             c.Path,
		CodecID:          c.CodecID,
		Width:            c.Width,
		Height:           c.Height,
		Frames:           len(c.Frames),
		TimestampScaleNS: c.TimestampScale,
		PixelFormat:      c.Config.PixelFormat,
		Config:           c.Config,
		ContainerCRCOK:   c.ContainerCRCOK,
		ContainerCRCBad:  c.ContainerCRCBad,
		HashManifest:     c.HashPath,
		Issues:           c.Issues,
	}
	for _, crc := range c.ContainerCRCs {
		out.ContainerCRCs = append(out.ContainerCRCs, jsonCRC{
			ParentID:      crc.ParentID,
			ParentOffset:  crc.ParentOffset,
			ElementOffset: crc.ElementOffset,
			Stored:        crc.Stored,
			Calculated:    crc.Calculated,
			OK:            crc.OK,
		})
	}
	return out
}
