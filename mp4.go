package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

type Box struct {
	Type         [4]byte
	Start        int64
	HeaderSize   int64
	PayloadStart int64
	End          int64
	DeclaredSize int64
	ExtendedSize bool
}

type TrackDisplayInfo struct {
	HandlerType   string   `json:"handlerType"`
	Version       byte     `json:"version"`
	Matrix        [9]int64 `json:"matrix"`
	DisplayWidth  float64  `json:"displayWidth"`
	DisplayHeight float64  `json:"displayHeight"`
	TKHDStart     int64    `json:"tkhdStart"`
	MatrixOffset  int64    `json:"matrixOffset"`
	WidthOffset   int64    `json:"widthOffset"`
	HeightOffset  int64    `json:"heightOffset"`
}

func (b Box) IsType(want string) bool {
	return string(b.Type[:]) == want
}

func listBoxesAt(f *os.File, start, end int64) ([]Box, error) {
	boxes := make([]Box, 0, 8)
	p := start
	var header [16]byte
	for p+8 <= end {
		if _, err := f.ReadAt(header[:8], p); err != nil {
			if errors.Is(err, io.EOF) && len(boxes) > 0 {
				break
			}
			return nil, fmt.Errorf("read box header at %d: %w", p, err)
		}
		size32 := binary.BigEndian.Uint32(header[0:4])
		boxType := header[4:8]
		headerSize := int64(8)
		size := int64(size32)
		extended := false
		if size32 == 1 {
			if _, err := f.ReadAt(header[8:16], p+8); err != nil {
				return nil, fmt.Errorf("read extended box size at %d: %w", p+8, err)
			}
			size = int64(binary.BigEndian.Uint64(header[8:16]))
			headerSize = 16
			extended = true
		} else if size32 == 0 {
			size = end - p
		}
		if size < headerSize || p+size > end {
			return nil, fmt.Errorf("invalid %q box size %d at %d (parent end %d)", string(boxType), size, p, end)
		}
		boxes = append(boxes, Box{
			Type:         [4]byte{boxType[0], boxType[1], boxType[2], boxType[3]},
			Start:        p,
			HeaderSize:   headerSize,
			PayloadStart: p + headerSize,
			End:          p + size,
			DeclaredSize: size,
			ExtendedSize: extended,
		})
		p += size
	}
	return boxes, nil
}

func firstBox(boxes []Box, typ string) *Box {
	for i := range boxes {
		if boxes[i].IsType(typ) {
			return &boxes[i]
		}
	}
	return nil
}

func ReadMP4TrackDisplays(path string) ([]TrackDisplayInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	top, err := listBoxesAt(f, 0, st.Size())
	if err != nil {
		return nil, err
	}
	moov := firstBox(top, "moov")
	if moov == nil {
		return nil, nil
	}
	traks, err := listBoxesAt(f, moov.PayloadStart, moov.End)
	if err != nil {
		return nil, err
	}

	result := make([]TrackDisplayInfo, 0, 2)
	for _, trakBox := range traks {
		if !trakBox.IsType("trak") {
			continue
		}
		children, err := listBoxesAt(f, trakBox.PayloadStart, trakBox.End)
		if err != nil {
			return nil, err
		}
		tkhdBox := firstBox(children, "tkhd")
		if tkhdBox == nil {
			continue
		}
		info, err := parseTKHD(f, *tkhdBox)
		if err != nil {
			return nil, err
		}
		if mdia := firstBox(children, "mdia"); mdia != nil {
			mdiaChildren, err := listBoxesAt(f, mdia.PayloadStart, mdia.End)
			if err == nil {
				if hdlr := firstBox(mdiaChildren, "hdlr"); hdlr != nil && hdlr.End-hdlr.PayloadStart >= 12 {
					var handler [4]byte
					if _, err := f.ReadAt(handler[:], hdlr.PayloadStart+8); err == nil {
						info.HandlerType = string(handler[:])
					}
				}
			}
		}
		result = append(result, info)
	}
	return result, nil
}

