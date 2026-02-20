package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"maibot/internal/process"
)

func (a *App) cleanup() error {

	if err := cleanupRepoArtifacts(); err != nil {
		return err
	}

	if err := a.removeWorkspace(); err != nil {
		return err
	}

	if err := a.cleanupLocks(); err != nil {
		return err
	}
	return nil
}

func cleanupRepoArtifacts() error {
	if strings.TrimSpace(os.Getenv("MAIBOT_ALLOW_DEV_CLEANUP")) != "1" {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := removePathIfExists(filepath.Join(cwd, "maibot")); err != nil {
		return err
	}
	if err := removePathIfExists(filepath.Join(cwd, "dist")); err != nil {
		return err
	}
	return nil
}

func (a *App) removeWorkspace() error {
	dir, err := a.workspaceDir(defaultName)
	if err != nil {
		return err
	}
	cfg, err := a.readWorkspaceConfig(defaultName)
	if err == nil && cfg.PID > 0 {
		if process.IsAlive(cfg.PID) {
			_ = process.Stop(cfg.PID, 5*time.Second)
		}
	}
	if err := removePathIfExists(dir); err != nil {
		return err
	}
	a.cleanupLog.Infof(a.t("log.cleanup_workspace_artifacts_completed"))
	return nil
}

func (a *App) cleanupLocks() error {
	root, err := a.dataRoot()
	if err != nil {
		return err
	}
	lockDir := filepath.Join(root, "locks")
	entries, err := os.ReadDir(lockDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if err := removePathIfExists(filepath.Join(lockDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func removePathIfExists(path string) error {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}
