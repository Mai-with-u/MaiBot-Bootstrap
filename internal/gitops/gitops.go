package gitops

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"strings"
	"time"

	"maibot/internal/config"
	"maibot/internal/logging"
)

type Operation string

const (
	OperationClone Operation = "clone"
	OperationPull  Operation = "pull"
)

type Source struct {
	Name string
	URL  string
}

type Attempt struct {
	Source     Source
	Attempt    int
	StartedAt  time.Time
	EndedAt    time.Time
	DurationMS int64
	Error      string
}

type Report struct {
	Operation  Operation
	Target     string
	StartedAt  time.Time
	EndedAt    time.Time
	Success    bool
	UsedSource Source
	Attempts   []Attempt
}

type Manager struct {
	cfg config.Git
	log *logging.Logger
}

func New(cfg config.Git, logger *logging.Logger) *Manager {
	return &Manager{cfg: cfg, log: logger}
}

func (m *Manager) Clone(ctx context.Context, repoURL string, destination string) (Report, error) {
	report := Report{Operation: OperationClone, Target: destination, StartedAt: time.Now().UTC()}
	sources := buildSources(repoURL, m.cfg)
	if len(sources) == 0 {
		report.EndedAt = time.Now().UTC()
		return report, errors.New("no git source available")
	}
	for _, src := range sources {
		for attempt := 1; attempt <= m.cfg.RetryPerSource; attempt++ {
			one := Attempt{Source: src, Attempt: attempt, StartedAt: time.Now().UTC()}
			args := []string{"clone", src.URL, destination}
			err := m.runGit(ctx, args, "")
			one.EndedAt = time.Now().UTC()
			one.DurationMS = one.EndedAt.Sub(one.StartedAt).Milliseconds()
			if err != nil {
				one.Error = err.Error()
				report.Attempts = append(report.Attempts, one)
				m.warnf("git clone failed source=%s attempt=%d err=%v", src.Name, attempt, err)
				if attempt < m.cfg.RetryPerSource {
					time.Sleep(time.Duration(m.cfg.RetryBackoffSeconds) * time.Second)
				}
				continue
			}
			report.Attempts = append(report.Attempts, one)
			report.Success = true
			report.UsedSource = src
			report.EndedAt = time.Now().UTC()
			m.okf("git clone success source=%s destination=%s", src.Name, destination)
			return report, nil
		}
	}
	report.EndedAt = time.Now().UTC()
	return report, fmt.Errorf("git clone failed after trying %d sources", len(sources))
}

func (m *Manager) Pull(ctx context.Context, repoDir string) (Report, error) {
	report := Report{Operation: OperationPull, Target: repoDir, StartedAt: time.Now().UTC()}
	args := []string{"-C", repoDir, "pull", "--ff-only"}
	one := Attempt{Source: Source{Name: "origin", URL: "(configured in repo)"}, Attempt: 1, StartedAt: time.Now().UTC()}
	err := m.runGit(ctx, args, repoDir)
	one.EndedAt = time.Now().UTC()
	one.DurationMS = one.EndedAt.Sub(one.StartedAt).Milliseconds()
	if err != nil {
		one.Error = err.Error()
		report.Attempts = append(report.Attempts, one)
		report.EndedAt = time.Now().UTC()
		m.warnf("git pull failed dir=%s err=%v", repoDir, err)
		return report, err
	}
	report.Attempts = append(report.Attempts, one)
	report.Success = true
	report.UsedSource = one.Source
	report.EndedAt = time.Now().UTC()
	m.okf("git pull success dir=%s", repoDir)
	return report, nil
}

func (m *Manager) warnf(format string, args ...any) {
	if m.log == nil {
		return
	}
	m.log.Warnf(format, args...)
}

func (m *Manager) okf(format string, args ...any) {
	if m.log == nil {
		return
	}
	m.log.Okf(format, args...)
}

func (m *Manager) runGit(ctx context.Context, args []string, workdir string) error {
	timeout := time.Duration(m.cfg.CommandTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	if strings.TrimSpace(workdir) != "" {
		cmd.Dir = workdir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return fmt.Errorf("%w: %s", err, trimmed)
		}
		return err
	}
	return nil
}

func buildSources(repoURL string, cfg config.Git) []Source {
	original := Source{Name: "origin", URL: repoURL}
	mirrorSources := make([]Source, 0, len(cfg.Mirrors))
	for _, mirror := range cfg.Mirrors {
		if !mirror.Enabled {
			continue
		}
		if u := rewriteURL(repoURL, mirror.BaseURL); u != "" {
			mirrorSources = append(mirrorSources, Source{Name: mirror.Name, URL: u})
		}
	}
	if cfg.MirrorFirst {
		return append(mirrorSources, original)
	}
	out := []Source{original}
	return append(out, mirrorSources...)
}

func rewriteURL(repoURL string, mirrorBase string) string {
	parsedRepo, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil {
		return ""
	}
	if parsedRepo.Host == "" || parsedRepo.Path == "" {
		return ""
	}
	base, err := url.Parse(strings.TrimSpace(mirrorBase))
	if err != nil || base.Host == "" {
		return ""
	}
	base.Path = path.Join(base.Path, parsedRepo.Host, strings.TrimPrefix(parsedRepo.Path, "/"))
	base.RawQuery = ""
	base.Fragment = ""
	return base.String()
}
