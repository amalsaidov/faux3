package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrDataDirLocked reports that another faux3 process holds the data dir.
var ErrDataDirLocked = errors.New("store: data dir is locked by another faux3 process")

// lockDataDir takes an exclusive, non-blocking flock on .faux3/lock. The
// kernel releases it when the process exits, even on SIGKILL.
func lockDataDir(dir string) (*os.File, error) {
	path := filepath.Join(dir, ".faux3", "lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, wrapWritable(dir, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("faux3: %s is in use by another faux3 process; run only one faux3 per data dir: %w", dir, ErrDataDirLocked)
		}
		return nil, fmt.Errorf("faux3: lock %s: %w", path, err)
	}
	return f, nil
}

// Close releases the data dir lock. The store must not be used afterwards.
func (s *FS) Close() error {
	if s.lock == nil {
		return nil
	}
	err := s.lock.Close()
	s.lock = nil
	return err
}
