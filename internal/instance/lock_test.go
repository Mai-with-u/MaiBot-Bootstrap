package instance

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireAndReleaseLock(t *testing.T) {
	dir := t.TempDir()
	lock, err := AcquireLock(dir, "demo", time.Second)
	if err != nil {
		t.Fatalf("AcquireLock error: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release error: %v", err)
	}
}

func TestAcquireLockTimeout(t *testing.T) {
	dir := t.TempDir()
	first, err := AcquireLock(dir, "demo", time.Second)
	if err != nil {
		t.Fatalf("AcquireLock first error: %v", err)
	}
	defer func() { _ = first.Release() }()

	_, err = AcquireLock(dir, "demo", 200*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}

	if _, statErr := filepath.Glob(filepath.Join(dir, "*.lock")); statErr != nil {
		t.Fatalf("glob lock error: %v", statErr)
	}
}
