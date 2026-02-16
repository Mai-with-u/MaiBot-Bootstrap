package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	logFile := filepath.Join(p.workdir, "instance.log")
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

func (a *App) serviceAction(action, name string) error {
	target, err := a.resolveInstanceTarget(name)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	prg := &instanceServiceProgram{
		executable: exe,
		args:       []string{instanceProc, target.ID, target.DisplayName},
		workdir:    target.Dir,
	}
	serviceName := "maibot-" + target.ID[:12]
	svc, err := kservice.New(prg, &kservice.Config{
		Name:             serviceName,
		DisplayName:      "MaiBot " + target.DisplayName,
		Description:      "MaiBot managed instance " + target.DisplayName,
		Arguments:        prg.args,
		WorkingDirectory: target.Dir,
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
		fmt.Printf("service=%s status=%v\n", serviceName, status)
	default:
		return fmt.Errorf("unsupported service action: %s", action)
	}
	a.instanceLog.Infof("service action %s completed for %s", action, target.DisplayName)
	return nil
}
