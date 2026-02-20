package app

import (
	"testing"

	"maibot/internal/config"
)

func TestDefaultWorkspaceName(t *testing.T) {
	if defaultName != "main" {
		t.Fatalf("defaultName = %q, want main", defaultName)
	}
}

func TestValidateConfig(t *testing.T) {
	a := &App{cfg: config.Config{Installer: config.Installer{Repo: "x/y", ReleaseChannel: "stable", DataHome: "/tmp/x", InstanceTickInterval: "15s"}}}
	if err := a.validateConfig(); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
}
