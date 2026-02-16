//go:build !windows

package process

import (
	"errors"
	"fmt"
	"syscall"
	"time"
)

func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil
}

func Stop(pid int, grace time.Duration) error {
	if pid <= 0 {
		return nil
	}
	if !IsAlive(pid) {
		return nil
	}

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		if !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("failed SIGTERM pid %d: %w", pid, err)
		}
		return nil
	}

	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !IsAlive(pid) {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}

	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		if !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("failed SIGKILL pid %d: %w", pid, err)
		}
	}
	return nil
}
