package sps

import "fmt"

// ParseH265SPS extracts VUI colour info from a raw H.265 SPS NAL unit
// (including the 2-byte NAL header).
func ParseH265SPS(nal []byte) (*Color, error) {
	if len(nal) < 3 {
		return nil, fmt.Errorf("h265 SPS NAL too short")
	}
	if (nal[0]>>1)&0x3f != 33 {
		return nil, fmt.Errorf("not an SPS NAL (type %d)", (nal[0]>>1)&0x3f)
	}
	r := newBitReader(ebspToRbsp(nal[2:]))
	r.u(4)                            // sps_video_parameter_set_id
	maxSubLayers := int(r.u(3))       // sps_max_sub_layers_minus1
	r.u(1)                            // sps_temporal_id_nesting_flag
	profileTierLevel(r, maxSubLayers) // profile_tier_level
	r.ue()                            // sps_seq_parameter_set_id
	chromaFormat := r.ue()
	if chromaFormat == 3 {
		r.u(1) // separate_colour_plane_flag
	}
	r.ue()           // pic_width_in_luma_samples
	r.ue()           // pic_height_in_luma_samples
	if r.u(1) == 1 { // conformance_window_flag
		r.ue()
		r.ue()
		r.ue()
		r.ue()
	}
	r.ue()                      // bit_depth_luma_minus8
	r.ue()                      // bit_depth_chroma_minus8
	log2MaxPocLsb := r.ue() + 4 // log2_max_pic_order_cnt_lsb_minus4
	subLayerOrdering := r.u(1)  // sps_sub_layer_ordering_info_present_flag
	start := maxSubLayers
	if subLayerOrdering == 1 {
		start = 0
	}
	for i := start; i <= maxSubLayers; i++ {
		r.ue() // sps_max_dec_pic_buffering_minus1
		r.ue() // sps_max_num_reorder_pics
		r.ue() // sps_max_latency_increase_plus1
	}
	r.ue()           // log2_min_luma_coding_block_size_minus3
	r.ue()           // log2_diff_max_min_luma_coding_block_size
	r.ue()           // log2_min_luma_transform_block_size_minus2
	r.ue()           // log2_diff_max_min_luma_transform_block_size
	r.ue()           // max_transform_hierarchy_depth_inter
	r.ue()           // max_transform_hierarchy_depth_intra
	if r.u(1) == 1 { // scaling_list_enabled_flag
		if r.u(1) == 1 { // sps_scaling_list_data_present_flag
			scalingListData(r)
		}
	}
	r.u(1)           // amp_enabled_flag
	r.u(1)           // sample_adaptive_offset_enabled_flag
	if r.u(1) == 1 { // pcm_enabled_flag
		r.u(4) // pcm_sample_bit_depth_luma_minus1
		r.u(4) // pcm_sample_bit_depth_chroma_minus1
		r.ue() // log2_min_pcm_luma_coding_block_size_minus3
		r.ue() // log2_diff_max_min_pcm_luma_coding_block_size
		r.u(1) // pcm_loop_filter_disabled_flag
	}
	numRPS := int(r.ue()) // num_short_term_ref_pic_sets
	numDeltaPocs := make([]int, numRPS)
	for i := 0; i < numRPS; i++ {
		numDeltaPocs[i] = stRefPicSet(r, i, numRPS, numDeltaPocs)
	}
	if r.u(1) == 1 { // long_term_ref_pics_present_flag
		n := r.ue() // num_long_term_ref_pics_sps
		for i := uint64(0); i < n; i++ {
			r.u(int(log2MaxPocLsb)) // lt_ref_pic_poc_lsb_sps
			r.u(1)                  // used_by_curr_pic_lt_sps_flag
		}
	}
	r.u(1)           // sps_temporal_mvp_enabled_flag
	r.u(1)           // strong_intra_smoothing_enabled_flag
	if r.u(1) == 0 { // vui_parameters_present_flag
		return nil, ErrNoVUI
	}
	return parseH265VUI(r)
}

func profileTierLevel(r *bitReader, maxSubLayers int) {
	r.u(2)  // general_profile_space
	r.u(1)  // general_tier_flag
	r.u(5)  // general_profile_idc
	r.u(32) // general_profile_compatibility_flags
	r.u(4)  // progressive/interlaced/non-packed/frame-only
	r.u(44) // general_reserved_zero_44bits
	r.u(8)  // general_level_idc
	profilePresent := make([]bool, maxSubLayers)
	levelPresent := make([]bool, maxSubLayers)
	for i := 0; i < maxSubLayers; i++ {
		profilePresent[i] = r.u(1) == 1
		levelPresent[i] = r.u(1) == 1
	}
	if maxSubLayers > 0 {
		for i := maxSubLayers; i < 8; i++ {
			r.u(2) // reserved_zero_2bits
		}
	}
	for i := 0; i < maxSubLayers; i++ {
		if profilePresent[i] {
			r.u(88)
		}
		if levelPresent[i] {
			r.u(8)
		}
	}
}

func scalingListData(r *bitReader) {
	for sizeID := 0; sizeID < 4; sizeID++ {
		maxMatrix := 6
		if sizeID == 3 {
			maxMatrix = 2
		}
		for matrixID := 0; matrixID < maxMatrix; matrixID++ {
			if r.u(1) == 0 { // scaling_list_pred_mode_flag
				r.ue() // scaling_list_pred_matrix_id_delta
				continue
			}
			coefNum := 1 << (4 + (sizeID << 1))
			if coefNum > 64 {
				coefNum = 64
			}
			if sizeID > 1 {
				r.se() // scaling_list_dc_coef_minus8
			}
			for i := 0; i < coefNum; i++ {
				r.se() // scaling_list_delta_coef
			}
		}
	}
}

// stRefPicSet parses one short-term reference picture set and returns
// its NumDeltaPocs for use by later predicted sets.
func stRefPicSet(r *bitReader, idx, numSets int, numDeltaPocs []int) int {
	if idx != 0 && r.u(1) == 1 { // inter_ref_pic_set_prediction_flag
		if idx == numSets {
			r.ue() // delta_idx_minus1
		}
		r.u(1) // delta_rps_sign
		r.ue() // abs_delta_rps_minus1
		refPocs := numDeltaPocs[idx-1]
		used := 0
		for j := 0; j <= refPocs; j++ {
			if r.u(1) == 1 { // used_by_curr_pic_flag
				used++
			} else if r.u(1) == 1 { // use_delta_flag
				used++
			}
		}
		return used
	}
	neg := int(r.ue()) // num_negative_pics
	pos := int(r.ue()) // num_positive_pics
	for i := 0; i < neg; i++ {
		r.ue() // delta_poc_s0_minus1
		r.u(1) // used_by_curr_pic_s0_flag
	}
	for i := 0; i < pos; i++ {
		r.ue() // delta_poc_s1_minus1
		r.u(1) // used_by_curr_pic_s1_flag
	}
	return neg + pos
}

func parseH265VUI(r *bitReader) (*Color, error) {
	if r.u(1) == 1 { // aspect_ratio_info_present_flag
		if r.u(8) == 255 {
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
