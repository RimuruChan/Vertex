//go:build linux

package instance

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Lock holds the process-lifetime lease for one sandbox instance namespace.
type Lock struct {
	file *os.File
}

// Acquire takes a non-blocking exclusive lock at path.
func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sandbox instance lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open sandbox instance lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("sandbox instance is already active: %w", err)
	}
	return &Lock{file: file}, nil
}

// Close releases the instance lease.
func (lock *Lock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	err := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	closeErr := lock.file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
