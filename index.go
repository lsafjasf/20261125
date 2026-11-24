package mediaidx

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Index owns an append-only journal and a contiguous media data file. The
// journal is a Merkle Mountain Range frontier plus leaf descriptors; no
// database is used.
type Index struct {
	mu sync.Mutex

	indexPath string
	dataPath  string
	indexFile *os.File
	dataFile  *os.File
	noSync    bool

	leaves             []Leaf
	mmr                mmrState
	info               CommitInfo
	committedIndexSize int64
	committedDataSize  int64
	nextByKind         map[Kind]uint64
	hasGap             bool
	damaged            bool
	closed             bool
	tx                 *Tx
}

// Open opens an index at indexPath and keeps media bytes in
// indexPath+".data".
func Open(indexPath string) (*Index, error) {
	return OpenWithOptions(indexPath, Options{})
}

// OpenWithOptions opens or creates an index. Existing uncommitted tails and
// media bytes beyond the last commit are discarded during recovery.
func OpenWithOptions(indexPath string, options Options) (*Index, error) {
	if indexPath == "" {
		return nil, fmt.Errorf("mediaidx: empty index path")
	}
	dataPath := options.DataPath
	if dataPath == "" {
		dataPath = indexPath + ".data"
	}
	if dataPath == indexPath {
		return nil, fmt.Errorf("mediaidx: index and data paths must differ")
	}
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		return nil, fmt.Errorf("create index directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	indexFile, err := os.OpenFile(indexPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	dataFile, err := os.OpenFile(dataPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		indexFile.Close()
		return nil, fmt.Errorf("open media data: %w", err)
	}

	index := &Index{
		indexPath:  indexPath,
		dataPath:   dataPath,
		indexFile:  indexFile,
		dataFile:   dataFile,
		noSync:     options.NoSync,
		nextByKind: make(map[Kind]uint64, 2),
	}
	if err := index.initialize(); err != nil {
		indexFile.Close()
		dataFile.Close()
		return nil, err
	}
	return index, nil
}

func (index *Index) initialize() error {
	snapshot, err := loadJournal(index.indexFile)
	if err != nil {
		info, statErr := index.indexFile.Stat()
		if statErr != nil || info.Size() > journalHeaderSize {
			return fmt.Errorf("load index: %w", err)
		}
		snapshot = journalSnapshot{tornTail: true}
	}

	if !snapshot.hasCommit {
		if err := index.indexFile.Truncate(0); err != nil {
			return fmt.Errorf("truncate empty index: %w", err)
		}
		header := encodeJournalHeader()
		if _, err := index.indexFile.WriteAt(header, 0); err != nil {
			return fmt.Errorf("write index header: %w", err)
		}
		snapshot.committedEnd = journalHeaderSize
	} else if snapshot.tornTail {
		if err := index.indexFile.Truncate(snapshot.committedEnd); err != nil {
			return fmt.Errorf("discard uncommitted journal tail: %w", err)
		}
	}

	index.leaves = snapshot.leaves
	index.mmr = snapshot.mmr
	index.info = snapshot.info
	index.committedIndexSize = snapshot.committedEnd
	index.committedDataSize = snapshot.info.DataSize
	if !snapshot.hasCommit {
		index.info.Root = index.mmr.root()
	}

	dataInfo, err := index.dataFile.Stat()
	if err != nil {
		return fmt.Errorf("stat media data: %w", err)
	}
	switch {
	case dataInfo.Size() > index.committedDataSize:
		if err := index.dataFile.Truncate(index.committedDataSize); err != nil {
			return fmt.Errorf("discard uncommitted media tail: %w", err)
		}
	case dataInfo.Size() < index.committedDataSize:
		index.damaged = true
	}

	if !index.noSync {
		if err := index.indexFile.Sync(); err != nil {
			return fmt.Errorf("sync recovered index: %w", err)
		}
		if err := index.dataFile.Sync(); err != nil {
			return fmt.Errorf("sync recovered media: %w", err)
		}
	}
	index.rebuildNextByKind()
	_, err = index.indexFile.Seek(index.committedIndexSize, io.SeekStart)
	if err != nil {
		return fmt.Errorf("seek index: %w", err)
	}
	_, err = index.dataFile.Seek(index.committedDataSize, io.SeekStart)
	if err != nil {
		return fmt.Errorf("seek media data: %w", err)
	}
	return nil
}

