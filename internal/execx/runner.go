package execx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
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
	Env         map[string]string
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
		sudoArgs := []string{name}
		if len(opts.Env) > 0 {
			sudoArgs = append([]string{"env"}, envArgs(opts.Env)...)
			sudoArgs = append(sudoArgs, name)
		}
		sudoArgs = append(sudoArgs, args...)
		return r.exec(ctx, "sudo", sudoArgs, opts.Env)
	}
	return r.exec(ctx, name, args, opts.Env)
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

func (r *Runner) exec(ctx context.Context, name string, args []string, env map[string]string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = r.In
	cmd.Stdout = r.Out
	cmd.Stderr = r.Err
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), envArgs(env)...)
	}
	return cmd.Run()
}

func envArgs(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%s", k, env[k]))
	}
	return out
}

func isTTY() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}
