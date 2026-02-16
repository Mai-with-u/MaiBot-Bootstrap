package logging

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestLoggerWritesModuleLineToFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "installer.log")
	root, err := NewRoot(Options{FilePath: logPath, MaxSizeMB: 5, RetentionDays: 7, MaxBackupFiles: 5})
	if err != nil {
		t.Fatalf("NewRoot error: %v", err)
	}

	logger := root.Module("instance")
	logger.Infof("hello %s", "world")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file error: %v", err)
	}
	line := strings.TrimSpace(string(data))
	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} \| INFO \| instance \| hello world$`)
	if !re.MatchString(line) {
		t.Fatalf("line format mismatch: %q", line)
	}
}

func TestLoggerRotationAndCleanup(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "installer.log")
	root, err := NewRoot(Options{FilePath: logPath, MaxSizeMB: 1, RetentionDays: 30, MaxBackupFiles: 1})
	if err != nil {
		t.Fatalf("NewRoot error: %v", err)
	}

	logger := root.Module("rotate")
	big := strings.Repeat("x", 1100000)
	for i := 0; i < 4; i++ {
		logger.Infof(big)
	}

	files, err := filepath.Glob(logPath + ".*")
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(files) > 1 {
		t.Fatalf("backup files = %d, want <= 1", len(files))
	}
}
