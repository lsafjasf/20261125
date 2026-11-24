package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Archive is an open file paired with its layered manifest.
type Archive struct {
	Path     string
	Reader   io.ReaderAt
	Size     int64
	Manifest *Manifest
	file     *os.File
}

// OpenArchive builds a manifest while retaining the file for byte-level diff.
func OpenArchive(path string, opts Options) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.IsDir() {
		f.Close()
		return nil, fmt.Errorf("%s is a directory", path)
	}
	manifest, err := BuildManifest(f, info.Size(), path, opts)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Archive{Path: path, Reader: f, Size: info.Size(), Manifest: manifest, file: f}, nil
}

// Close releases the underlying file.
func (a *Archive) Close() error {
	if a == nil || a.file == nil {
		return nil
	}
	return a.file.Close()
}

// BuildManifest hashes raw container blocks and all supported semantic layers.
func BuildManifest(r io.ReaderAt, size int64, name string, opts Options) (*Manifest, error) {
	opts = opts.withDefaults()
	whole, blocks, err := buildRawBlocks(r, size, opts.BlockSize)
	if err != nil {
		return nil, err
	}

	m := &Manifest{
		Schema:   SchemaVersion,
		FileName: filepath.Base(name),
		Format:   FormatOther,
		Size:     size,
		SHA256:   whole,
		Options:  opts,
		Blocks:   blocks,
	}

	format := detectFormat(r, size)
	m.Format = format
	switch format {
	case FormatWAV:
		if err := buildWAVManifest(m, r, size, opts); err != nil {
			m.Warnings = append(m.Warnings, err.Error())
		}
	case FormatMP4:
		if err := buildMP4Manifest(m, r, size, opts); err != nil {
			m.Warnings = append(m.Warnings, err.Error())
		}
	}
	return m, nil
}

func detectFormat(r io.ReaderAt, size int64) string {
	var head [16]byte
	n, err := r.ReadAt(head[:], 0)
	if err != nil && err != io.EOF {
		return FormatOther
	}
	if n >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WAVE" {
		return FormatWAV
	}
	if n >= 8 && string(head[4:8]) == "ftyp" {
		return FormatMP4
	}
	if n >= 8 && size >= 8 {
		switch string(head[4:8]) {
		case "moov", "mdat", "wide", "free", "skip":
			if _, err := readBoxHeader(r, 0, size); err == nil {
				return FormatMP4
			}
		}
	}
	return FormatOther
}
