package app

import (
	"errors"
	"os"
	"path/filepath"
)

func (a *App) cleanup(args []string) error {
	if len(args) == 0 || args[0] != "--test-artifacts" {
		return errors.New("usage: maibot cleanup --test-artifacts [instance_names...]")
	}

	if err := cleanupRepoArtifacts(); err != nil {
		return err
	}

	targets := args[1:]
	if len(targets) == 0 {
		targets = []string{"demo", "demo2", "麦麦🚀"}
	}

	for _, name := range targets {
		if err := a.removeInstanceByName(name); err != nil {
			return err
		}
	}

	if err := a.cleanupLocks(); err != nil {
		return err
	}
	return nil
}

func cleanupRepoArtifacts() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	paths := []string{
		filepath.Join(cwd, "maibot"),
		filepath.Join(cwd, "dist"),
	}
	for _, p := range paths {
		if err := removePathIfExists(p); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) removeInstanceByName(name string) error {
	target, err := a.resolveInstanceTarget(name)
	if err != nil {
		return err
	}
	dir := target.Dir
	configPath := target.ConfigPath
	cfg, err := readConfig(configPath)
	if err == nil && cfg.PID > 0 {
		process, findErr := os.FindProcess(cfg.PID)
		if findErr == nil {
			_ = process.Kill()
		}
	}
	if err := removePathIfExists(dir); err != nil {
		return err
	}
	if err := a.removeRegistryEntry(target.ID); err != nil {
		return err
	}
	a.cleanupLog.Infof("removed test instance: %s (%s)", target.DisplayName, target.ID)
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
