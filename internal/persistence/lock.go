package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// fileLock provides a cross-process advisory lock over the data directory.
// It prevents concurrent writers (and concurrent readers during recovery) from
// racing on the ledger file so that the monotonic sequence and hash chain can
// never be broken when multiple service instances share one data directory.
type fileLock struct {
	file *os.File
}

// newFileLock opens (creating if needed) the lock file and blocks until it
// acquires the requested lock mode.
func newFileLock(directory string, mode int) (*fileLock, error) {
	lockPath := filepath.Join(directory, ".lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开锁文件: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), mode); err != nil {
		file.Close()
		return nil, fmt.Errorf("获取文件锁: %w", err)
	}
	return &fileLock{file: file}, nil
}

func (l *fileLock) release() {
	if l == nil || l.file == nil {
		return
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
}
