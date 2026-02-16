package app

import (
	"testing"

	"maibot/internal/config"
)

func TestResolveLogsArgs(t *testing.T) {
	name, tail := resolveLogsArgs([]string{"demo", "--tail", "12"})
	if name != "demo" || tail != 12 {
		t.Fatalf("got name=%q tail=%d", name, tail)
	}

	name, tail = resolveLogsArgs([]string{"--tail", "7"})
	if name != defaultName || tail != 7 {
		t.Fatalf("got name=%q tail=%d", name, tail)
	}
}

func TestValidateConfig(t *testing.T) {
	a := &App{cfg: config.Config{Installer: config.Installer{Repo: "x/y", ReleaseChannel: "stable", DataHome: "/tmp/x", InstanceTickInterval: "15s"}}}
	if err := a.validateConfig(); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
}
