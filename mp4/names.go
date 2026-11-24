package mp4

import "strconv"

// PrimariesName maps ISO/IEC 23001-8 colour primaries codes.
func PrimariesName(v uint16) string {
	switch v {
	case 1:
		return "BT.709"
	case 4:
		return "BT.470M"
	case 5:
		return "BT.601-625"
	case 6:
		return "BT.601-525"
	case 7:
		return "SMPTE-240M"
	case 8:
		return "Generic film"
	case 9:
		return "BT.2020"
	case 10:
		return "XYZ"
	case 11:
		return "SMPTE-431 (DCI-P3)"
	case 12:
		return "SMPTE-432 (Display P3)"
	case 22:
		return "EBU Tech 3213"
	}
	return "reserved(" + strconv.Itoa(int(v)) + ")"
}

// TransferName maps ISO/IEC 23001-8 transfer characteristic codes.
func TransferName(v uint16) string {
	switch v {
	case 1:
		return "BT.709"
	case 4:
		return "Gamma 2.2"
	case 5:
		return "Gamma 2.8"
	case 6:
		return "BT.601"
	case 7:
		return "SMPTE-240M"
	case 8:
		return "Linear"
	case 13:
		return "sRGB"
	case 14:
		return "BT.2020 10-bit"
	case 15:
		return "BT.2020 12-bit"
	case 16:
		return "PQ (SMPTE ST 2084)"
	case 17:
		return "SMPTE-428"
	case 18:
		return "HLG (ARIB STD-B67)"
	}
	return "reserved(" + strconv.Itoa(int(v)) + ")"
}

// MatrixName maps ISO/IEC 23001-8 matrix coefficient codes.
func MatrixName(v uint16) string {
	switch v {
	case 0:
		return "Identity/GBR"
	case 1:
		return "BT.709"
	case 4:
		return "FCC"
	case 5:
		return "BT.470BG"
	case 6:
		return "BT.601"
	case 7:
		return "SMPTE-240M"
	case 9:
		return "BT.2020 non-constant"
	case 10:
		return "BT.2020 constant"
	case 14:
		return "BT.2100 ICtCp"
	}
	return "reserved(" + strconv.Itoa(int(v)) + ")"
}

// IsHDRTransfer reports whether a transfer code signals HDR.
func IsHDRTransfer(v uint16) bool { return v == 16 || v == 18 }
