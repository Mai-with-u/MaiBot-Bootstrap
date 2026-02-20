package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"maibot/internal/process"
)

const (
	workspaceID             = "workspace"
	workspaceStateInstalled = "installed"
	workspaceStateRunning   = "running"
	workspaceStateStopped   = "stopped"
	workspaceStateUpdating  = "updating"
)

type workspaceConfig struct {
	Version   int       `json:"version"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Status    string    `json:"status"`
	PID       int       `json:"pid"`
}

func (a *App) dataRoot() (string, error) {
	root := strings.TrimSpace(a.cfg.Installer.DataHome)
	if root == "" {
		return "", errors.New("installer.data_home is empty")
	}
	return root, nil
}

func (a *App) workspaceDir() (string, error) {
	root, err := a.dataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "workspace"), nil
}

func (a *App) workspaceConfigPath() (string, error) {
	dir, err := a.workspaceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func (a *App) workspaceLogPath() (string, error) {
	dir, err := a.workspaceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "workspace.log"), nil
}

func (a *App) installInstance(_ string) error {
	dir, err := a.workspaceDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	now := time.Now().UTC()
	cfg := workspaceConfig{
		Version:   configVersion,
		Name:      defaultName,
		CreatedAt: now,
		UpdatedAt: now,
		Status:    workspaceStateInstalled,
		PID:       0,
	}

	configPath, err := a.workspaceConfigPath()
	if err != nil {
		return err
	}
	if existing, readErr := a.readWorkspaceConfig(); readErr == nil {
		cfg = existing
		if cfg.Name == "" {
			cfg.Name = defaultName
		}
		if cfg.CreatedAt.IsZero() {
			cfg.CreatedAt = now
		}
		cfg.UpdatedAt = now
		cfg.Status = workspaceStateInstalled
		cfg.PID = 0
	}
	if err := writeWorkspaceConfig(configPath, cfg); err != nil {
		return err
	}

	logPath, err := a.workspaceLogPath()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(logPath, os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func (a *App) startInstance(_ string) error {
	cfg, err := a.readWorkspaceConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("workspace is not initialized, run: maibot install")
		}
		return err
	}

	if cfg.PID > 0 {
		if process.IsAlive(cfg.PID) {
			a.instanceLog.Infof("workspace already running with pid %d", cfg.PID)
			return nil
		}
		cfg.PID = 0
	}

	dir, err := a.workspaceDir()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logPath, err := a.workspaceLogPath()
	if err != nil {
		return err
	}
	lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, instanceProc, workspaceID, defaultName)
	cmd.Dir = dir
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.Env = append(os.Environ(), "MAIBOT_WORKSPACE_DIR="+dir)

	if err := cmd.Start(); err != nil {
		_ = lf.Close()
		return err
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		_ = lf.Close()
		return err
	}
	if err := lf.Close(); err != nil {
		return err
	}

	cfg.Status = workspaceStateRunning
	cfg.PID = pid
	cfg.UpdatedAt = time.Now().UTC()
	configPath, err := a.workspaceConfigPath()
	if err != nil {
		return err
	}
	if err := writeWorkspaceConfig(configPath, cfg); err != nil {
		return err
	}
	return nil
}

func (a *App) stopInstance(_ string) error {
	cfg, err := a.readWorkspaceConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("workspace is not initialized, run: maibot install")
		}
		return err
	}

	if cfg.PID > 0 {
		if err := process.Stop(cfg.PID, 6*time.Second); err != nil {
			return err
		}
	}
	cfg.Status = workspaceStateStopped
	cfg.PID = 0
	cfg.UpdatedAt = time.Now().UTC()
	configPath, err := a.workspaceConfigPath()
	if err != nil {
		return err
	}
	return writeWorkspaceConfig(configPath, cfg)
}

func (a *App) restartInstance(_ string) error {
	if err := a.stopInstance(defaultName); err != nil {
		return err
	}
	return a.startInstance(defaultName)
}

func (a *App) statusInstance(_ string) error {
	cfg, err := a.readWorkspaceConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("workspace is not initialized")
		}
		return err
	}

	state := cfg.Status
	if cfg.PID > 0 && process.IsAlive(cfg.PID) {
		state = workspaceStateRunning
	} else if state == workspaceStateRunning {
		state = workspaceStateStopped
	}

	fmt.Printf("workspace=%s\n", cfg.Name)
	fmt.Printf("id=%s\n", workspaceID)
	fmt.Printf("state=%s\n", state)
	fmt.Printf("pid=%d\n", cfg.PID)
	fmt.Printf("updated_at=%s\n", cfg.UpdatedAt.Format(time.RFC3339))
	return nil
}

func (a *App) logsInstance(_ string, tail int) error {
	logPath, err := a.workspaceLogPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("workspace log not found")
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if tail <= 0 {
		tail = 50
	}
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	for _, line := range lines {
		_, _ = io.WriteString(os.Stdout, line+"\n")
	}
	return nil
}

func (a *App) runInstance(id string, displayName string) {
	interval := 15 * time.Second
	if d, err := time.ParseDuration(strings.TrimSpace(a.cfg.Installer.InstanceTickInterval)); err == nil && d > 0 {
		interval = d
	}

	a.instanceLog.Infof("workspace worker started: %s (%s)", displayName, id)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		a.instanceLog.Infof("heartbeat workspace=%s id=%s", displayName, id)
	}
}

func (a *App) updateInstance(_ string) error {
	cfg, err := a.readWorkspaceConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("workspace is not initialized, run: maibot install")
		}
		return err
	}
	cfg.Status = workspaceStateUpdating
	cfg.UpdatedAt = time.Now().UTC()
	configPath, err := a.workspaceConfigPath()
	if err != nil {
		return err
	}
	if err := writeWorkspaceConfig(configPath, cfg); err != nil {
		return err
	}
	cfg.Status = workspaceStateInstalled
	cfg.UpdatedAt = time.Now().UTC()
	return writeWorkspaceConfig(configPath, cfg)
}

func (a *App) readWorkspaceConfig() (workspaceConfig, error) {
	configPath, err := a.workspaceConfigPath()
	if err != nil {
		return workspaceConfig{}, err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return workspaceConfig{}, err
	}
	var cfg workspaceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return workspaceConfig{}, err
	}
	if cfg.Name == "" {
		cfg.Name = defaultName
	}
	return cfg, nil
}

func writeWorkspaceConfig(path string, cfg workspaceConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func sha256Hex(in []byte) string {
	sum := sha256.Sum256(in)
	return hex.EncodeToString(sum[:])
}
