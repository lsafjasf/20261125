package ebml

// Matroska element IDs used by this tool.
const (
	IDEBML                  = 0x1A45DFA3
	IDDocType               = 0x4282
	IDDocTypeVersion        = 0x4287
	IDSegment               = 0x18538067
	IDInfo                  = 0x1549A966
	IDTimecodeScale         = 0x2AD7B1
	IDDuration              = 0x4489
	IDTracks                = 0x1654AE6B
	IDTrackEntry            = 0xAE
	IDTrackNumber           = 0xD7
	IDTrackUID              = 0x73C5
	IDTrackType             = 0x83
	IDCodecID               = 0x86
	IDCodecPrivate          = 0x63A2
	IDDefaultDuration       = 0x23E383
	IDVideo                 = 0xE0
	IDPixelWidth            = 0xB0
	IDPixelHeight           = 0xBA
	IDDisplayWidth          = 0x54B0
	IDDisplayHeight         = 0x54BA
	IDColour                = 0x55B0
	IDMatrixCoefficients    = 0x55B1
	IDBitsPerChannel        = 0x55B2
	IDChromaSubsamplingHorz = 0x55B3
	IDChromaSubsamplingVert = 0x55B4
	IDRange                 = 0x55B9
	IDCluster               = 0x1F43B675
	IDTimecode              = 0xE7
	IDSimpleBlock           = 0xA3
	IDBlockGroup            = 0xA0
	IDBlock                 = 0xA1
	IDReferenceBlock        = 0xFB
	IDBlockDuration         = 0x9B
	IDCues                  = 0x1C53BB6B
	IDCuePoint              = 0xBB
	IDCueTime               = 0xB3
	IDCueTrackPositions     = 0xB7
	IDCueTrack              = 0xF7
	IDCueClusterPosition    = 0xF1
	IDCueRelativePosition   = 0xF0
	IDCRC32                 = 0xBF
	IDVoid                  = 0xEC
)

// MasterIDs is the set of master elements this parser recurses into.
var MasterIDs = map[uint32]bool{
	IDEBML:              true,
	IDSegment:           true,
	IDInfo:              true,
	IDTracks:            true,
	IDTrackEntry:        true,
	IDVideo:             true,
	IDColour:            true,
	IDCluster:           true,
	IDBlockGroup:        true,
	IDCues:              true,
	IDCuePoint:          true,
	IDCueTrackPositions: true,
}

// Name returns a human-readable element name, or a hex ID.
func Name(id uint32) string {
	switch id {
	case IDEBML:
		return "EBML"
	case IDDocType:
		return "DocType"
	case IDDocTypeVersion:
		return "DocTypeVersion"
	case IDSegment:
		return "Segment"
	case IDInfo:
		return "Info"
	case IDTimecodeScale:
		return "TimecodeScale"
	case IDDuration:
		return "Duration"
	case IDTracks:
		return "Tracks"
	case IDTrackEntry:
		return "TrackEntry"
	case IDTrackNumber:
		return "TrackNumber"
	case IDTrackUID:
		return "TrackUID"
	case IDTrackType:
		return "TrackType"
	case IDCodecID:
		return "CodecID"
	case IDCodecPrivate:
		return "CodecPrivate"
	case IDDefaultDuration:
		return "DefaultDuration"
	case IDVideo:
		return "Video"
	case IDPixelWidth:
		return "PixelWidth"
	case IDPixelHeight:
		return "PixelHeight"
	case IDColour:
		return "Colour"
	case IDBitsPerChannel:
		return "BitsPerChannel"
	case IDChromaSubsamplingHorz:
		return "ChromaSubsamplingHorz"
	case IDChromaSubsamplingVert:
		return "ChromaSubsamplingVert"
	case IDCluster:
		return "Cluster"
	case IDTimecode:
		return "Timecode"
	case IDSimpleBlock:
		return "SimpleBlock"
	case IDBlockGroup:
		return "BlockGroup"
	case IDBlock:
		return "Block"
	case IDReferenceBlock:
		return "ReferenceBlock"
	case IDCRC32:
		return "CRC-32"
	case IDCues:
		return "Cues"
	}
	return "0x" + hexID(id)
}

func hexID(id uint32) string {
	const digits = "0123456789abcdef"
	var buf [8]byte
	n := 0
	for id != 0 && n < len(buf) {
		buf[len(buf)-1-n] = digits[id&0xf]
		id >>= 4
		n++
	}
	return string(buf[len(buf)-n:])
}
