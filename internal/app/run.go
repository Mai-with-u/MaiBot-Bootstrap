package app

import (
	"context"
	"time"

	"maibot/internal/execx"
)

func (a *App) runCommand(args []string, sensitive bool, sudo bool, prompt string) error {
	if len(args) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	runner := execx.NewRunner()
	return runner.Run(ctx, args[0], args[1:], execx.Options{
		Sensitive:   sensitive,
		RequireSudo: sudo,
		Prompt:      prompt,
	})
}
