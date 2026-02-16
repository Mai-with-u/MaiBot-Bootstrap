//go:build !windows

package process

import (
	"os"
	"testing"
	"time"
)

func TestIsAliveCurrentProcess(t *testing.T) {
	if !IsAlive(os.Getpid()) {
		t.Fatalf("expected current process to be alive")
	}
}

func TestStopInvalidPID(t *testing.T) {
	if err := Stop(-1, time.Second); err != nil {
		t.Fatalf("Stop invalid pid error: %v", err)
	}
}
