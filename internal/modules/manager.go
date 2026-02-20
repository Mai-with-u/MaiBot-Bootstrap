package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"maibot/internal/config"
	"maibot/internal/execx"
	"maibot/internal/fetchx"
	"maibot/internal/logging"
)

type Provider interface {
	Name() string
	List(ctx context.Context) ([]config.ModuleDefinition, error)
}

type Executor interface {
	Run(ctx context.Context, name string, args []string, opts execx.Options) error
}

type InstallAttempt struct {
	StepName  string
	Command   string
	Args      []string
	Try       int
	StartedAt time.Time
	EndedAt   time.Time
	Error     string
}

type InstallReport struct {
	Module     string
	Source     string
	StartedAt  time.Time
	EndedAt    time.Time
	Success    bool
	Attempts   []InstallAttempt
	Resolution string
}

type Manager struct {
	cfg      config.Modules
	mirrors  config.Mirrors
	log      *logging.Logger
	executor Executor

	providers []Provider
}

func New(cfg config.Modules, mirrors config.Mirrors, logger *logging.Logger, executor Executor) *Manager {
	providers := []Provider{NewStaticProvider("builtin", BuiltinDefinitions())}
	for _, u := range cfg.CatalogURLs {
		trimmed := strings.TrimSpace(u)
		if trimmed == "" {
			continue
		}
		providers = append(providers, NewHTTPProvider(trimmed, cfg.CatalogTimeoutSec))
	}
	if cfg.PreferCatalogSource && len(providers) > 1 {
		providers = append(providers[1:], providers[0])
	}
	return newWithProviders(cfg, mirrors, logger, executor, providers)
}

func newWithProviders(cfg config.Modules, mirrors config.Mirrors, logger *logging.Logger, executor Executor, providers []Provider) *Manager {
	if executor == nil {
		executor = execx.NewRunner()
	}
	if len(providers) == 0 {
		providers = []Provider{NewStaticProvider("builtin", BuiltinDefinitions())}
	}
	return &Manager{
		cfg:       cfg,
		mirrors:   mirrors,
		log:       logger,
		executor:  executor,
		providers: providers,
	}
}

func (m *Manager) List(ctx context.Context) ([]config.ModuleDefinition, error) {
	type origin struct {
		name string
		def  config.ModuleDefinition
	}
	defs := map[string]origin{}
	order := make([]string, 0)
	for _, p := range m.providers {
		items, err := p.List(ctx)
		if err != nil {
			m.warnf("load module catalog failed provider=%s err=%v", p.Name(), err)
			continue
		}
		for _, def := range items {
			name := strings.TrimSpace(def.Name)
			if name == "" {
				continue
			}
			if _, exists := defs[name]; !exists {
				order = append(order, name)
			}
			defs[name] = origin{name: p.Name(), def: def}
		}
	}
	out := make([]config.ModuleDefinition, 0, len(order))
	for _, name := range order {
		out = append(out, defs[name].def)
	}
	return out, nil
}

