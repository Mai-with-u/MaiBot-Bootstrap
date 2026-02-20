package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateCreatesMaibotConf(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("LoadOrCreate error: %v", err)
	}
	if cfg.Version != schemaVersion {
		t.Fatalf("version = %d, want %d", cfg.Version, schemaVersion)
	}
	if cfg.Installer.ReleaseChannel != "stable" {
		t.Fatalf("release channel = %q, want stable", cfg.Installer.ReleaseChannel)
	}

	path := filepath.Join(home, ".maibot", "maibot.conf")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected maibot.conf, stat error: %v", err)
	}
}

func TestLoadOrCreateMigratesLegacyConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	base := filepath.Join(home, ".maibot")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	legacy := []byte("{\n  \"version\": 1,\n  \"installer\": {\n    \"repo\": \"x/y\",\n    \"data_home\": \"" + base + "\",\n    \"instance_tick_interval\": \"10s\",\n    \"lock_timeout_seconds\": 4\n  },\n  \"logging\": {\n    \"file_path\": \"" + filepath.Join(base, "logs", "installer.log") + "\",\n    \"max_size_mb\": 5,\n    \"retention_days\": 3,\n    \"max_backup_files\": 5\n  }\n}\n")
	legacyPath := filepath.Join(base, "config.json")
	if err := os.WriteFile(legacyPath, legacy, 0o644); err != nil {
		t.Fatalf("write legacy config error: %v", err)
	}

	cfg, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("LoadOrCreate error: %v", err)
	}
	if cfg.Version != schemaVersion {
		t.Fatalf("version = %d, want %d", cfg.Version, schemaVersion)
	}
	if cfg.Installer.ReleaseChannel != "stable" {
		t.Fatalf("release channel = %q, want stable", cfg.Installer.ReleaseChannel)
	}

	if _, err := os.Stat(filepath.Join(base, "maibot.conf")); err != nil {
		t.Fatalf("expected maibot.conf, stat error: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(base, "config.backup.*.json"))
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected migration backup file")
	}
}

func TestApplyDefaultsAddsSharedDownloadMirrorsToGit(t *testing.T) {
	base := t.TempDir()
	cfg := Config{
		Installer: Installer{DataHome: base},
		Logging:   Logging{FilePath: filepath.Join(base, "logs", "installer.log")},
		Mirrors:   Mirrors{URLs: []string{"https://ghfast.top", "https://gh-proxy.com"}},
		Git:       Git{Mirrors: []GitMirror{}},
	}
	out := applyDefaults(cfg, base)
	if len(out.Git.Mirrors) < 2 {
		t.Fatalf("git mirrors len=%d, want >=2", len(out.Git.Mirrors))
	}
	if out.Git.Mirrors[0].BaseURL != "https://ghfast.top" {
		t.Fatalf("first shared mirror=%q", out.Git.Mirrors[0].BaseURL)
	}
}
