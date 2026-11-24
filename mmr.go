package mediaidx

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

type peak struct {
	hash   Hash
	height uint8
}

type mmrState struct {
	count    uint64
	peaks    []peak
	appended uint64
}

func (m *mmrState) clone() mmrState {
	return mmrState{
		count:    m.count,
		peaks:    append([]peak(nil), m.peaks...),
		appended: m.appended,
	}
}

func (m *mmrState) appendLeaf(dataDigest Hash) {
	next := peak{
		hash:   hashLeaf(dataDigest),
		height: 0,
	}
	for len(m.peaks) > 0 && m.peaks[len(m.peaks)-1].height == next.height {
		left := m.peaks[len(m.peaks)-1]
		m.peaks = m.peaks[:len(m.peaks)-1]
		next = peak{
			hash:   hashNode(left.hash, next.hash),
			height: left.height + 1,
		}
	}
	m.peaks = append(m.peaks, next)
	m.count++
	m.appended++
}

func (m mmrState) root() Hash {
	if len(m.peaks) == 0 {
		return sha256.Sum256([]byte("mediaidx:empty"))
	}
	result := m.peaks[len(m.peaks)-1].hash
	for i := len(m.peaks) - 2; i >= 0; i-- {
		result = hashNode(m.peaks[i].hash, result)
	}
	return result
}

func hashLeaf(dataDigest Hash) Hash {
	h := sha256.New()
	h.Write([]byte{0})
	h.Write(dataDigest[:])
	var result Hash
	copy(result[:], h.Sum(nil))
	return result
}

func hashNode(left, right Hash) Hash {
	h := sha256.New()
	h.Write([]byte{1})
	h.Write(left[:])
	h.Write(right[:])
	var result Hash
	copy(result[:], h.Sum(nil))
	return result
}

func peakHeights(count uint64) []uint8 {
	if count == 0 {
		return nil
	}
	heights := make([]uint8, 0, 64)
	for bit := 63; bit >= 0; bit-- {
		if count&(uint64(1)<<uint(bit)) != 0 {
			heights = append(heights, uint8(bit))
		}
	}
	return heights
}

func mmrFromPeaks(count uint64, hashes []Hash) (mmrState, error) {
	heights := peakHeights(count)
	if len(heights) != len(hashes) {
		return mmrState{}, fmt.Errorf("peak count %d does not match leaf count %d", len(hashes), count)
	}
	peaks := make([]peak, len(hashes))
	for i := range hashes {
		peaks[i] = peak{hash: hashes[i], height: heights[i]}
	}
	return mmrState{count: count, peaks: peaks}, nil
}

func (m mmrState) peakHashes() []Hash {
	hashes := make([]Hash, len(m.peaks))
	for i := range m.peaks {
		hashes[i] = m.peaks[i].hash
	}
	return hashes
}

func appendUint64(dst []byte, value uint64) []byte {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], value)
	return append(dst, buf[:]...)
}
