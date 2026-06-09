package common

import (
	"context"
	"os"
	"os/exec"
)

// CommandRunner executes shell commands, streaming stdout/stderr to the terminal
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type realRunner struct{}

func NewRealRunner() CommandRunner { return &realRunner{} }

func (r *realRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
