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

func (a *App) workspaceDir(name string) (string, error) {
	_, _ = a, name
	dir, found, err := detectWorkspaceDir()
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("workspace is not initialized in current directory, run: maibot init")
	}
	return dir, nil
}

func workspaceDirForInit() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, ".maibot"), nil
}

func detectWorkspaceDir() (string, bool, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", false, err
	}
	cur := start
	for {
		candidate := filepath.Join(cur, ".maibot")
		st, statErr := os.Stat(candidate)
		if statErr == nil && st.IsDir() {
			return candidate, true, nil
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", false, statErr
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", false, nil
}

func sanitizeWorkspaceName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return defaultName
	}
	var b strings.Builder
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return defaultName
	}
	return result
}

func (a *App) workspaceConfigPath(name string) (string, error) {
	dir, err := a.workspaceDir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func (a *App) workspaceLogPath(name string) (string, error) {
	dir, err := a.workspaceDir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "workspace.log"), nil
}

func (a *App) installInstance(name string) error {
	_, _ = a, name
	dir, err := workspaceDirForInit()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	now := time.Now().UTC()
	workspaceName := sanitizeWorkspaceName(name)
	if workspaceName == "" {
		workspaceName = defaultName
	}
	cfg := workspaceConfig{
		Version:   configVersion,
		Name:      workspaceName,
		CreatedAt: now,
		UpdatedAt: now,
		Status:    workspaceStateInstalled,
		PID:       0,
	}

	configPath := filepath.Join(dir, "config.json")
	var readErr error
	var existing workspaceConfig
	existing, readErr = readWorkspaceConfigByPath(configPath)
	if readErr == nil {
		cfg = existing
		if cfg.Name == "" {
			cfg.Name = workspaceName
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

	logPath := filepath.Join(dir, "workspace.log")
	f, err := os.OpenFile(logPath, os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func (a *App) startInstance(name string) error {
	selected := sanitizeWorkspaceName(name)
	cfg, err := a.readWorkspaceConfig(selected)
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

	dir, err := a.workspaceDir(selected)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logPath, err := a.workspaceLogPath(selected)
	if err != nil {
		return err
	}
	lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, instanceProc, workspaceID, selected)
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
	configPath, err := a.workspaceConfigPath(selected)
	if err != nil {
		return err
	}
	if err := writeWorkspaceConfig(configPath, cfg); err != nil {
		return err
	}
	return nil
}

func (a *App) stopInstance(name string) error {
	selected := sanitizeWorkspaceName(name)
	cfg, err := a.readWorkspaceConfig(selected)
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
	configPath, err := a.workspaceConfigPath(selected)
	if err != nil {
		return err
	}
	return writeWorkspaceConfig(configPath, cfg)
}

func (a *App) restartInstance(_ string) error {
	selected := sanitizeWorkspaceName(defaultName)
	if err := a.stopInstance(selected); err != nil {
		return err
	}
	return a.startInstance(selected)
}

func (a *App) statusInstance(name string) error {
	cfg, err := a.readWorkspaceConfig(name)
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

func (a *App) logsInstance(name string, tail int) error {
	logPath, err := a.workspaceLogPath(name)
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

func (a *App) updateInstance(name string) error {
	cfg, err := a.readWorkspaceConfig(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("workspace is not initialized, run: maibot install")
		}
		return err
	}
	cfg.Status = workspaceStateUpdating
	cfg.UpdatedAt = time.Now().UTC()
	configPath, err := a.workspaceConfigPath(name)
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

func (a *App) readWorkspaceConfig(name string) (workspaceConfig, error) {
	configPath, err := a.workspaceConfigPath(name)
	if err != nil {
		return workspaceConfig{}, err
	}
	return readWorkspaceConfigByPath(configPath)
}

func readWorkspaceConfigByPath(configPath string) (workspaceConfig, error) {
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

func (a *App) listWorkspaces(roots []string, maxDepth int) error {
	if len(roots) == 0 {
		roots = []string{"."}
	}

	currentMarker := ""
	if curDir, found, err := detectWorkspaceDir(); err == nil && found {
		currentMarker = curDir
	}

	type row struct {
		name string
		root string
	}
	rows := make([]row, 0)
	seen := map[string]bool{}
	processWorkspace := func(workspaceRoot string) {
		if seen[workspaceRoot] {
			return
		}
		cfgPath := filepath.Join(workspaceRoot, ".maibot", "config.json")
		cfg, cfgErr := readWorkspaceConfigByPath(cfgPath)
		if cfgErr != nil {
			cfg = workspaceConfig{Name: filepath.Base(workspaceRoot)}
		}
		rows = append(rows, row{name: cfg.Name, root: workspaceRoot})
		seen[workspaceRoot] = true
	}

	type scanNode struct {
		path  string
		depth int
	}

	for _, root := range roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return err
		}

		queue := []scanNode{{path: absRoot, depth: 0}}
		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]

			entries, err := os.ReadDir(node.path)
			if err != nil {
				return err
			}

			hasWorkspace := false
			for _, entry := range entries {
				if entry.IsDir() && entry.Name() == ".maibot" {
					hasWorkspace = true
					break
				}
			}
			if hasWorkspace {
				processWorkspace(node.path)
				continue
			}

			if maxDepth >= 0 && node.depth >= maxDepth {
				continue
			}

			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				name := entry.Name()
				if name == ".git" || name == "node_modules" || name == ".idea" || name == "vendor" || name == "dist" || name == "build" || strings.HasPrefix(name, ".") {
					continue
				}
				queue = append(queue, scanNode{path: filepath.Join(node.path, name), depth: node.depth + 1})
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].name == rows[j].name {
			return rows[i].root < rows[j].root
		}
		return rows[i].name < rows[j].name
	})

	if len(rows) == 0 {
		fmt.Println("no workspace found")
		return nil
	}
	for _, r := range rows {
		marker := " "
		if currentMarker != "" && filepath.Join(r.root, ".maibot") == currentMarker {
			marker = "*"
		}
		fmt.Printf("%s %s\t%s\n", marker, r.name, r.root)
	}
	return nil
}
