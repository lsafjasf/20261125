package ebml

import (
	"fmt"
)

const (
	IDEBML            uint32 = 0x1A45DFA3
	IDDocType         uint32 = 0x4282
	IDDocTypeVersion  uint32 = 0x4287
	IDSegment         uint32 = 0x18538067
	IDSeekHead        uint32 = 0x114D9B74
	IDSeek            uint32 = 0x4DBB
	IDSeekID          uint32 = 0x53AB
	IDSeekPosition    uint32 = 0x53AC
	IDInfo            uint32 = 0x1549A966
	IDTimecodeScale   uint32 = 0x2AD7B1
	IDTimestampScale  uint32 = 0x2AD7B1
	IDDuration        uint32 = 0x4489
	IDMuxingApp       uint32 = 0x4D80
	IDWritingApp      uint32 = 0x5741
	IDTracks          uint32 = 0x1654AE6B
	IDTrackEntry      uint32 = 0xAE
	IDTrackNumber     uint32 = 0xD7
	IDTrackUID        uint32 = 0x73C5
	IDTrackType       uint32 = 0x83
	IDCodecID         uint32 = 0x86
	IDCodecPrivate    uint32 = 0x63A2
	IDDefaultDuration uint32 = 0x23E383
	IDVideo           uint32 = 0xE0
	IDPixelWidth      uint32 = 0xB0
	IDPixelHeight     uint32 = 0xBA
	IDCluster         uint32 = 0x1F43B675
	IDTimecode        uint32 = 0xE7
	IDSimpleBlock     uint32 = 0xA3
	IDBlockGroup      uint32 = 0xA0
	IDBlock           uint32 = 0xA1
	IDBlockDuration   uint32 = 0x9B
	IDReferenceBlock  uint32 = 0xFB
	IDCues            uint32 = 0x1C53BB6B
	IDCuePoint        uint32 = 0xBB
	IDCueTime         uint32 = 0xB3
	IDCueTrackPos     uint32 = 0xB7
	IDCueTrack        uint32 = 0xF7
	IDCueClusterPos   uint32 = 0xF1
	IDCueRelativePos  uint32 = 0xF0
	IDCueBlockNumber  uint32 = 0x5378
	IDCRC32           uint32 = 0xBF
	IDVoid            uint32 = 0xEC
)

var knownIDs = map[uint32]string{
	IDEBML: "EBML", IDDocType: "DocType", IDDocTypeVersion: "DocTypeVersion",
	IDSegment: "Segment", IDSeekHead: "SeekHead", IDSeek: "Seek",
	IDSeekID: "SeekID", IDSeekPosition: "SeekPosition", IDInfo: "Info",
	IDTimecodeScale: "TimecodeScale", IDDuration: "Duration",
	IDMuxingApp: "MuxingApp", IDWritingApp: "WritingApp", IDTracks: "Tracks",
	IDTrackEntry: "TrackEntry", IDTrackNumber: "TrackNumber",
	IDTrackUID: "TrackUID", IDTrackType: "TrackType", IDCodecID: "CodecID",
	IDCodecPrivate: "CodecPrivate", IDDefaultDuration: "DefaultDuration",
	IDVideo: "Video", IDPixelWidth: "PixelWidth", IDPixelHeight: "PixelHeight",
	IDCluster: "Cluster", IDTimecode: "Timecode", IDSimpleBlock: "SimpleBlock",
	IDBlockGroup: "BlockGroup", IDBlock: "Block", IDBlockDuration: "BlockDuration",
	IDReferenceBlock: "ReferenceBlock", IDCues: "Cues", IDCuePoint: "CuePoint",
	IDCueTime: "CueTime", IDCueTrackPos: "CueTrackPositions",
	IDCueTrack: "CueTrack", IDCueClusterPos: "CueClusterPosition",
	IDCueRelativePos: "CueRelativePosition", IDCueBlockNumber: "CueBlockNumber",
	IDCRC32: "CRC-32", IDVoid: "Void",
}

var containerIDs = map[uint32]bool{}

func Name(id uint32) string {
	if name, ok := knownIDs[id]; ok {
		return name
	}
	return "Unknown(" + HexID(id) + ")"
}

func IsKnown(id uint32) bool {
	_, ok := knownIDs[id]
	if !ok {
		_, ok = generatedNames[id]
	}
	return ok
}

func IsContainer(id uint32) bool {
	return containerIDs[id] || generatedContainers[id]
}

func HexID(id uint32) string {
	return fmt.Sprintf("0x%X", id)
}

func HexIDPadded(id uint32, size int) string {
	return fmt.Sprintf("0x%0*X", size*2, id)
}
