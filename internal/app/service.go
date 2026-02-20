package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	kservice "github.com/kardianos/service"
	"maibot/internal/process"
)

type instanceServiceProgram struct {
	executable string
	args       []string
	workdir    string
	cmd        *exec.Cmd
}

func (p *instanceServiceProgram) Start(kservice.Service) error {
	p.cmd = exec.Command(p.executable, p.args...)
	p.cmd.Dir = p.workdir
	logFile := filepath.Join(p.workdir, "workspace.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	p.cmd.Stdout = f
	p.cmd.Stderr = f
	if err := p.cmd.Start(); err != nil {
		_ = f.Close()
		return err
	}
	go func() {
		_ = p.cmd.Wait()
		_ = f.Close()
	}()
	return nil
}

func (p *instanceServiceProgram) Stop(kservice.Service) error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return process.Stop(p.cmd.Process.Pid, 5*time.Second)
}

func (a *App) serviceAction(action, _ string) error {
	workdir, err := a.workspaceDir(defaultName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	prg := &instanceServiceProgram{
		executable: exe,
		args:       []string{instanceProc, workspaceID, defaultName},
		workdir:    workdir,
	}
	serviceName := workspaceServiceName(workdir)
	svc, err := kservice.New(prg, &kservice.Config{
		Name:             serviceName,
		DisplayName:      "MaiBot Workspace",
		Description:      "MaiBot workspace service " + serviceName,
		Arguments:        prg.args,
		WorkingDirectory: workdir,
	})
	if err != nil {
		return err
	}

	switch action {
	case "install":
		if err := svc.Install(); err != nil {
			return err
		}
	case "uninstall":
		if err := svc.Uninstall(); err != nil {
			return err
		}
	case "start":
		if err := svc.Start(); err != nil {
			return err
		}
	case "stop":
		if err := svc.Stop(); err != nil {
			return err
		}
	case "status":
		status, err := svc.Status()
		if err != nil {
			return err
		}
		fmt.Printf(a.tf("service.status_line", serviceName, status))
	default:
		return fmt.Errorf(a.tf("err.service_unsupported_action", action))
	}
	a.instanceLog.Infof(a.tf("log.service_action_completed", action))
	return nil
}

func workspaceServiceName(workdir string) string {
	workspaceRoot := filepath.Dir(workdir)
	base := sanitizeServiceToken(filepath.Base(workspaceRoot))
	hash := sha256Hex([]byte(workspaceRoot))
	if len(hash) > 8 {
		hash = hash[:8]
	}
	name := fmt.Sprintf("maibot-%s-%s", base, hash)
	if len(name) > 80 {
		name = name[:80]
	}
	return name
}

func sanitizeServiceToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "workspace"
	}
	var b strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "workspace"
	}
	return out
}
