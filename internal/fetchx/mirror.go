package fetchx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"maibot/internal/logging"
)

var defaultMirrors = []string{
	"https://ghfast.top",
	"https://gh.wuliya.xin",
	"https://gh-proxy.com",
	"https://github.moeyy.xyz",
}

type Resolver struct {
	mirrors  []string
	probeURL string
	timeout  time.Duration
	log      *logging.Logger
}

func NewResolver(mirrors []string, probeURL string, timeoutSeconds int, logger *logging.Logger) *Resolver {
	resolved := normalizeMirrors(mirrors)
	if len(resolved) == 0 {
		resolved = append([]string{}, defaultMirrors...)
	}
	probe := strings.TrimSpace(probeURL)
	if probe == "" {
		probe = "https://raw.githubusercontent.com/Mai-with-u/plugin-repo/refs/heads/main/plugins.json"
	}
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &Resolver{mirrors: resolved, probeURL: probe, timeout: timeout, log: logger}
}

func (r *Resolver) Resolve(ctx context.Context) (string, []string) {
	if strings.TrimSpace(r.probeURL) == "" || len(r.mirrors) == 0 {
		return "", append([]string{}, r.mirrors...)
	}
	for _, prefix := range r.mirrors {
		ok := r.probe(ctx, prefix)
		if ok {
			r.infof("download mirror selected: %s", prefix)
			return prefix, append([]string{}, r.mirrors...)
		}
		r.warnf("download mirror probe failed: %s", prefix)
	}
	r.warnf("all download mirrors probe failed, fallback to direct")
	return "", append([]string{}, r.mirrors...)
}

func (r *Resolver) probe(parent context.Context, prefix string) bool {
	ctx, cancel := context.WithTimeout(parent, r.timeout)
	defer cancel()

	url := combine(prefix, r.probeURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: r.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 256))
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func normalizeMirrors(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, item := range in {
		trimmed := strings.TrimRight(strings.TrimSpace(item), "/")
		if trimmed == "" {
			continue
		}
		if seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func combine(prefix, rawURL string) string {
	p := strings.TrimRight(strings.TrimSpace(prefix), "/")
	u := strings.TrimSpace(rawURL)
	if p == "" {
		return u
	}
	if u == "" {
		return p
	}
	if strings.HasPrefix(u, "https://") {
		u = strings.TrimPrefix(u, "https://")
	} else if strings.HasPrefix(u, "http://") {
		u = strings.TrimPrefix(u, "http://")
	}
	return fmt.Sprintf("%s/%s", p, u)
}

func (r *Resolver) infof(format string, args ...any) {
	if r.log == nil {
		return
	}
	r.log.Infof(format, args...)
}

func (r *Resolver) warnf(format string, args ...any) {
	if r.log == nil {
		return
	}
	r.log.Warnf(format, args...)
}
