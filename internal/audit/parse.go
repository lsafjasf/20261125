package audit

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// AuditFile opens path strictly read-only and audits its edit lists.
func AuditFile(path string) (FileReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileReport{Path: path, Error: err.Error()}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return FileReport{Path: path, Error: err.Error()}, err
	}
	report := AuditReaderAt(f, info.Size(), path)
	return report, nil
}

// AuditReaderAt audits an in-memory or file-backed BMFF/MOV representation.
func AuditReaderAt(r io.ReaderAt, size int64, path string) FileReport {
	report := FileReport{Path: path, FileSize: size}

	top, err := ParseBoxes(r, 0, size)
	if err != nil {
		report.Error = err.Error()
		report.Findings = append(report.Findings, Finding{
			Severity: "error", Code: "box_parse_failed", Message: err.Error(),
		})
		return report
	}
	report.Boxes = top

	moov := FindChild(top, "moov")
	if moov == nil {
		report.Error = "moov box not found"
		report.Findings = append(report.Findings, Finding{
			Severity: "error", Code: "moov_missing", Message: report.Error, BoxPath: "/moov",
		})
		return report
	}

	mvhd := FindChild(moov.Children, "mvhd")
	movie, finding := parseMVHD(mvhd)
	if finding != nil {
		report.Findings = append(report.Findings, *finding)
	}
	report.Movie = movie

	traks := FindAll(moov.Children, "trak")
	for trackIndex, trak := range traks {
		track := auditTrack(trak, trackIndex, movie)
		report.Tracks = append(report.Tracks, track)
		report.Findings = append(report.Findings, track.Findings...)
	}

	return report
}

func parseMVHD(box *Box) (*MovieInfo, *Finding) {
	if box == nil {
		return nil, &Finding{Severity: "error", Code: "mvhd_missing", Message: "mvhd box not found", BoxPath: "/moov/mvhd"}
	}
	p, err := box.Payload()
	if err != nil {
		return nil, &Finding{Severity: "error", Code: "mvhd_read_failed", Message: err.Error(), BoxPath: "/moov/mvhd"}
	}
	version := p[0]
	if version == 1 {
		if len(p) < 32 {
			return nil, shortPayloadFinding("mvhd", len(p), 32)
		}
		ts := binary.BigEndian.Uint32(p[20:24])
		dur := binary.BigEndian.Uint64(p[24:32])
		return &MovieInfo{Timescale: ts, Duration: dur, DurationSecond: rationalSeconds(dur, ts)}, nil
	}
	if len(p) < 20 {
		return nil, shortPayloadFinding("mvhd", len(p), 20)
	}
	ts := binary.BigEndian.Uint32(p[12:16])
	dur := uint64(binary.BigEndian.Uint32(p[16:20]))
	return &MovieInfo{Timescale: ts, Duration: dur, DurationSecond: rationalSeconds(dur, ts)}, nil
}

