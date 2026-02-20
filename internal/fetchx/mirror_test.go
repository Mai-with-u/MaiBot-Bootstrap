package fetchx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCombine(t *testing.T) {
	got := combine("https://ghfast.top", "https://github.com/a/b")
	want := "https://ghfast.top/github.com/a/b"
	if got != want {
		t.Fatalf("combine = %q, want %q", got, want)
	}
}

func TestResolveSelectsFirstHealthy(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadGateway)
	}))
	defer bad.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer good.Close()

	r := NewResolver([]string{bad.URL, good.URL}, "https://raw.githubusercontent.com/Mai-with-u/plugin-repo/refs/heads/main/plugins.json", 2, nil)
	selected, all := r.Resolve(context.Background())
	if selected != good.URL {
		t.Fatalf("selected=%q, want %q", selected, good.URL)
	}
	if len(all) != 2 {
		t.Fatalf("mirrors len=%d, want 2", len(all))
	}
}
