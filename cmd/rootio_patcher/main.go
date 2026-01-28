package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/alecthomas/kong"

	"rootio_patcher/cmd/rootio_patcher/config"
	"rootio_patcher/cmd/rootio_patcher/maven"
	"rootio_patcher/cmd/rootio_patcher/npm"
	"rootio_patcher/cmd/rootio_patcher/pip"
)

var version = "dev"

// CLI defines the command-line interface
type CLI struct {
	Version kong.VersionFlag `short:"v" help:"Print version information"`

	Pip   PipCmd   `cmd:"" help:"Python/pip package remediation"`
	Npm   NpmCmd   `cmd:"" help:"npm package remediation"`
	Maven MavenCmd `cmd:"" help:"Maven package remediation"`
}

// PipCmd handles pip-related commands
type PipCmd struct {
	Remediate PipRemediateCmd `cmd:"" help:"Remediate Python packages (post-install patching)"`
}

// PipRemediateCmd remediates installed Python packages
type PipRemediateCmd struct {
	PythonPath string `default:"python" help:"Path to Python interpreter"`
	DryRun     bool   `default:"true" help:"Preview changes without applying them"`
	UseAlias   bool   `default:"true" help:"Use Root.io aliased packages"`
}

// NpmCmd handles npm-related commands
type NpmCmd struct {
	Remediate NpmRemediateCmd `cmd:"" help:"Remediate npm/yarn/pnpm packages (updates package.json with overrides)"`
}

// NpmRemediateCmd remediates npm packages by patching lock file and package.json
type NpmRemediateCmd struct {
	PackageManager string `default:"npm" enum:"npm,yarn,pnpm" help:"Package manager to use (npm, yarn, or pnpm)"`
	DryRun         bool   `default:"true" help:"Preview changes without applying them"`
}

// MavenCmd handles Maven-related commands
type MavenCmd struct {
	Remediate MavenRemediateCmd `cmd:"" help:"Remediate Maven packages (pre-install patching of pom.xml)"`
}

// MavenRemediateCmd remediates Maven packages by patching pom.xml
type MavenRemediateCmd struct {
	File   string `default:"pom.xml" help:"Path to pom.xml"`
	DryRun bool   `default:"true" help:"Preview changes without applying them"`
}

func main() {
	os.Exit(run())
}

func run() int {
	// Setup context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Parse CLI first (so --help works without env vars)
	var cli CLI
	kongCtx := kong.Parse(&cli,
		kong.Name("rootio_patcher"),
		kong.Description("Automated security patching for Python, npm, and Maven packages with Root.io"),
		kong.UsageOnError(),
		kong.Vars{"version": version},
		kong.BindTo(ctx, (*context.Context)(nil)), // Bind context with interface type
	)

	// Load configuration from environment variables (after parsing, before running)
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n✗ Failed to load environment configuration: %v\n", err)
		return 1
	}

	// Create logger with log level from config
	logger := createLogger(cfg.LogLevel)

	// Execute the selected command, passing cfg and logger
	if err := kongCtx.Run(cfg, logger); err != nil {
		fmt.Fprintf(os.Stderr, "\n✗ Error: %v\n", err)
		return 1
	}

	return 0
}

// createLogger creates a structured logger with the specified level
func createLogger(logLevelStr string) *slog.Logger {
	var logLevel slog.Level
	switch logLevelStr {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
}

// Run executes the pip remediate command
func (cmd *PipRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting pip remediation")

	app := pip.NewApp(cfg, cmd.PythonPath, cmd.DryRun, cmd.UseAlias, logger)
	return app.Run(ctx)
}

// Run executes the npm remediate command
func (cmd *NpmRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting npm remediation", slog.String("package_manager", cmd.PackageManager))

	app := npm.NewApp(cfg.APIKey, cfg.APIURL, cmd.PackageManager, cmd.DryRun, logger)
	return app.Run(ctx)
}

// Run executes the maven remediate command
func (cmd *MavenRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting Maven remediation", slog.String("file", cmd.File))

	app := maven.NewApp(cfg.APIKey, cfg.APIURL, cmd.File, cmd.DryRun, logger)
	return app.Run(ctx)
}
