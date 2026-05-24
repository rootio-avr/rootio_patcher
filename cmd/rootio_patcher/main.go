package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/alecthomas/kong"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/config"
	"rootio_patcher/cmd/rootio_patcher/golang"
	"rootio_patcher/cmd/rootio_patcher/maven"
	"rootio_patcher/cmd/rootio_patcher/npm"
	"rootio_patcher/cmd/rootio_patcher/nuget"
	"rootio_patcher/cmd/rootio_patcher/pip"
	"rootio_patcher/pkg/rootio"
)

var version = "dev"

// CLI defines the command-line interface
type CLI struct {
	Version kong.VersionFlag `short:"v" help:"Print version information"`

	Pip   PipCmd   `cmd:"" help:"Python/pip package remediation"`
	Npm   NpmCmd   `cmd:"" help:"npm package remediation"`
	Maven MavenCmd `cmd:"" help:"Maven package remediation"`
	Go    GoCmd    `cmd:"" help:"Go module remediation"`
	Nuget NuGetCmd `cmd:"" help:"NuGet package remediation"`
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
	Directory      string `default:"." short:"C" help:"Project directory containing the lock file and package.json (defaults to current directory)"`
	DryRun         bool   `default:"true" help:"Preview changes without applying them"`
	UseAlias       bool   `default:"true" help:"Use Root.io aliased packages (@rootio/pkg) instead of original package names"`
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
		kong.Description("Automated security patching for Python, npm, go, nuget and Maven packages with Root.io"),
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
		if errors.Is(err, common.ErrPatchesAvailable) {
			// Dry-run found patches: signal to CI that changes are needed
			return 2
		}
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
	logger.InfoContext(ctx, "Starting npm remediation",
		slog.String("package_manager", cmd.PackageManager),
		slog.Bool("use_alias", cmd.UseAlias))

	dir, err := filepath.Abs(cmd.Directory)
	if err != nil {
		return fmt.Errorf("invalid directory: %w", err)
	}

	app := npm.NewApp(cfg.APIKey, cfg.APIURL, cmd.PackageManager, dir, cmd.DryRun, cmd.UseAlias, logger)
	return app.Run(ctx)
}

// Run executes the maven remediate command
func (cmd *MavenRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting Maven remediation", slog.String("file", cmd.File))

	app := maven.NewApp(cfg.APIKey, cfg.APIURL, cmd.File, cmd.DryRun, logger)
	return app.Run(ctx)
}

// GoCmd handles Go module commands
type GoCmd struct {
	Remediate GoRemediateCmd `cmd:"" help:"Remediate Go modules (pre-build patching of go.mod with replace directives)"`
}

// GoRemediateCmd remediates Go modules by adding replace directives to go.mod
type GoRemediateCmd struct {
	GoMod  string `default:"go.mod" help:"Path to go.mod"`
	DryRun bool   `default:"true" help:"Preview changes without applying them"`
}

// Run executes the go remediate command
func (cmd *GoRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting Go module remediation", slog.String("go_mod", cmd.GoMod))

	app := golang.NewApp(
		cfg.APIKey, cfg.APIURL, cfg.PKGURL, cmd.GoMod, cmd.DryRun, logger,
		golang.NewGoModParser(logger),
		rootio.NewClient(cfg.APIURL, cfg.APIKey),
		golang.NewRealCommandRunner(),
	)
	return app.Run(ctx)
}

// NuGetCmd handles NuGet-related commands
type NuGetCmd struct {
	Remediate NuGetRemediateCmd `cmd:"" help:"Remediate NuGet packages (updates .csproj or packages.config in place)"`
}

// NuGetRemediateCmd remediates NuGet packages by patching manifest files
type NuGetRemediateCmd struct {
	File      string `help:"Path to a specific .csproj or packages.config file (overrides --directory)"`
	Directory string `default:"." short:"C" help:"Project directory to auto-discover NuGet manifests (default: current directory)"`
	DryRun    bool   `default:"true" help:"Preview changes without applying them"`
}

// Run executes the nuget remediate command
func (cmd *NuGetRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting NuGet remediation")

	var path string
	var err error
	if cmd.File != "" {
		path, err = filepath.Abs(cmd.File)
		if err != nil {
			return fmt.Errorf("invalid file path: %w", err)
		}
	} else {
		path, err = filepath.Abs(cmd.Directory)
		if err != nil {
			return fmt.Errorf("invalid directory: %w", err)
		}
	}

	app := nuget.NewApp(cfg.APIKey, cfg.APIURL, path, cmd.DryRun, logger)
	return app.Run(ctx)
}