func (m *Manager) Install(ctx context.Context, moduleName string) (InstallReport, error) {
	report := InstallReport{Module: moduleName, StartedAt: time.Now().UTC()}
	def, source, err := m.resolveModule(ctx, moduleName)
	if err != nil {
		report.EndedAt = time.Now().UTC()
		return report, err
	}
	report.Source = source
	if len(def.Install) == 0 {
		report.EndedAt = time.Now().UTC()
		return report, fmt.Errorf("module %q has no install steps", moduleName)
	}
	proxyPrefix, candidates := fetchx.NewResolver(m.mirrors.URLs, m.mirrors.ProbeURL, m.mirrors.ProbeSeconds, m.log).Resolve(ctx)
	mirrorJoined := strings.Join(candidates, ",")
	env := map[string]string{
		"MAIBOT_PROXY_PREFIX":  proxyPrefix,
		"MAIBOT_PROXY_MIRRORS": mirrorJoined,
	}
	if proxyPrefix == "" {
		m.warnf("module download proxy fallback to direct, mirrors=%s", mirrorJoined)
	} else {
		m.okf("module download proxy selected=%s", proxyPrefix)
	}

	for _, step := range def.Install {
		stepName := strings.TrimSpace(step.Name)
		if stepName == "" {
			stepName = step.Command
		}
		if strings.TrimSpace(step.Command) == "" {
			report.EndedAt = time.Now().UTC()
			return report, fmt.Errorf("module %q has invalid step with empty command", moduleName)
		}

		var lastErr error
		for try := 1; try <= m.cfg.InstallRetries; try++ {
			attempt := InstallAttempt{
				StepName:  stepName,
				Command:   step.Command,
				Args:      append([]string{}, step.Args...),
				Try:       try,
				StartedAt: time.Now().UTC(),
			}
			err := m.executor.Run(ctx, step.Command, step.Args, execx.Options{
				Sensitive:   step.Sensitive,
				RequireSudo: step.RequireSudo,
				Prompt:      step.Prompt,
				Env:         env,
			})
			attempt.EndedAt = time.Now().UTC()
			if err != nil {
				attempt.Error = err.Error()
				report.Attempts = append(report.Attempts, attempt)
				lastErr = err
				m.warnf("module install step failed module=%s step=%s try=%d err=%v", moduleName, stepName, try, err)
				if try < m.cfg.InstallRetries && m.cfg.InstallBackoffSec > 0 {
					time.Sleep(time.Duration(m.cfg.InstallBackoffSec) * time.Second)
				}
				continue
			}
			report.Attempts = append(report.Attempts, attempt)
			lastErr = nil
			m.okf("module install step success module=%s step=%s try=%d", moduleName, stepName, try)
			break
		}
		if lastErr != nil {
			report.EndedAt = time.Now().UTC()
			report.Resolution = "failed"
			return report, fmt.Errorf("module %q install failed at step %q: %w", moduleName, stepName, lastErr)
		}
	}

	report.Success = true
	report.Resolution = "installed"
	report.EndedAt = time.Now().UTC()
	m.okf("module install success module=%s source=%s", moduleName, source)
	return report, nil
}

func (m *Manager) resolveModule(ctx context.Context, moduleName string) (config.ModuleDefinition, string, error) {
	name := strings.TrimSpace(moduleName)
	if name == "" {
		return config.ModuleDefinition{}, "", fmt.Errorf("module name is empty")
	}
	for _, p := range m.providers {
		items, err := p.List(ctx)
		if err != nil {
			m.warnf("module provider unavailable provider=%s err=%v", p.Name(), err)
			continue
		}
		for _, def := range items {
			if strings.EqualFold(strings.TrimSpace(def.Name), name) {
				return def, p.Name(), nil
			}
		}
	}
	return config.ModuleDefinition{}, "", fmt.Errorf("module %q not found in configured catalogs", moduleName)
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

type StaticProvider struct {
	name        string
	definitions []config.ModuleDefinition
}

func NewStaticProvider(name string, definitions []config.ModuleDefinition) *StaticProvider {
	copyDefs := make([]config.ModuleDefinition, len(definitions))
	copy(copyDefs, definitions)
	if strings.TrimSpace(name) == "" {
		name = "static"
	}
	return &StaticProvider{name: name, definitions: copyDefs}
}

func (p *StaticProvider) Name() string { return p.name }

func (p *StaticProvider) List(_ context.Context) ([]config.ModuleDefinition, error) {
	copyDefs := make([]config.ModuleDefinition, len(p.definitions))
	copy(copyDefs, p.definitions)
	return copyDefs, nil
}

type HTTPProvider struct {
	url     string
	timeout time.Duration
}

func NewHTTPProvider(url string, timeoutSeconds int) *HTTPProvider {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPProvider{url: url, timeout: timeout}
}

func (p *HTTPProvider) Name() string { return "http:" + p.url }

func (p *HTTPProvider) List(ctx context.Context) ([]config.ModuleDefinition, error) {
	client := &http.Client{Timeout: p.timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("catalog status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		Modules []config.ModuleDefinition `json:"modules"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil && len(wrapped.Modules) > 0 {
		return wrapped.Modules, nil
	}
	var direct []config.ModuleDefinition
	if err := json.Unmarshal(body, &direct); err != nil {
		return nil, err
	}
	return direct, nil
}
