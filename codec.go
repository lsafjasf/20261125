package transparentvideo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

func Encode(writer io.Writer, video *Video) (int64, error) {
	if err := validateVideoShape(video); err != nil {
		return 0, err
	}

	buffer := bytes.NewBuffer(nil)
	if _, err := buffer.WriteString(formatMagic); err != nil {
		return 0, err
	}
	if err := binary.Write(buffer, binary.BigEndian, video.Version); err != nil {
		return 0, err
	}
	if err := binary.Write(buffer, binary.BigEndian, uint32(video.Width)); err != nil {
		return 0, err
	}
	if err := binary.Write(buffer, binary.BigEndian, uint32(video.Height)); err != nil {
		return 0, err
	}
	if err := writeString(buffer, string(video.Mode)); err != nil {
		return 0, err
	}
	if err := binary.Write(buffer, binary.BigEndian, uint32(len(video.Frames))); err != nil {
		return 0, err
	}

	for _, frame := range video.Frames {
		if err := writeString(buffer, frame.Purpose); err != nil {
			return 0, err
		}
		if err := binary.Write(buffer, binary.BigEndian, uint32(frame.Width)); err != nil {
			return 0, err
		}
		if err := binary.Write(buffer, binary.BigEndian, uint32(frame.Height)); err != nil {
			return 0, err
		}
		if err := binary.Write(buffer, binary.BigEndian, uint32(len(frame.Pix))); err != nil {
			return 0, err
		}
		if _, err := buffer.Write(frame.Pix); err != nil {
			return 0, err
		}
	}

	written, err := writer.Write(buffer.Bytes())
	return int64(written), err
}

func Decode(reader io.Reader) (*Video, error) {
	magic := make([]byte, len(formatMagic))
	if _, err := io.ReadFull(reader, magic); err != nil {
		return nil, err
	}
	if string(magic) != formatMagic {
		return nil, errors.New("invalid transparent video magic")
	}

	var version uint16
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil {
		return nil, err
	}
	if version != formatVersion {
		return nil, fmt.Errorf("unsupported transparent video version %d", version)
	}

	width, err := readUint32(reader)
	if err != nil {
		return nil, err
	}
	height, err := readUint32(reader)
	if err != nil {
		return nil, err
	}
	mode, err := readString(reader)
	if err != nil {
		return nil, err
	}
	frameCount, err := readUint32(reader)
	if err != nil {
		return nil, err
	}

	video := &Video{
		Version: version,
		Width:   int(width),
		Height:  int(height),
		Mode:    AlphaMode(mode),
		Frames:  make([]Frame, 0, frameCount),
	}
	for index := uint32(0); index < frameCount; index++ {
		purpose, err := readString(reader)
		if err != nil {
			return nil, err
		}
		frameWidth, err := readUint32(reader)
		if err != nil {
			return nil, err
		}
		frameHeight, err := readUint32(reader)
		if err != nil {
			return nil, err
		}
		pixelLength, err := readUint32(reader)
		if err != nil {
			return nil, err
		}
		if pixelLength%4 != 0 {
			return nil, errors.New("frame pixel data must contain RGBA bytes")
		}
		pix := make([]byte, pixelLength)
		if _, err := io.ReadFull(reader, pix); err != nil {
			return nil, err
		}
		video.Frames = append(video.Frames, Frame{
			Purpose: purpose,
			Width:   int(frameWidth),
			Height:  int(frameHeight),
			Pix:     pix,
		})
	}

	if err := validateVideoShape(video); err != nil {
		return nil, err
	}
	return video, VerifyVideo(video)
}

func writeString(buffer *bytes.Buffer, value string) error {
	if err := binary.Write(buffer, binary.BigEndian, uint32(len(value))); err != nil {
		return err
	}
	_, err := buffer.WriteString(value)
	return err
}

func readString(reader io.Reader) (string, error) {
	length, err := readUint32(reader)
	if err != nil {
		return "", err
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", err
	}
	return string(data), nil
}

func readUint32(reader io.Reader) (uint32, error) {
	var value uint32
	err := binary.Read(reader, binary.BigEndian, &value)
	return value, err
}

func validateVideoShape(video *Video) error {
	if video == nil {
		return errors.New("video is nil")
	}
	if video.Version != formatVersion {
		return fmt.Errorf("unsupported transparent video version %d", video.Version)
	}
	if video.Width <= 0 || video.Height <= 0 {
		return errors.New("video dimensions must be positive")
	}
	if !validMode(video.Mode) {
		return fmt.Errorf("unsupported alpha mode %q", video.Mode)
	}
	if len(video.Frames) != 3 {
		return errors.New("video must contain source, black-background and white-background frames")
	}

	expectedPurposes := []string{PurposeSource, PurposeBlack, PurposeWhite}
	expectedPixels := video.Width * video.Height * 4
	for index, purpose := range expectedPurposes {
		frame := video.Frames[index]
		if frame.Purpose != purpose {
			return fmt.Errorf("frame %d has purpose %q, want %q", index, frame.Purpose, purpose)
		}
		if frame.Width != video.Width || frame.Height != video.Height {
			return fmt.Errorf("frame %q has invalid dimensions", purpose)
		}
		if len(frame.Pix) != expectedPixels {
			return fmt.Errorf("frame %q has %d pixel bytes, want %d", purpose, len(frame.Pix), expectedPixels)
		}
	}
	return nil
}
