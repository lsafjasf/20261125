package testutil

import (
	"bytes"
	"encoding/binary"
)

// bitWriter writes bits MSB-first for crafting SPS bitstreams.
type bitWriter struct {
	buf bytes.Buffer
	acc uint32
	n   int
}

func (w *bitWriter) u(v uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		w.acc = w.acc<<1 | uint32((v>>i)&1)
		w.n++
		if w.n == 8 {
			w.buf.WriteByte(byte(w.acc))
			w.acc, w.n = 0, 0
		}
	}
}

func (w *bitWriter) ue(v uint64) {
	codeNum := v + 1
	bits := 0
	for tmp := codeNum; tmp > 0; tmp >>= 1 {
		bits++
	}
	for i := 0; i < bits-1; i++ {
		w.u(0, 1)
	}
	w.u(codeNum, bits)
}

func (w *bitWriter) rbspStop() {
	w.u(1, 1)
	for w.n != 0 {
		w.u(0, 1)
	}
}

// h264SPS builds a minimal H.264 SPS NAL with VUI colour description.
func h264SPS(primaries, transfer, matrix byte, fullRange bool) []byte {
	w := &bitWriter{}
	w.u(66, 8) // profile_idc baseline
	w.u(0, 8)  // constraint flags
	w.u(30, 8) // level_idc
	w.ue(0)    // sps id
	w.ue(0)    // log2_max_frame_num_minus4
	w.ue(0)    // pic_order_cnt_type
	w.ue(0)    // log2_max_pic_order_cnt_lsb_minus4
	w.ue(1)    // max_num_ref_frames
	w.u(0, 1)  // gaps
	w.ue(119)  // width mbs
	w.ue(67)   // height map units
	w.u(1, 1)  // frame_mbs_only
	w.u(1, 1)  // direct_8x8
	w.u(0, 1)  // cropping
	w.u(1, 1)  // vui present
	w.u(0, 1)  // aspect ratio
	w.u(0, 1)  // overscan
	w.u(1, 1)  // video signal type
	w.u(5, 3)  // video format
	if fullRange {
		w.u(1, 1)
	} else {
		w.u(0, 1)
	}
	w.u(1, 1) // colour description present
	w.u(uint64(primaries), 8)
	w.u(uint64(transfer), 8)
	w.u(uint64(matrix), 8)
	w.rbspStop()
	return append([]byte{0x67}, w.buf.Bytes()...)
}

// h265SPS builds a minimal H.265 SPS NAL with VUI colour description.
func h265SPS(primaries, transfer, matrix byte, fullRange bool) []byte {
	w := &bitWriter{}
	w.u(0, 4)           // vps id
	w.u(0, 3)           // max_sub_layers_minus1
	w.u(1, 1)           // temporal nesting
	w.u(0, 2)           // profile space
	w.u(0, 1)           // tier
	w.u(2, 5)           // profile_idc Main10
	w.u(0x60000000, 32) // compat flags
	w.u(0, 4)           // progressive/interlaced/non-packed/frame-only
	w.u(0, 44)          // reserved
	w.u(120, 8)         // level
	w.ue(0)             // sps id
	w.ue(1)             // chroma 4:2:0
	w.ue(1920)          // width
	w.ue(1080)          // height
	w.u(0, 1)           // conformance window
	w.ue(2)             // bit depth luma (10-bit)
	w.ue(2)             // bit depth chroma
	w.ue(4)             // log2_max_pic_order_cnt_lsb_minus4
	w.u(0, 1)           // sub_layer_ordering_info_present
	w.ue(0)             // max_dec_pic_buffering
	w.ue(0)             // max_num_reorder
	w.ue(0)             // max_latency
	w.ue(0)             // log2_min_coding_block
	w.ue(3)             // log2_diff_max_min_coding_block
	w.ue(0)             // log2_min_transform_block
	w.ue(3)             // log2_diff_max_min_transform_block
	w.ue(0)             // hierarchy inter
	w.ue(0)             // hierarchy intra
	w.u(0, 1)           // scaling list
	w.u(0, 1)           // amp
	w.u(0, 1)           // sao
	w.u(0, 1)           // pcm
	w.ue(0)             // num_short_term_ref_pic_sets
	w.u(0, 1)           // long_term_ref_pics
	w.u(0, 1)           // temporal mvp
	w.u(0, 1)           // strong intra smoothing
	w.u(1, 1)           // vui present
	w.u(0, 1)           // aspect ratio
	w.u(0, 1)           // overscan
	w.u(1, 1)           // video signal type
	w.u(5, 3)           // video format
	if fullRange {
		w.u(1, 1)
	} else {
		w.u(0, 1)
	}
	w.u(1, 1) // colour description present
	w.u(uint64(primaries), 8)
	w.u(uint64(transfer), 8)
	w.u(uint64(matrix), 8)
	w.rbspStop()
	return append([]byte{0x42, 0x01}, w.buf.Bytes()...)
}

func avcC(spsNAL []byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{1, 66, 0, 30, 0xFF, 0xE1})
	binary.Write(&b, binary.BigEndian, uint16(len(spsNAL)))
	b.Write(spsNAL)
	b.WriteByte(0) // num PPS
	return b.Bytes()
}

func hvcC(spsNAL []byte) []byte {
	hdr := make([]byte, 23)
	hdr[0] = 1 // configurationVersion
	hdr[22] = 1
	var b bytes.Buffer
	b.Write(hdr)
	b.WriteByte(0xA0 | 33) // array_completeness + SPS type
	binary.Write(&b, binary.BigEndian, uint16(1))
	binary.Write(&b, binary.BigEndian, uint16(len(spsNAL)))
	b.Write(spsNAL)
	return b.Bytes()
}