func auditTrack(trak *Box, index int, movie *MovieInfo) TrackReport {
	track := TrackReport{Index: index}
	trackPath := fmt.Sprintf("/moov/trak[%d]", index)

	if tkhdBox := FindChild(trak.Children, "tkhd"); tkhdBox != nil {
		info, finding := parseTKHD(tkhdBox)
		if finding != nil {
			finding.TrackIndex = index
			track.Findings = append(track.Findings, *finding)
		}
		track.TKHD = info
		if info != nil {
			track.TrackID = info.TrackID
		}
	} else {
		track.Findings = append(track.Findings, Finding{
			Severity: "warning", Code: "tkhd_missing", Message: "trak has no tkhd box",
			BoxPath: trackPath + "/tkhd", TrackIndex: index,
		})
	}

	mdia := FindChild(trak.Children, "mdia")
	if mdia == nil {
		track.Findings = append(track.Findings, Finding{
			Severity: "error", Code: "mdia_missing", Message: "trak has no mdia box",
			BoxPath: trackPath + "/mdia", TrackIndex: index,
		})
		return track
	}

	if hdlrBox := FindChild(mdia.Children, "hdlr"); hdlrBox != nil {
		if h, err := parseHDLR(hdlrBox); err == nil {
			track.Handler = h
			track.IsAudio = h == "soun"
		} else {
			track.Findings = append(track.Findings, Finding{
				Severity: "warning", Code: "hdlr_parse_failed", Message: err.Error(),
				BoxPath: trackPath + "/mdia/hdlr", TrackIndex: index,
			})
		}
	}

	mdhd, finding := parseMDHD(FindChild(mdia.Children, "mdhd"))
	if finding != nil {
		finding.TrackIndex = index
		track.Findings = append(track.Findings, *finding)
	}
	track.MDHD = mdhd

	edts := FindChild(trak.Children, "edts")
	if edts == nil {
		track.Findings = append(track.Findings, Finding{
			Severity: "info", Code: "edts_absent",
			Message: "no edts/elst; media timeline starts at media time 0 and composition time 0",
			BoxPath: trackPath + "/edts", TrackIndex: index,
		})
		return track
	}

	elstBox := FindChild(edts.Children, "elst")
	elst, rawEdits, finding := parseELST(elstBox)
	if finding != nil {
		finding.TrackIndex = index
		track.Findings = append(track.Findings, *finding)
		return track
	}
	track.ELST = elst

	edits, start, calcFindings := calculateEdits(rawEdits, movie, track.TKHD, track.MDHD, index)
	track.Edits = edits
	track.AudioStart = start
	track.Findings = append(track.Findings, calcFindings...)
	return track
}

func parseTKHD(box *Box) (*TKHDInfo, *Finding) {
	p, err := box.Payload()
	if err != nil {
		return nil, &Finding{Severity: "error", Code: "tkhd_read_failed", Message: err.Error(), BoxPath: "/moov/trak/tkhd"}
	}
	v := p[0]
	if v == 1 {
		if len(p) < 36 {
			return nil, shortPayloadFinding("tkhd", len(p), 36)
		}
		return &TKHDInfo{Version: v, TrackID: binary.BigEndian.Uint32(p[20:24]), Duration: binary.BigEndian.Uint64(p[28:36])}, nil
	}
	if len(p) < 24 {
		return nil, shortPayloadFinding("tkhd", len(p), 24)
	}
	return &TKHDInfo{Version: v, TrackID: binary.BigEndian.Uint32(p[12:16]), Duration: uint64(binary.BigEndian.Uint32(p[20:24]))}, nil
}

func parseMDHD(box *Box) (*MDHDInfo, *Finding) {
	if box == nil {
		return nil, &Finding{Severity: "warning", Code: "mdhd_missing", Message: "mdia/mdhd not found", BoxPath: "/moov/trak/mdia/mdhd"}
	}
	p, err := box.Payload()
	if err != nil {
		return nil, &Finding{Severity: "error", Code: "mdhd_read_failed", Message: err.Error(), BoxPath: "/moov/trak/mdia/mdhd"}
	}
	v := p[0]
	if v == 1 {
		if len(p) < 32 {
			return nil, shortPayloadFinding("mdhd", len(p), 32)
		}
		return &MDHDInfo{Version: v, Timescale: binary.BigEndian.Uint32(p[20:24]), Duration: binary.BigEndian.Uint64(p[24:32])}, nil
	}
	if len(p) < 20 {
		return nil, shortPayloadFinding("mdhd", len(p), 20)
	}
	return &MDHDInfo{Version: v, Timescale: binary.BigEndian.Uint32(p[12:16]), Duration: uint64(binary.BigEndian.Uint32(p[16:20]))}, nil
}

func parseHDLR(box *Box) (string, error) {
	p, err := box.Payload()
	if err != nil {
		return "", err
	}
	if len(p) < 12 {
		return "", fmt.Errorf("hdlr payload too short: %d", len(p))
	}
	return fourCC(p[8:12]), nil
}

func shortPayloadFinding(boxType string, got, want int) *Finding {
	return &Finding{
		Severity: "error",
		Code:     "short_" + boxType + "_payload",
		Message:  fmt.Sprintf("%s payload length %d, want at least %d", boxType, got, want),
		BoxPath:  "/moov/**/" + boxType,
	}
}
