package media

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"rec2020-to-rec709-shoulder/internal/pipeline"
)

func ReadPFM(path string) (*pipeline.FloatImage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)

	magic, err := readPFMLine(reader)
	if err != nil {
		return nil, err
	}
	color := strings.TrimSpace(magic) == "PF"
	if !color && strings.TrimSpace(magic) != "Pf" {
		return nil, fmt.Errorf("unsupported PFM magic %q", magic)
	}
	dims, err := readPFMLine(reader)
	if err != nil {
		return nil, err
	}
	parts := strings.Fields(dims)
	if len(parts) != 2 {
		return nil, fmt.Errorf("bad PFM dimensions %q", dims)
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, err
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, err
	}
	endianLine, err := readPFMLine(reader)
	if err != nil {
		return nil, err
	}
	scale, err := strconv.ParseFloat(strings.TrimSpace(endianLine), 64)
	if err != nil {
		return nil, err
	}
	var byteOrder binary.ByteOrder = binary.LittleEndian
	if scale >= 0 {
		byteOrder = binary.BigEndian
	}
	channels := 3
	if !color {
		channels = 1
	}

	im := pipeline.NewFloatImage(width, height)
	rowFloats := width * channels
	row := make([]float32, rowFloats)
	for storageY := height - 1; storageY >= 0; storageY-- {
		if err := binary.Read(reader, byteOrder, row); err != nil {
			return nil, err
		}
		for x := 0; x < width; x++ {
			target := (storageY*width + x) * 3
			if color {
				im.Pix[target] = float64(row[x*channels]) * math.Abs(scale)
				im.Pix[target+1] = float64(row[x*channels+1]) * math.Abs(scale)
				im.Pix[target+2] = float64(row[x*channels+2]) * math.Abs(scale)
			} else {
				v := float64(row[x]) * math.Abs(scale)
				im.Pix[target], im.Pix[target+1], im.Pix[target+2] = v, v, v
			}
		}
	}
	return im, nil
}

func WritePFM(path string, im *pipeline.FloatImage) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	fmt.Fprintf(writer, "PF\n%d %d\n-1.0\n", im.Width, im.Height)
	row := make([]float32, im.Width*3)
	for y := im.Height - 1; y >= 0; y-- {
		for x := 0; x < im.Width; x++ {
			i := (y*im.Width + x) * 3
			row[x*3] = float32(im.Pix[i])
			row[x*3+1] = float32(im.Pix[i+1])
			row[x*3+2] = float32(im.Pix[i+2])
		}
		if err := binary.Write(writer, binary.LittleEndian, row); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func readPFMLine(r *bufio.Reader) (string, error) {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line, nil
		}
	}
}
