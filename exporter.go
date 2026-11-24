package transparentvideo

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	formatMagic   = "TVID"
	formatVersion = uint16(1)
)

const (
	PurposeSource = "source"
	PurposeBlack  = "black-background"
	PurposeWhite  = "white-background"
)

type AlphaMode string

const (
	AlphaStraight      AlphaMode = "straight"
	AlphaPremultiplied AlphaMode = "premultiplied"
)

type Color struct {
	R, G, B, A uint8
}

var (
	Black = Color{R: 0, G: 0, B: 0, A: 255}
	White = Color{R: 255, G: 255, B: 255, A: 255}
)

type Frame struct {
	Purpose string
	Width   int
	Height  int
	Pix     []byte
}

type Config struct {
	Width  int
	Height int
	Mode   AlphaMode
}

type Video struct {
	Version uint16
	Width   int
	Height  int
	Mode    AlphaMode
	Frames  []Frame
}

type Exporter struct {
	config Config
	source Frame
	verify func(*Video) error
}

func New(config Config) (*Exporter, error) {
	if config.Width <= 0 || config.Height <= 0 {
		return nil, errors.New("width and height must be positive")
	}
	if config.Mode == "" {
		config.Mode = AlphaStraight
	}
	if !validMode(config.Mode) {
		return nil, fmt.Errorf("unsupported alpha mode %q", config.Mode)
	}

	exporter := &Exporter{
		config: config,
		verify: VerifyVideo,
	}
	exporter.source = newTestSource(config.Width, config.Height, config.Mode)
	return exporter, nil
}

func (e *Exporter) Mode() AlphaMode {
	return e.config.Mode
}

func (e *Exporter) SourceFrame() Frame {
	return cloneFrame(e.source)
}

func (e *Exporter) SetMode(mode AlphaMode) error {
	if !validMode(mode) {
		return fmt.Errorf("unsupported alpha mode %q", mode)
	}
	if mode == e.config.Mode {
		return nil
	}

	converted := convertMode(e.source, e.config.Mode, mode)
	e.config.Mode = mode
	e.source = converted
	return nil
}

func (e *Exporter) RenderTestFrame(background Color) (Frame, error) {
	if background.A != 255 {
		return Frame{}, errors.New("test background must be opaque")
	}
	switch background {
	case Black:
		return renderComposite(e.source, e.config.Mode, background, PurposeBlack), nil
	case White:
		return renderComposite(e.source, e.config.Mode, background, PurposeWhite), nil
	default:
		return Frame{}, errors.New("test background must be Black or White")
	}
}

func (e *Exporter) BuildVideo() (*Video, error) {
	black, err := e.RenderTestFrame(Black)
	if err != nil {
		return nil, err
	}
	white, err := e.RenderTestFrame(White)
	if err != nil {
		return nil, err
	}

	return &Video{
		Version: formatVersion,
		Width:   e.config.Width,
		Height:  e.config.Height,
		Mode:    e.config.Mode,
		Frames: []Frame{
			cloneFrame(e.source),
			black,
			white,
		},
	}, nil
}

func (e *Exporter) Export(path string) error {
	video, err := e.BuildVideo()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	temp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		if !committed {
			temp.Close()
			os.Remove(tempPath)
		}
	}()

	if _, err := Encode(temp, video); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	writtenVideo, err := Decode(temp)
	if err != nil {
		return err
	}
	if err := e.verify(writtenVideo); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func validMode(mode AlphaMode) bool {
	return mode == AlphaStraight || mode == AlphaPremultiplied
}

func cloneFrame(frame Frame) Frame {
	pix := make([]byte, len(frame.Pix))
	copy(pix, frame.Pix)
	return Frame{
		Purpose: frame.Purpose,
		Width:   frame.Width,
		Height:  frame.Height,
		Pix:     pix,
	}
}

func newTestSource(width, height int, mode AlphaMode) Frame {
	frame := Frame{
		Purpose: PurposeSource,
		Width:   width,
		Height:  height,
		Pix:     make([]byte, width*height*4),
	}
	for index := 0; index < width*height; index++ {
		offset := index * 4
		frame.Pix[offset] = 255
		frame.Pix[offset+1] = 0
		frame.Pix[offset+2] = 0
		frame.Pix[offset+3] = 127
	}
	if mode == AlphaPremultiplied {
		return premultiply(frame)
	}
	return frame
}

func convertMode(frame Frame, from, to AlphaMode) Frame {
	if from == to {
		return cloneFrame(frame)
	}
	if to == AlphaPremultiplied {
		return premultiply(frame)
	}
	return straighten(frame)
}

func premultiply(frame Frame) Frame {
	result := cloneFrame(frame)
	for index := 0; index < len(result.Pix); index += 4 {
		alpha := result.Pix[index+3]
		result.Pix[index] = scale(alpha, result.Pix[index])
		result.Pix[index+1] = scale(alpha, result.Pix[index+1])
		result.Pix[index+2] = scale(alpha, result.Pix[index+2])
	}
	return result
}

func straighten(frame Frame) Frame {
	result := cloneFrame(frame)
	for index := 0; index < len(result.Pix); index += 4 {
		alpha := result.Pix[index+3]
		if alpha == 0 {
			result.Pix[index] = 0
			result.Pix[index+1] = 0
			result.Pix[index+2] = 0
			continue
		}
		result.Pix[index] = unscale(alpha, result.Pix[index])
		result.Pix[index+1] = unscale(alpha, result.Pix[index+1])
		result.Pix[index+2] = unscale(alpha, result.Pix[index+2])
	}
	return result
}

func scale(alpha, value uint8) uint8 {
	return uint8((int(alpha)*int(value) + 128) / 255)
}

func unscale(alpha, value uint8) uint8 {
	result := (int(value)*255 + int(alpha)/2) / int(alpha)
	return clampByte(result)
}

func renderComposite(source Frame, mode AlphaMode, background Color, purpose string) Frame {
	result := Frame{
		Purpose: purpose,
		Width:   source.Width,
		Height:  source.Height,
		Pix:     make([]byte, len(source.Pix)),
	}
	for index := 0; index < len(source.Pix); index += 4 {
		alpha := source.Pix[index+3]
		red, green, blue := sourceColor(source.Pix[index:])
		if mode == AlphaStraight {
			red = scale(alpha, red)
			green = scale(alpha, green)
			blue = scale(alpha, blue)
		}
		result.Pix[index] = compositePremultiplied(red, background.R, alpha)
		result.Pix[index+1] = compositePremultiplied(green, background.G, alpha)
		result.Pix[index+2] = compositePremultiplied(blue, background.B, alpha)
		result.Pix[index+3] = 255
	}
	return result
}

func sourceColor(pix []byte) (uint8, uint8, uint8) {
	return pix[0], pix[1], pix[2]
}

func compositePremultiplied(source, background, alpha uint8) uint8 {
	value := int(source) + (int(background)*(255-int(alpha))+127)/255
	return clampByte(value)
}

func clampByte(value int) uint8 {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(value)
}
