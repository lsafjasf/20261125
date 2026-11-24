package mediaidx

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"
)

const (
	journalVersion         = 1
	journalHeaderSize      = 16
	recordHeaderSize       = 9
	recordTrailerSize      = 4
	maxRecordPayload       = 1 << 20
	recordTypeLeaf    byte = 1
	recordTypeCommit  byte = 2
)

var (
	journalMagic = [8]byte{'M', 'D', 'I', 'D', 'X', '0', '0', '1'}
	recordMagic  = [4]byte{'M', 'D', 'R', '1'}
	crcTable     = crc32.MakeTable(crc32.Castagnoli)
)

var errInvalidJournal = ErrCorruptIndex

func encodeJournalHeader() []byte {
	header := make([]byte, journalHeaderSize)
	copy(header[:8], journalMagic[:])
	binary.LittleEndian.PutUint16(header[8:10], journalVersion)
	binary.LittleEndian.PutUint32(header[12:16], crc32.Checksum(header[:12], crcTable))
	return header
}

func decodeJournalHeader(header []byte) error {
	if len(header) != journalHeaderSize {
		return fmt.Errorf("%w: short header", errInvalidJournal)
	}
	if string(header[:8]) != string(journalMagic[:]) {
		return fmt.Errorf("%w: bad magic", errInvalidJournal)
	}
	if version := binary.LittleEndian.Uint16(header[8:10]); version != journalVersion {
		return fmt.Errorf("%w: unsupported version %d", errInvalidJournal, version)
	}
	expected := binary.LittleEndian.Uint32(header[12:16])
	actual := crc32.Checksum(header[:12], crcTable)
	if expected != actual {
		return fmt.Errorf("%w: header checksum mismatch", errInvalidJournal)
	}
	return nil
}

func sealRecord(recordType byte, payload []byte) []byte {
	record := make([]byte, recordHeaderSize+len(payload)+recordTrailerSize)
	copy(record[:4], recordMagic[:])
	record[4] = recordType
	binary.LittleEndian.PutUint32(record[5:9], uint32(len(payload)))
	copy(record[9:], payload)
	checksum := crc32.Checksum(record[4:recordHeaderSize+len(payload)], crcTable)
	binary.LittleEndian.PutUint32(record[recordHeaderSize+len(payload):], checksum)
	return record
}

func validRecordMagic(header []byte) bool {
	return len(header) >= 4 &&
		header[0] == recordMagic[0] &&
		header[1] == recordMagic[1] &&
		header[2] == recordMagic[2] &&
		header[3] == recordMagic[3]
}

func encodeLeafPayload(leaf Leaf) []byte {
	payload := make([]byte, 0, 73)
	payload = appendUint64(payload, leaf.Seq)
	payload = append(payload, byte(leaf.Locator.Kind))
	payload = appendUint64(payload, leaf.Locator.Start)
	payload = appendUint64(payload, leaf.Locator.Count)
	payload = appendUint64(payload, uint64(leaf.DataOffset))
	payload = appendUint64(payload, uint64(leaf.DataSize))
	payload = append(payload, leaf.Digest[:]...)
	return payload
}

func decodeLeafPayload(payload []byte) (Leaf, error) {
	if len(payload) != 73 {
		return Leaf{}, fmt.Errorf("%w: leaf payload size %d", errInvalidJournal, len(payload))
	}
	kind := Kind(payload[8])
	if !kind.valid() {
		return Leaf{}, fmt.Errorf("%w: unknown unit kind %d", errInvalidJournal, kind)
	}
	dataOffset := binary.LittleEndian.Uint64(payload[25:33])
	dataSize := binary.LittleEndian.Uint64(payload[33:41])
	if dataOffset > math.MaxInt64 || dataSize == 0 || dataSize > math.MaxInt64 {
		return Leaf{}, fmt.Errorf("%w: invalid data range", errInvalidJournal)
	}
	leaf := Leaf{
		Seq: binary.LittleEndian.Uint64(payload[0:8]),
		Locator: Locator{
			Kind:  kind,
			Start: binary.LittleEndian.Uint64(payload[9:17]),
			Count: binary.LittleEndian.Uint64(payload[17:25]),
		},
		DataOffset: int64(dataOffset),
		DataSize:   int64(dataSize),
	}
	copy(leaf.Digest[:], payload[41:73])
	if leaf.Locator.Count == 0 {
		return Leaf{}, fmt.Errorf("%w: zero unit count", errInvalidJournal)
	}
	if leaf.Locator.Kind == Frame && leaf.Locator.Count != 1 {
		return Leaf{}, fmt.Errorf("%w: a frame leaf must cover exactly one frame", errInvalidJournal)
	}
	return leaf, nil
}

type commitPayload struct {
	Number    uint64
	LeafCount uint64
	DataSize  int64
	Previous  Hash
	Root      Hash
	Peaks     []Hash
}

func encodeCommitPayload(commit commitPayload) ([]byte, error) {
	if commit.DataSize < 0 || len(commit.Peaks) > 64 {
		return nil, fmt.Errorf("invalid commit payload")
	}
	payload := make([]byte, 0, 92+len(commit.Peaks)*sha256.Size)
	payload = appendUint64(payload, commit.Number)
	payload = appendUint64(payload, commit.LeafCount)
	payload = appendUint64(payload, uint64(commit.DataSize))
	payload = append(payload, commit.Previous[:]...)
	payload = append(payload, commit.Root[:]...)
	var count [4]byte
	binary.LittleEndian.PutUint32(count[:], uint32(len(commit.Peaks)))
	payload = append(payload, count[:]...)
	for _, peak := range commit.Peaks {
		payload = append(payload, peak[:]...)
	}
	return payload, nil
}

func decodeCommitPayload(payload []byte) (commitPayload, error) {
	if len(payload) < 92 {
		return commitPayload{}, fmt.Errorf("%w: short commit payload", errInvalidJournal)
	}
	count := binary.LittleEndian.Uint32(payload[88:92])
	if count > 64 || len(payload) != 92+int(count)*sha256.Size {
		return commitPayload{}, fmt.Errorf("%w: invalid peak count", errInvalidJournal)
	}
	dataSize := binary.LittleEndian.Uint64(payload[16:24])
	if dataSize > math.MaxInt64 {
		return commitPayload{}, fmt.Errorf("%w: data size overflow", errInvalidJournal)
	}
	commit := commitPayload{
		Number:    binary.LittleEndian.Uint64(payload[0:8]),
		LeafCount: binary.LittleEndian.Uint64(payload[8:16]),
		DataSize:  int64(dataSize),
		Peaks:     make([]Hash, count),
	}
	copy(commit.Previous[:], payload[24:56])
	copy(commit.Root[:], payload[56:88])
	for i := range commit.Peaks {
		copy(commit.Peaks[i][:], payload[92+i*sha256.Size:])
	}
	return commit, nil
}

func recordID(recordWithoutCRC []byte) Hash {
	return sha256.Sum256(recordWithoutCRC)
}
