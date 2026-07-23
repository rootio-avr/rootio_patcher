package rpm

import (
	"context"
	"log/slog"
	"strings"

	"rootio_patcher/cmd/rootio_patcher/common"
)

// CommandRunner is an alias for common.CommandRunner
type CommandRunner = common.CommandRunner

func NewRealRunner() CommandRunner { return common.NewRealRunner() }

// Executor performs the refresh/upgrade steps for a given Manager
type Executor struct {
	manager Manager
	runner  CommandRunner
	logger  *slog.Logger
}

func NewExecutor(manager Manager, logger *slog.Logger, runner CommandRunner) *Executor {
	return &Executor{manager: manager, runner: runner, logger: logger}
}

// Refresh refreshes the package metadata cache, if the manager supports a
// separate refresh step.
func (e *Executor) Refresh(ctx context.Context) error {
	if len(e.manager.RefreshArgs) == 0 {
		return nil
	}
	e.logger.Debug(e.manager.Name + " refresh")
	return e.runner.Run(ctx, e.manager.Binary, e.manager.RefreshArgs...)
}

// UpgradeAll upgrades the named packages to the latest available version.
func (e *Executor) UpgradeAll(ctx context.Context, names []string) error {
	if len(names) == 0 {
		return nil
	}
	e.logger.Debug("upgrading packages", "packages", strings.Join(names, " "))
	return e.runner.Run(ctx, e.manager.Binary, e.manager.UpgradeArgs(names)...)
}
