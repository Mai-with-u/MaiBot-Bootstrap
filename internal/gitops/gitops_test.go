package gitops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"maibot/internal/config"
)

func TestBuildSourcesMirrorPriority(t *testing.T) {
	cfg := config.Git{
		Mirrors:     []config.GitMirror{{Name: "m1", BaseURL: "https://mirror.local", Enabled: true}},
		MirrorFirst: true,
	}
	got := buildSources("https://github.com/org/repo.git", cfg)
	if len(got) != 2 {
		t.Fatalf("sources length = %d, want 2", len(got))
	}
	if got[0].Name != "m1" {
		t.Fatalf("first source = %q, want m1", got[0].Name)
	}
	if got[1].Name != "origin" {
		t.Fatalf("second source = %q, want origin", got[1].Name)
	}
}

func TestCloneFallbackAndRetry(t *testing.T) {
	fakeBin := t.TempDir()
	logFile := filepath.Join(fakeBin, "git.calls")
	script := filepath.Join(fakeBin, "git")
	body := "#!/bin/sh\n" +
		"echo \"$@\" >> \"$GITOPS_CALLS\"\n" +
		"if [ \"$1\" = \"clone\" ]; then\n" +
		"  case \"$2\" in\n" +
		"    *mirror.local*) exit 2 ;;\n" +
		"    *) mkdir -p \"$3\"; exit 0 ;;\n" +
		"  esac\n" +
		"fi\n" +
		"if [ \"$1\" = \"-C\" ]; then exit 0; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake git script: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+oldPath)
	t.Setenv("GITOPS_CALLS", logFile)

	mgr := New(config.Git{
		Mirrors:             []config.GitMirror{{Name: "mirror", BaseURL: "https://mirror.local", Enabled: true}},
		MirrorFirst:         true,
		RetryPerSource:      2,
		RetryBackoffSeconds: 0,
		CommandTimeoutSec:   5,
	}, nil)

	dest := filepath.Join(t.TempDir(), "repo")
	report, err := mgr.Clone(context.Background(), "https://github.com/acme/repo.git", dest)
	if err != nil {
		t.Fatalf("clone error: %v", err)
	}
	if !report.Success {
		t.Fatalf("report success=false")
	}
	if report.UsedSource.Name != "origin" {
		t.Fatalf("used source = %q, want origin", report.UsedSource.Name)
	}
	if len(report.Attempts) != 3 {
		t.Fatalf("attempts = %d, want 3 (2 mirror + 1 origin)", len(report.Attempts))
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("destination not created: %v", err)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("call lines = %d, want 3", len(lines))
	}
}
