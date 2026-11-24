package repack

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

type transaction struct {
	output   string
	dir      string
	temp     string
	backup   string
	hadFile  bool
	tempFile *os.File
}

// beginTransaction moves any existing output aside as a backup. The move is
// atomic because both names live in the same directory, and no new content
// has been produced yet, so rollback at this point is a single rename.
func beginTransaction(output string) (*transaction, error) {
	dir := filepath.Dir(output)
	info, err := os.Stat(dir)
	switch {
	case err == nil:
		if !info.IsDir() {
			return nil, fmt.Errorf("output parent %q is not a directory", dir)
		}
	case os.IsNotExist(err):
		return nil, fmt.Errorf("output directory %q does not exist", dir)
	default:
		return nil, fmt.Errorf("stat output directory: %w", err)
	}

	tx := &transaction{output: output, dir: dir}
	if _, err := os.Stat(output); err == nil {
		backup, err := os.CreateTemp(dir, filepath.Base(output)+".backup-*")
		if err != nil {
			return nil, fmt.Errorf("create backup slot: %w", err)
		}
		backupName := backup.Name()
		if err := backup.Close(); err != nil {
			os.Remove(backupName)
			return nil, fmt.Errorf("close backup slot: %w", err)
		}
		if err := os.Rename(output, backupName); err != nil {
			os.Remove(backupName)
			return nil, fmt.Errorf("back up existing output: %w", err)
		}
		tx.backup = backupName
		tx.hadFile = true
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat output: %w", err)
	}

	temp, err := os.CreateTemp(dir, filepath.Base(output)+".tmp-*")
	if err != nil {
		if tx.hadFile {
			_ = os.Rename(tx.backup, output)
		}
		return nil, fmt.Errorf("create temporary output: %w", err)
	}
	tx.temp = temp.Name()
	tx.tempFile = temp
	return tx, nil
}

func (tx *transaction) commit(encoded []byte, opts Options) error {
	if _, err := tx.tempFile.Write(encoded); err != nil {
		return fmt.Errorf("write temporary output: %w", err)
	}
	if opts.FailAt == FailAfterWrite {
		return fmt.Errorf("injected failure at %s", FailAfterWrite)
	}
	if err := tx.tempFile.Sync(); err != nil {
		return fmt.Errorf("flush temporary output: %w", err)
	}
	if opts.CorruptTemp {
		if _, err := tx.tempFile.WriteAt([]byte{0xff ^ encoded[0]}, 0); err != nil {
			return fmt.Errorf("fault injection write: %w", err)
		}
	}
	if err := tx.verify(encoded); err != nil {
		return err
	}
	if opts.FailAt == FailVerifyOutput {
		return fmt.Errorf("%w: injected at %s", ErrCorrupt, FailVerifyOutput)
	}
	if opts.FailAt == FailBeforeRename {
		return fmt.Errorf("injected failure at %s", FailBeforeRename)
	}
	if err := tx.tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary output: %w", err)
	}
	tx.tempFile = nil
	if err := os.Rename(tx.temp, tx.output); err != nil {
		return fmt.Errorf("install new output: %w", err)
	}
	tx.temp = ""
	if err := syncDir(tx.dir); err != nil {
		return fmt.Errorf("sync output directory: %w", err)
	}
	if opts.FailAt == FailAfterRename {
		return fmt.Errorf("injected failure at %s", FailAfterRename)
	}
	if tx.hadFile {
		if err := os.Remove(tx.backup); err != nil {
			return fmt.Errorf("remove old output backup: %w", err)
		}
		tx.backup = ""
	}
	return nil
}

// verify re-reads the temporary file from scratch and checks every checksum
// and byte against the data that was intended to be written.
func (tx *transaction) verify(expected []byte) error {
	got, err := os.ReadFile(tx.temp)
	if err != nil {
		return fmt.Errorf("re-read temporary output: %w", err)
	}
	if !bytes.Equal(got, expected) {
		return fmt.Errorf("%w: temporary output bytes differ from planned content", ErrCorrupt)
	}
	boxes, err := ReadPack(tx.temp)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	roundTrip, err := EncodePack(boxes)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if !bytes.Equal(roundTrip, expected) {
		return fmt.Errorf("%w: round-trip content mismatch", ErrCorrupt)
	}
	return nil
}

// rollback restores the output to exactly what the caller had before Repack.
func (tx *transaction) rollback() error {
	if tx.tempFile != nil {
		_ = tx.tempFile.Close()
	}
	var firstErr error
	if tx.temp != "" {
		if err := os.Remove(tx.temp); err != nil && !os.IsNotExist(err) {
			firstErr = err
		}
	}
	if tx.hadFile {
		if err := os.Rename(tx.backup, tx.output); err != nil && !os.IsNotExist(err) {
			if firstErr == nil {
				firstErr = err
			}
		}
	} else if _, err := os.Stat(tx.output); err == nil {
		if err := os.Remove(tx.output); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("rollback: %w", firstErr)
	}
	return syncDir(tx.dir)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
