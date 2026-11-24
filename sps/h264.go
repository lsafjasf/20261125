package sps

import "fmt"

// Color carries VUI colour description fields from a video bitstream.
type Color struct {
	Primaries uint16
	Transfer  uint16
	Matrix    uint16
	FullRange bool
}

// h264HighProfiles are profiles carrying the chroma_format_idc block.
var h264HighProfiles = map[uint64]bool{
	100: true, 110: true, 122: true, 244: true, 44: true,
	83: true, 86: true, 118: true, 128: true,
	138: true, 139: true, 134: true, 135: true,
}

// ParseH264SPS extracts VUI colour info from a raw H.264 SPS NAL unit
// (including the 1-byte NAL header).
func ParseH264SPS(nal []byte) (*Color, error) {
	if len(nal) < 2 {
		return nil, fmt.Errorf("h264 SPS NAL too short")
	}
	if nal[0]&0x1f != 7 {
		return nil, fmt.Errorf("not an SPS NAL (type %d)", nal[0]&0x1f)
	}
	r := newBitReader(ebspToRbsp(nal[1:]))
	profile := r.u(8)
	r.u(8) // constraint flags + reserved
	r.u(8) // level_idc
	r.ue() // seq_parameter_set_id
	if h264HighProfiles[profile] {
		chromaFormat := r.ue()
		if chromaFormat == 3 {
			r.u(1) // separate_colour_plane_flag
		}
		r.ue()           // bit_depth_luma_minus8
		r.ue()           // bit_depth_chroma_minus8
		r.u(1)           // qpprime_y_zero_transform_bypass_flag
		if r.u(1) == 1 { // seq_scaling_matrix_present_flag
			n := 8
			if chromaFormat != 3 {
				n = 12
			}
			for i := 0; i < n; i++ {
				if r.u(1) == 1 {
					skipScalingList(r, scalingListSize(i))
				}
			}
		}
	}
	r.ue() // log2_max_frame_num_minus4
	pocType := r.ue()
	if pocType == 0 {
		r.ue() // log2_max_pic_order_cnt_lsb_minus4
	} else if pocType == 1 {
		r.u(1) // delta_pic_order_always_zero_flag
		r.se() // offset_for_non_ref_pic
		r.se() // offset_for_top_to_bottom_field
		n := r.ue()
		for i := uint64(0); i < n; i++ {
			r.se()
		}
	}
	r.ue()           // max_num_ref_frames
	r.u(1)           // gaps_in_frame_num_value_allowed_flag
	r.ue()           // pic_width_in_mbs_minus1
	r.ue()           // pic_height_in_map_units_minus1
	if r.u(1) == 0 { // frame_mbs_only_flag
		r.u(1) // mb_adaptive_frame_field_flag
	}
	r.u(1)           // direct_8x8_inference_flag
	if r.u(1) == 1 { // frame_cropping_flag
		r.ue()
		r.ue()
		r.ue()
		r.ue()
	}
	if r.u(1) == 0 { // vui_parameters_present_flag
		return nil, ErrNoVUI
	}
	return parseH264VUI(r)
}

func scalingListSize(i int) int {
	if i < 6 {
		return 16
	}
	return 64
}

func skipScalingList(r *bitReader, size int) {
	lastScale, nextScale := 8, 8
	for j := 0; j < size; j++ {
		if nextScale != 0 {
			nextScale = (lastScale + int(r.se()) + 256) % 256
		}
		if nextScale != 0 {
			lastScale = nextScale
		}
	}
}

func parseH264VUI(r *bitReader) (*Color, error) {
	if r.u(1) == 1 { // aspect_ratio_info_present_flag
		idc := r.u(8)
		if idc == 255 {
			r.u(16)
			r.u(16)
		}
	}
	if r.u(1) == 1 { // overscan_info_present_flag
		r.u(1)
	}
	if r.u(1) == 0 { // video_signal_type_present_flag
		return nil, ErrNoVUI
	}
	r.u(3) // video_format
	fullRange := r.u(1) == 1
	if r.u(1) == 0 { // colour_description_present_flag
		return nil, ErrNoVUI
	}
	if r.err != nil {
		return nil, r.err
	}
	return &Color{
		Primaries: uint16(r.u(8)),
		Transfer:  uint16(r.u(8)),
		Matrix:    uint16(r.u(8)),
		FullRange: fullRange,
	}, nil
}
