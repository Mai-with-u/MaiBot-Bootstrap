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
	"sort"
	"strings"
	"time"

	"maibot/internal/instance"
	"maibot/internal/process"
	"maibot/internal/registry"
)

type instanceConfig struct {
	Version     int       `json:"version"`
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Name        string    `json:"name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Status      string    `json:"status"`
	PID         int       `json:"pid"`
}

func (a *App) installInstance(name string) error {
	resolvedName := instanceName(name)
	resolvedID := instanceID(resolvedName)

	dir, err := a.instanceDir(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lock, err := a.acquireInstanceLockByID(resolvedID)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Release()
	}()

	now := time.Now().UTC()
	cfg := instanceConfig{
		Version:     configVersion,
		ID:          resolvedID,
		DisplayName: resolvedName,
		CreatedAt:   now,
		UpdatedAt:   now,
		Status:      instance.StateInstalled,
		PID:         0,
	}

	path := filepath.Join(dir, "config.json")
	if existing, err := readConfig(path); err == nil {
		cfg = existing
		if cfg.DisplayName == "" {
			cfg.DisplayName = instanceName(cfg.Name)
		}
		if cfg.ID == "" {
			cfg.ID = instanceID(cfg.DisplayName)
		}
		cfg.UpdatedAt = now
		if cfg.Status == "" {
			cfg.Status = instance.StateInstalled
		}
		if err := instance.ValidateTransition(cfg.Status, instance.StateInstalled); err != nil {
			return err
		}
		cfg.Status = instance.StateInstalled
	}
	cfg.DisplayName = resolvedName
	cfg.ID = resolvedID

	if err := writeConfig(path, cfg); err != nil {
		return err
	}
	if err := a.upsertRegistryEntry(cfg, dir); err != nil {
		return err
	}

	logPath := filepath.Join(dir, "instance.log")
	f, err := os.OpenFile(logPath, os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	a.instanceLog.Infof("instance directory: %s", dir)
	return nil
}

func (a *App) startInstance(name string) error {
	target, err := a.resolveInstanceTarget(name)
	if err != nil {
		return err
	}

	lock, err := a.acquireInstanceLockByID(target.ID)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Release()
	}()

	dir := target.Dir
	configPath := target.ConfigPath
	cfg, err := readConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("instance %q is not installed, run: maibot install %s", target.DisplayName, target.DisplayName)
		}
		return err
	}

	if cfg.PID != 0 {
		if process.IsAlive(cfg.PID) {
			a.instanceLog.Infof("instance already running with pid %d", cfg.PID)
			return nil
		}
		a.instanceLog.Warnf("stale pid %d found in config, recovering", cfg.PID)
		cfg.PID = 0
	}
	if cfg.Status == "" {
		cfg.Status = instance.StateInstalled
	}
	if err := instance.ValidateTransition(cfg.Status, instance.StateRunning); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logFile := filepath.Join(dir, "instance.log")
	lf, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, instanceProc, cfg.ID, cfg.DisplayName)
	cmd.Dir = dir
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.Env = append(os.Environ(), "MAIBOT_INSTANCE_DIR="+dir)

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

	cfg.Status = instance.StateRunning
	cfg.PID = pid
	cfg.UpdatedAt = time.Now().UTC()
	if err := writeConfig(configPath, cfg); err != nil {
		return err
	}
	if err := a.upsertRegistryEntry(cfg, dir); err != nil {
		return err
	}
	a.instanceLog.Infof("pid: %d", cfg.PID)
	return nil
}

func (a *App) stopInstance(name string) error {
	target, err := a.resolveInstanceTarget(name)
	if err != nil {
		return err
	}

	lock, err := a.acquireInstanceLockByID(target.ID)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Release()
	}()

	dir := target.Dir
	configPath := target.ConfigPath
	cfg, err := readConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("instance %q is not installed, run: maibot install %s", target.DisplayName, target.DisplayName)
		}
		return err
	}

	if cfg.Status == "" {
		cfg.Status = instance.StateInstalled
	}
	if err := instance.ValidateTransition(cfg.Status, instance.StateStopped); err != nil {
		return err
	}

	if cfg.PID > 0 {
		if err := process.Stop(cfg.PID, 6*time.Second); err != nil {
			return err
		}
	}
	cfg.Status = instance.StateStopped
	cfg.PID = 0
	cfg.UpdatedAt = time.Now().UTC()
	if err := writeConfig(configPath, cfg); err != nil {
		return err
	}
	if err := a.upsertRegistryEntry(cfg, dir); err != nil {
		return err
	}
	return nil
}

func (a *App) restartInstance(name string) error {
	if err := a.stopInstance(name); err != nil {
		return err
	}
	return a.startInstance(name)
}

func (a *App) statusInstance(name string) error {
	target, err := a.resolveInstanceTarget(name)
	if err != nil {
		return err
	}
	configPath := target.ConfigPath
	cfg, err := readConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("instance %q is not installed", target.DisplayName)
		}
		return err
	}

	running := cfg.PID > 0 && process.IsAlive(cfg.PID)
	state := cfg.Status
	if running {
		state = instance.StateRunning
	} else if state == instance.StateRunning {
		state = instance.StateStopped
	}

	fmt.Printf("name=%s\n", cfg.DisplayName)
	fmt.Printf("id=%s\n", cfg.ID)
	fmt.Printf("state=%s\n", state)
	fmt.Printf("pid=%d\n", cfg.PID)
	fmt.Printf("updated_at=%s\n", cfg.UpdatedAt.Format(time.RFC3339))
	return nil
}

func (a *App) logsInstance(name string, tail int) error {
	target, err := a.resolveInstanceTarget(name)
	if err != nil {
		return err
	}
	dir := target.Dir
	logPath := filepath.Join(dir, "instance.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("instance log not found for %q", target.DisplayName)
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

	a.instanceLog.Infof("instance worker started: %s (%s)", displayName, id)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		a.instanceLog.Infof("heartbeat instance=%s id=%s", displayName, id)
	}
}

func (a *App) updateInstance(name string) error {
	target, err := a.resolveInstanceTarget(name)
	if err != nil {
		return err
	}

	lock, err := a.acquireInstanceLockByID(target.ID)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Release()
	}()

	dir := target.Dir
	configPath := target.ConfigPath
	cfg, err := readConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("instance %q is not installed, run: maibot install %s", target.DisplayName, target.DisplayName)
		}
		return err
	}

	if cfg.Status == "" {
		cfg.Status = instance.StateInstalled
	}
	if err := instance.ValidateTransition(cfg.Status, instance.StateUpdating); err != nil {
		return err
	}
	cfg.Status = instance.StateUpdating
	cfg.UpdatedAt = time.Now().UTC()
	if err := writeConfig(configPath, cfg); err != nil {
		return err
	}

	if err := instance.ValidateTransition(cfg.Status, instance.StateInstalled); err != nil {
		return err
	}
	cfg.Status = instance.StateInstalled
	cfg.UpdatedAt = time.Now().UTC()
	if err := writeConfig(configPath, cfg); err != nil {
		return err
	}
	if err := a.upsertRegistryEntry(cfg, dir); err != nil {
		return err
	}

	a.updateLog.Infof("updated timestamp: %s", cfg.UpdatedAt.Format(time.RFC3339))
	return nil
}

func (a *App) listInstances() error {
	store, err := a.registryStore()
	if err != nil {
		return err
	}
	entries, err := store.List()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		if err := a.syncRegistryFromDisk(); err != nil {
			return err
		}
		entries, err = store.List()
		if err != nil {
			return err
		}
	}
	if len(entries) == 0 {
		fmt.Println("no instances")
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].DisplayName < entries[j].DisplayName
	})
	for _, e := range entries {
		fmt.Printf("%s\t%s\n", e.DisplayName, e.ID)
	}
	return nil
}

func (a *App) dataRoot() (string, error) {
	return strings.TrimSpace(a.cfg.Installer.DataHome), nil
}

func (a *App) instancesDir() (string, error) {
	root, err := a.dataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "instances"), nil
}

func (a *App) instanceDir(name string) (string, error) {
	base, err := a.instancesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, instanceID(name)), nil
}

func instanceName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return defaultName
	}
	return trimmed
}

func instanceID(name string) string {
	return sha256Hex([]byte(instanceName(name)))
}

func readConfig(path string) (instanceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return instanceConfig{}, err
	}
	var cfg instanceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return instanceConfig{}, err
	}
	if cfg.DisplayName == "" {
		cfg.DisplayName = instanceName(cfg.Name)
	}
	if cfg.ID == "" {
		cfg.ID = instanceID(cfg.DisplayName)
	}
	return cfg, nil
}

func writeConfig(path string, cfg instanceConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (a *App) acquireInstanceLockByID(id string) (*instance.Lock, error) {
	root, err := a.dataRoot()
	if err != nil {
		return nil, err
	}
	lockDir := filepath.Join(root, "locks")
	timeout := time.Duration(a.cfg.Installer.LockTimeoutSeconds) * time.Second
	return instance.AcquireLock(lockDir, id, timeout)
}

type instanceTarget struct {
	ID          string
	DisplayName string
	Dir         string
	ConfigPath  string
}

func (a *App) resolveInstanceTarget(ref string) (instanceTarget, error) {
	base, err := a.instancesDir()
	if err != nil {
		return instanceTarget{}, err
	}
	resolved := instanceName(ref)
	store, err := a.registryStore()
	if err != nil {
		return instanceTarget{}, err
	}
	entry, found, err := store.Resolve(resolved)
	if err != nil {
		return instanceTarget{}, err
	}
	if found {
		dir := strings.TrimSpace(entry.Path)
		if dir == "" {
			dir = filepath.Join(base, entry.ID)
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(base, entry.ID)
		}
		display := strings.TrimSpace(entry.DisplayName)
		if display == "" {
			display = resolved
		}
		return instanceTarget{
			ID:          entry.ID,
			DisplayName: display,
			Dir:         dir,
			ConfigPath:  filepath.Join(dir, "config.json"),
		}, nil
	}

	id := resolved
	if !isHexID(id) {
		id = instanceID(resolved)
	}
	dir := filepath.Join(base, id)
	return instanceTarget{
		ID:          id,
		DisplayName: resolved,
		Dir:         dir,
		ConfigPath:  filepath.Join(dir, "config.json"),
	}, nil
}

func (a *App) upsertRegistryEntry(cfg instanceConfig, dir string) error {
	store, err := a.registryStore()
	if err != nil {
		return err
	}
	return store.Upsert(registry.Entry{
		ID:          cfg.ID,
		DisplayName: cfg.DisplayName,
		Path:        dir,
		Status:      cfg.Status,
		CreatedAt:   cfg.CreatedAt,
		UpdatedAt:   cfg.UpdatedAt,
	})
}

func (a *App) removeRegistryEntry(id string) error {
	store, err := a.registryStore()
	if err != nil {
		return err
	}
	return store.RemoveByID(id)
}

func (a *App) registryStore() (*registry.Store, error) {
	base, err := a.instancesDir()
	if err != nil {
		return nil, err
	}
	return registry.New(filepath.Join(base, "index.json")), nil
}

func (a *App) syncRegistryFromDisk() error {
	base, err := a.instancesDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cfgPath := filepath.Join(base, e.Name(), "config.json")
		cfg, err := readConfig(cfgPath)
		if err != nil {
			continue
		}
		if cfg.Status == "" {
			cfg.Status = instance.StateInstalled
		}
		if cfg.CreatedAt.IsZero() {
			cfg.CreatedAt = time.Now().UTC()
		}
		if cfg.UpdatedAt.IsZero() {
			cfg.UpdatedAt = cfg.CreatedAt
		}
		if err := a.upsertRegistryEntry(cfg, filepath.Join(base, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func isHexID(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, r := range v {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func sha256Hex(in []byte) string {
	sum := sha256.Sum256(in)
	return hex.EncodeToString(sum[:])
}
