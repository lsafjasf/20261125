package audit

import (
	"encoding/binary"
	"encoding/hex"

	"ffv1mkaudit/ffv1"
)

func analyzeCodecPrivate(track *TrackInfo, data []byte, dataOffset int64, p *parser) {
	track.PrivateSize = len(data)
	if isFFV1Codec(track.CodecID) {
		track.FFV1 = true
		checkFFV1Config(track, data, dataOffset, p)
		return
	}
	if track.CodecID == "V_MS/VFW/FOURCC" && len(data) >= 20 {
		fourCC := string(data[16:20])
		track.CodecFourCC = fourCC
		if fourCC == "FFV1" {
			track.FFV1 = true
			if len(data) >= 12 && track.Width == 0 {
				track.Width = uint64(int32(binary.LittleEndian.Uint32(data[4:8])))
				height := int32(binary.LittleEndian.Uint32(data[8:12]))
				if height < 0 {
					height = -height
				}
				track.Height = uint64(height)
			}
			headerSize := int(binary.LittleEndian.Uint32(data[0:4]))
			if headerSize > 0 && headerSize < len(data) {
				checkFFV1Config(track, data[headerSize:], dataOffset+int64(headerSize), p)
			}
		}
	}
}

func checkFFV1Config(track *TrackInfo, config []byte, offset int64, p *parser) {
	if len(config) < 4 {
		return
	}
	_, calculated, ok := ffv1.AnalyzeConfig(config)
	track.PrivateCRC32 = hex.EncodeToString([]byte{
		byte(calculated >> 24), byte(calculated >> 16),
		byte(calculated >> 8), byte(calculated),
	})
	crcOK := ok
	track.PrivateCRCOK = &crcOK
	if !crcOK {
		p.addIssue(Issue{
			Severity: SeverityError,
			Code:     "ffv1_config_crc_mismatch",
			Message:  "FFV1 Configuration Record CRC is not zero",
			Offset:   offset,
		})
	}
}
