package execx

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestConfirmRejectsWhenNoTTY(t *testing.T) {
	r := &Runner{
		In:     strings.NewReader("y\n"),
		Out:    &strings.Builder{},
		Err:    &strings.Builder{},
		IsTTY:  func() bool { return false },
		IsRoot: func() bool { return false },
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Run(ctx, "echo", []string{"ok"}, Options{Sensitive: true}); err == nil {
		t.Fatalf("expected error without TTY")
	}
}

func TestSensitiveConfirmCancel(t *testing.T) {
	r := &Runner{
		In:     strings.NewReader("n\n"),
		Out:    &strings.Builder{},
		Err:    &strings.Builder{},
		IsTTY:  func() bool { return true },
		IsRoot: func() bool { return true },
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Run(ctx, "echo", []string{"ok"}, Options{Sensitive: true}); err == nil {
		t.Fatalf("expected cancel error")
	}
}

func TestRequireSudoWithFakeRootSkipsSudo(t *testing.T) {
	r := &Runner{
		In:     strings.NewReader(""),
		Out:    &strings.Builder{},
		Err:    &strings.Builder{},
		IsTTY:  func() bool { return true },
		IsRoot: func() bool { return true },
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := r.Run(ctx, "nonexistent-command", nil, Options{RequireSudo: true})
	if err == nil {
		t.Fatalf("expected error executing nonexistent command")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		return
	}
}
