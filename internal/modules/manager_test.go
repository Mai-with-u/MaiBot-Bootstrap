package modules

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"maibot/internal/config"
	"maibot/internal/execx"
)

type fakeExecutor struct {
	failUntil map[string]int
	calls     map[string]int
}

func (f *fakeExecutor) Run(_ context.Context, name string, args []string, _ execx.Options) error {
	key := name
	if len(args) > 0 {
		key += " " + args[0]
	}
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[key]++
	if f.failUntil == nil {
		return nil
	}
	if threshold, ok := f.failUntil[key]; ok && f.calls[key] <= threshold {
		return fmt.Errorf("intentional failure %s call=%d", key, f.calls[key])
	}
	return nil
}

func TestInstallWithRetry(t *testing.T) {
	cfg := config.Modules{
		InstallRetries:    2,
		InstallBackoffSec: 0,
	}
	exec := &fakeExecutor{failUntil: map[string]int{"installer napcat": 1}}
	mgr := newWithProviders(cfg, config.Mirrors{}, nil, exec, []Provider{NewStaticProvider("test", []config.ModuleDefinition{
		{Name: "napcat", Install: []config.ModuleStep{{Name: "install napcat", Command: "installer", Args: []string{"napcat"}}}},
	})})
	report, err := mgr.Install(context.Background(), "napcat")
	if err != nil {
		t.Fatalf("install error: %v", err)
	}
	if !report.Success {
		t.Fatalf("report success=false")
	}
	if len(report.Attempts) != 2 {
		t.Fatalf("attempts=%d, want 2", len(report.Attempts))
	}
}

func TestListFromHTTPCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"modules":[{"name":"napcat","description":"qq bridge","install":[{"name":"step","command":"echo","args":["ok"]}]}]}`))
	}))
	defer server.Close()

	mgr := newWithProviders(config.Modules{CatalogURLs: []string{server.URL}, CatalogTimeoutSec: 2, InstallRetries: 1}, config.Mirrors{}, nil, &fakeExecutor{}, []Provider{
		NewHTTPProvider(server.URL, 2),
	})
	defs, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(defs) != 1 || defs[0].Name != "napcat" {
		t.Fatalf("unexpected defs: %+v", defs)
	}
}