func (index *Index) rebuildNextByKind() {
	index.nextByKind = make(map[Kind]uint64, 2)
	index.hasGap = false
	for _, leaf := range index.leaves {
		previous := index.nextByKind[leaf.Locator.Kind]
		if leaf.Locator.Start > previous {
			index.hasGap = true
		}
		next, ok := leaf.Locator.End()
		if !ok {
			continue
		}
		if next > previous {
			index.nextByKind[leaf.Locator.Kind] = next
		}
	}
}

// Begin starts an atomic append transaction. No leaf becomes visible until
// Commit completes.
func (index *Index) Begin() (*Tx, error) {
	index.mu.Lock()
	defer index.mu.Unlock()
	if index.closed {
		return nil, ErrClosed
	}
	if index.damaged {
		return nil, ErrDamaged
	}
	if index.tx != nil {
		return nil, ErrTransactionActive
	}
	dataInfo, err := index.dataFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat media before append: %w", err)
	}
	switch {
	case dataInfo.Size() < index.committedDataSize:
		index.damaged = true
		return nil, ErrDamaged
	case dataInfo.Size() > index.committedDataSize:
		if err := index.dataFile.Truncate(index.committedDataSize); err != nil {
			return nil, fmt.Errorf("discard uncommitted media suffix: %w", err)
		}
	}
	next := make(map[Kind]uint64, len(index.nextByKind))
	for kind, value := range index.nextByKind {
		next[kind] = value
	}
	tx := &Tx{
		index:      index,
		dataSize:   index.committedDataSize,
		mmr:        index.mmr.clone(),
		nextByKind: next,
		hasGap:     index.hasGap,
	}
	index.tx = tx
	return tx, nil
}

// CommitInfo returns the durable frontier without reading media bytes.
func (index *Index) CommitInfo() CommitInfo {
	index.mu.Lock()
	defer index.mu.Unlock()
	return index.info
}

// Leaves returns a copy of all committed leaf descriptors.
func (index *Index) Leaves() []Leaf {
	index.mu.Lock()
	defer index.mu.Unlock()
	return append([]Leaf(nil), index.leaves...)
}

// Paths returns the index and media file names.
func (index *Index) Paths() (string, string) {
	index.mu.Lock()
	defer index.mu.Unlock()
	return index.indexPath, index.dataPath
}

// Close aborts an active transaction and closes both files.
func (index *Index) Close() error {
	index.mu.Lock()
	defer index.mu.Unlock()
	if index.closed {
		return nil
	}
	if index.tx != nil {
		_ = index.abortLocked(index.tx)
	}
	index.closed = true
	indexError := index.indexFile.Close()
	dataError := index.dataFile.Close()
	if indexError != nil {
		return indexError
	}
	return dataError
}

// LocateOffset maps a byte offset to the committed leaf containing it.
func (index *Index) LocateOffset(offset int64) (Leaf, bool) {
	index.mu.Lock()
	defer index.mu.Unlock()
	if offset < 0 {
		return Leaf{}, false
	}
	position := sort.Search(len(index.leaves), func(i int) bool {
		return index.leaves[i].end() > offset
	})
	if position == len(index.leaves) || index.leaves[position].DataOffset > offset {
		return Leaf{}, false
	}
	return index.leaves[position], true
}

// LocateRange returns all committed leaves overlapping [offset, offset+size).
func (index *Index) LocateRange(offset, size int64) ([]Leaf, error) {
	if offset < 0 || size <= 0 {
		return nil, fmt.Errorf("%w: byte range offset=%d size=%d", ErrInvalidData, offset, size)
	}
	end := offset + size
	if end < offset {
		return nil, fmt.Errorf("%w: byte range overflow", ErrInvalidData)
	}
	index.mu.Lock()
	defer index.mu.Unlock()
	first := sort.Search(len(index.leaves), func(i int) bool {
		return index.leaves[i].end() > offset
	})
	var result []Leaf
	for i := first; i < len(index.leaves) && index.leaves[i].DataOffset < end; i++ {
		result = append(result, index.leaves[i])
	}
	return result, nil
}

// LocateUnit returns the leaf containing a frame or sample coordinate.
func (index *Index) LocateUnit(kind Kind, unit uint64) (Leaf, bool) {
	if !kind.valid() {
		return Leaf{}, false
	}
	index.mu.Lock()
	defer index.mu.Unlock()
	for _, leaf := range index.leaves {
		if leaf.Locator.Kind != kind {
			continue
		}
		end, ok := leaf.Locator.End()
		if ok && unit >= leaf.Locator.Start && unit < end {
			return leaf, true
		}
	}
	return Leaf{}, false
}
