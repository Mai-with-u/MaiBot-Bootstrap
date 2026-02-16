package instance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Lock struct {
	path string
}

func AcquireLock(dir, name string, timeout time.Duration) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(dir, sanitizeLockName(name)+".lock")
	deadline := time.Now().Add(timeout)

	for {
		f, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "pid=%d\ncreated_unix=%d\n", os.Getpid(), time.Now().Unix())
			_ = f.Close()
			return &Lock{path: lockPath}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}

		stale, staleErr := isStale(lockPath, 10*time.Minute)
		if staleErr == nil && stale {
			_ = os.Remove(lockPath)
			continue
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting lock %s", lockPath)
		}
		time.Sleep(120 * time.Millisecond)
	}
}

func (l *Lock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	err := os.Remove(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func sanitizeLockName(name string) string {
	if name == "" {
		return "default"
	}
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r)
		case r >= '0' && r <= '9':
			out = append(out, r)
		case r == '-' || r == '_' || r == '.':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "default"
	}
	return string(out)
}

func isStale(path string, age time.Duration) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return time.Since(info.ModTime()) > age, nil
}
