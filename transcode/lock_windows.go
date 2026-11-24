//go:build windows

package transcode

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	errorLockViolation      = syscall.Errno(33)
)

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	lockFileEx   = kernel32.NewProc("LockFileEx")
	unlockFileEx = kernel32.NewProc("UnlockFileEx")
)

type fileLock struct {
	file       *os.File
	overlapped syscall.Overlapped
}

func acquireFileLock(ctx context.Context, name string) (*fileLock, error) {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	lock := &fileLock{file: file}
	for {
		ok, _, callErr := lockFileEx.Call(
			file.Fd(),
			lockfileExclusiveLock|lockfileFailImmediately,
			0,
			1,
			0,
			uintptr(unsafe.Pointer(&lock.overlapped)),
		)
		if ok != 0 {
			return lock, nil
		}
		if callErr != errorLockViolation {
			file.Close()
			return nil, callErr
		}
		timer := time.NewTimer(40 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (lock *fileLock) release() {
	_, _, _ = unlockFileEx.Call(
		lock.file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&lock.overlapped)),
	)
	_ = lock.file.Close()
}