func parseTKHD(f *os.File, box Box) (TrackDisplayInfo, error) {
	var version [1]byte
	if _, err := f.ReadAt(version[:], box.PayloadStart); err != nil {
		return TrackDisplayInfo{}, err
	}
	preFieldLen := int64(20) // v0: creation/mod/track/reserved/duration
	if version[0] == 1 {
		preFieldLen = 32 // v1 uses 64-bit creation/modification/duration
	}
	// version+flags(4), five timing/id fields, reserved(8), layer(2),
	// alternate group(2), volume(2), reserved(2).
	matrixOffset := box.PayloadStart + 4 + preFieldLen + 16
	widthOffset := matrixOffset + 36
	heightOffset := widthOffset + 4

	var rawMatrix [36]byte
	if _, err := f.ReadAt(rawMatrix[:], matrixOffset); err != nil {
		return TrackDisplayInfo{}, fmt.Errorf("read tkhd matrix: %w", err)
	}
	info := TrackDisplayInfo{
		Version:      version[0],
		TKHDStart:    box.Start,
		MatrixOffset: matrixOffset,
		WidthOffset:  widthOffset,
		HeightOffset: heightOffset,
	}
	for i := 0; i < 9; i++ {
		info.Matrix[i] = int64(int32(binary.BigEndian.Uint32(rawMatrix[i*4 : i*4+4])))
	}
	var fixed [4]byte
	if _, err := f.ReadAt(fixed[:], widthOffset); err != nil {
		return TrackDisplayInfo{}, err
	}
	info.DisplayWidth = float64(binary.BigEndian.Uint32(fixed[:])) / 65536.0
	if _, err := f.ReadAt(fixed[:], heightOffset); err != nil {
		return TrackDisplayInfo{}, err
	}
	info.DisplayHeight = float64(binary.BigEndian.Uint32(fixed[:])) / 65536.0
	return info, nil
}

func rotationMatrixRaw(turnsCW int) [9]int32 {
	one := int32(0x00010000)
	minusOne := int32(-0x00010000)
	identityScale := int32(0x40000000)
	switch normTurnsCW(turnsCW) {
	case 1:
		return [9]int32{0, one, 0, minusOne, 0, 0, 0, 0, identityScale}
	case 2:
		return [9]int32{minusOne, 0, 0, 0, minusOne, 0, 0, 0, identityScale}
	case 3:
		return [9]int32{0, minusOne, 0, one, 0, 0, 0, 0, identityScale}
	default:
		return [9]int32{one, 0, 0, 0, one, 0, 0, 0, identityScale}
	}
}

func PatchTKHDTransform(path string, turnsCW, displayWidth, displayHeight int) error {
	tracks, err := ReadMP4TrackDisplays(path)
	if err != nil {
		return err
	}
	if len(tracks) == 0 {
		return errors.New("no MP4 tkhd box found; cannot patch display matrix")
	}
	track := tracks[0]
	for _, candidate := range tracks {
		if candidate.HandlerType == "vide" {
			track = candidate
			break
		}
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	matrix := rotationMatrixRaw(turnsCW)
	var matrixBytes [36]byte
	for i, v := range matrix {
		binary.BigEndian.PutUint32(matrixBytes[i*4:i*4+4], uint32(v))
	}
	if _, err := f.WriteAt(matrixBytes[:], track.MatrixOffset); err != nil {
		return err
	}
	var dimension [4]byte
	binary.BigEndian.PutUint32(dimension[:], uint32(displayWidth<<16))
	if _, err := f.WriteAt(dimension[:], track.WidthOffset); err != nil {
		return err
	}
	binary.BigEndian.PutUint32(dimension[:], uint32(displayHeight<<16))
	if _, err := f.WriteAt(dimension[:], track.HeightOffset); err != nil {
		return err
	}
	return nil
}
