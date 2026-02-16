package execx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Runner struct {
	In     io.Reader
	Out    io.Writer
	Err    io.Writer
	IsTTY  func() bool
	IsRoot func() bool
}

type Options struct {
	Sensitive   bool
	RequireSudo bool
	Prompt      string
}

func NewRunner() *Runner {
	return &Runner{
		In:     os.Stdin,
		Out:    os.Stdout,
		Err:    os.Stderr,
		IsTTY:  isTTY,
		IsRoot: isRoot,
	}
}

func (r *Runner) Run(ctx context.Context, name string, args []string, opts Options) error {
	if opts.Sensitive {
		ok, err := r.confirm(opts.Prompt)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("operation cancelled")
		}
	}

	if opts.RequireSudo && !r.IsRoot() {
		if !r.IsTTY() {
			return errors.New("sudo requires a TTY")
		}
		if err := r.ensureSudo(ctx); err != nil {
			return err
		}
		return r.exec(ctx, "sudo", append([]string{name}, args...)...)
	}
	return r.exec(ctx, name, args...)
}

func (r *Runner) confirm(prompt string) (bool, error) {
	if !r.IsTTY() {
		return false, errors.New("confirmation requires a TTY")
	}
	text := strings.TrimSpace(prompt)
	if text == "" {
		text = "Sensitive operation. Continue?"
	}
	_, _ = fmt.Fprintf(r.Out, "%s [y/N]: ", text)
	reader := bufio.NewReader(r.In)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	value := strings.ToLower(strings.TrimSpace(line))
	return value == "y" || value == "yes", nil
}

func (r *Runner) ensureSudo(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sudo", "-v")
	cmd.Stdin = r.In
	cmd.Stdout = r.Out
	cmd.Stderr = r.Err
	return cmd.Run()
}

func (r *Runner) exec(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = r.In
	cmd.Stdout = r.Out
	cmd.Stderr = r.Err
	return cmd.Run()
}

func isTTY() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}
