//go:build windows

package process

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	text := strings.TrimSpace(strings.ToLower(string(out)))
	if text == "" || strings.Contains(text, "no tasks are running") {
		return false
	}
	return strings.Contains(text, fmt.Sprintf(","+"%d", pid)) || strings.Contains(text, fmt.Sprintf("\"%d\"", pid))
}

func Stop(pid int, grace time.Duration) error {
	if pid <= 0 || !IsAlive(pid) {
		return nil
	}
	_ = grace
	if err := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T").Run(); err == nil {
		return nil
	}
	if err := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T", "/F").Run(); err != nil {
		return fmt.Errorf("failed to stop pid %d: %w", pid, err)
	}
	return nil
}
