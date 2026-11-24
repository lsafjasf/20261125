// Package testutil builds synthetic multi-track MP4 files with crafted
// colour metadata for testing and demo purposes.
package testutil

// BuildMixedFile creates a 3-track file:
//
//	track 1: avc1, colr BT.709, SPS VUI BT.709 (consistent SDR)
//	track 2: hvc1, NO colr, SPS VUI BT.2020/PQ full-range (HDR, untagged)
//	track 3: avc1, colr BT.709, SPS VUI BT.2020/PQ (container tag lies)
func BuildMixedFile() []byte {
	trak1 := videoTrak(1, visualSampleEntry("avc1", 1920, 1080,
		colrNclx(1, 1, 1, false),
		box("avcC", avcC(h264SPS(1, 1, 1, false)))))
	trak2 := videoTrak(2, visualSampleEntry("hvc1", 1920, 1080,
		box("hvcC", hvcC(h265SPS(9, 16, 9, true)))))
	trak3 := videoTrak(3, visualSampleEntry("avc1", 1920, 1080,
		colrNclx(1, 1, 1, false),
		box("avcC", avcC(h264SPS(9, 16, 9, true)))))
	return buildMP4(trak1, trak2, trak3)
}
