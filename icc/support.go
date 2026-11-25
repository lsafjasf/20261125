package icc

// Declared matrix/TRC subset. Anything not listed here is reported as
// unsupported rather than interpreted.
var supportedTags = map[string]TagKind{
	"rXYZ": TagMatrix,
	"gXYZ": TagMatrix,
	"bXYZ": TagMatrix,
	"wtpt": TagMatrix,
	"rTRC": TagTRC,
	"gTRC": TagTRC,
	"bTRC": TagTRC,
}

// classifyTag maps a raw signature to the subset classification.
func classifyTag(sig string) TagKind {
	if kind, ok := supportedTags[sig]; ok {
		return kind
	}
	return TagUnsupported
}

func tagKindName(k TagKind) string {
	switch k {
	case TagMatrix:
		return "matrix"
	case TagTRC:
		return "TRC"
	default:
		return "unsupported"
	}
}
