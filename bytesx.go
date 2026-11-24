package main

import "encoding/binary"

func beUint16(b []byte, off int) uint16 {
	return binary.BigEndian.Uint16(b[off : off+2])
}

func beUint32(b []byte, off int) uint32 {
	return binary.BigEndian.Uint32(b[off : off+4])
}

func beUint64(b []byte, off int) uint64 {
	return binary.BigEndian.Uint64(b[off : off+8])
}

func leUint16(b []byte, off int) uint16 {
	return binary.LittleEndian.Uint16(b[off : off+2])
}

func leUint32(b []byte, off int) uint32 {
	return binary.LittleEndian.Uint32(b[off : off+4])
}
