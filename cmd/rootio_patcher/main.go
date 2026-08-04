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

	"rootio_patcher/cmd/rootio_patcher/apk"
	"rootio_patcher/cmd/rootio_patcher/apt"
	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/composer"
	"rootio_patcher/cmd/rootio_patcher/config"
	"rootio_patcher/cmd/rootio_patcher/golang"
	"rootio_patcher/cmd/rootio_patcher/maven"
	"rootio_patcher/cmd/rootio_patcher/npm"
	"rootio_patcher/cmd/rootio_patcher/nuget"
	"rootio_patcher/cmd/rootio_patcher/pip"
	"rootio_patcher/cmd/rootio_patcher/rpm"
	"rootio_patcher/pkg/rootio"
)

var version = "dev"

// CLI defines the command-line interface
type CLI struct {
	Version kong.VersionFlag `short:"v" help:"Print version information"`

	Pip      PipCmd      `cmd:"" help:"Python/pip package remediation"`
	Npm      NpmCmd      `cmd:"" help:"npm package remediation"`
	Maven    MavenCmd    `cmd:"" help:"Maven package remediation"`
	Go       GoCmd       `cmd:"" help:"Go module remediation"`
	Nuget    NuGetCmd    `cmd:"" help:"NuGet package remediation"`
	Composer ComposerCmd `cmd:"" help:"Composer (PHP) package remediation"`
	Apt      AptCmd      `cmd:"" help:"APT (Debian/Ubuntu) OS-level package remediation"`
	Apk      ApkCmd      `cmd:"" help:"APK (Alpine Linux) OS-level package remediation"`
	Yum      YumCmd      `cmd:"" help:"yum (RHEL/CentOS) OS-level package upgrade"`
	Dnf      DnfCmd      `cmd:"" help:"dnf (Fedora/RHEL) OS-level package upgrade"`
	Microdnf MicrodnfCmd `cmd:"" help:"microdnf (minimal RHEL-family images) OS-level package upgrade"`
}

// AptCmd handles APT-related commands
type AptCmd struct {
	Remediate AptRemediateCmd `cmd:"" help:"Remediate Debian/Ubuntu OS packages (post-install patching)"`
}

// AptRemediateCmd remediates installed APT packages
type AptRemediateCmd struct {
	DryRun       bool     `default:"true" help:"Preview changes without applying them"`
	UseAlias     bool     `default:"true" help:"Use Root.io aliased packages (rootio-*); set false to install under original names"`
	Verbose      bool     `default:"false" help:"Print each remediation step"`
	SkipUpgrades bool     `default:"false" help:"Skip the broad upstream upgrade; apply Root patches only"`
	Ignore       []string `help:"Ignore package@version (repeatable). Also merged with .rootioignore file." name:"ignore" sep:","`
}

// Run executes the apt remediate command
func (cmd *AptRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting apt remediation", slog.Bool("dry_run", cmd.DryRun), slog.Bool("use_alias", cmd.UseAlias))

	ignoreSet := common.LoadIgnoreList(".rootioignore", cmd.Ignore)
	app := apt.NewApp(cfg.APIKey, cfg.APIURL, cfg.PKGURL, cmd.DryRun, cmd.UseAlias, cmd.Verbose, cmd.SkipUpgrades, ignoreSet, logger)
	return app.Run(ctx)
}

// ApkCmd handles APK-related commands
type ApkCmd struct {
	Remediate ApkRemediateCmd `cmd:"" help:"Remediate Alpine Linux OS packages (post-install patching)"`
}

// ApkRemediateCmd remediates installed APK packages
type ApkRemediateCmd struct {
	DryRun       bool     `default:"true" help:"Preview changes without applying them"`
	UseAlias     bool     `default:"true" help:"Use Root.io aliased packages (rootio-*); set false to install under original names"`
	Verbose      bool     `default:"false" help:"Print each remediation step"`
	SkipUpgrades bool     `default:"false" help:"Skip the broad upstream upgrade; apply Root patches only"`
	Ignore       []string `help:"Ignore package@version (repeatable). Also merged with .rootioignore file." name:"ignore" sep:","`
}

// Run executes the apk remediate command
func (cmd *ApkRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting apk remediation", slog.Bool("dry_run", cmd.DryRun), slog.Bool("use_alias", cmd.UseAlias))

	ignoreSet := common.LoadIgnoreList(".rootioignore", cmd.Ignore)
	app := apk.NewApp(cfg.APIKey, cfg.APIURL, cfg.PKGURL, cmd.DryRun, cmd.UseAlias, cmd.Verbose, cmd.SkipUpgrades, ignoreSet, logger)
	return app.Run(ctx)
}

// YumCmd handles yum-related commands
type YumCmd struct {
	Remediate YumRemediateCmd `cmd:"" help:"Upgrade installed RHEL/CentOS packages to their latest version via yum"`
}

// YumRemediateCmd upgrades installed yum packages to the latest version.
// Root.io has no targeted patches for yum, so this only performs the broad
// upstream upgrade — it never calls the analyze API or installs patches.
type YumRemediateCmd struct {
	DryRun bool     `default:"true" help:"Preview changes without applying them"`
	Ignore []string `help:"Ignore package@version (repeatable). Also merged with .rootioignore file." name:"ignore" sep:","`
}

// Run executes the yum remediate command
func (cmd *YumRemediateCmd) Run(ctx context.Context, _ *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting yum remediation", slog.Bool("dry_run", cmd.DryRun))

	ignoreSet := common.LoadIgnoreList(".rootioignore", cmd.Ignore)
	app := rpm.NewApp(rpm.YumManager(), cmd.DryRun, ignoreSet, logger)
	return app.Run(ctx)
}

// DnfCmd handles dnf-related commands
type DnfCmd struct {
	Remediate DnfRemediateCmd `cmd:"" help:"Upgrade installed Fedora/RHEL packages to their latest version via dnf"`
}

// DnfRemediateCmd upgrades installed dnf packages to the latest version.
// Root.io has no targeted patches for dnf, so this only performs the broad
// upstream upgrade — it never calls the analyze API or installs patches.
type DnfRemediateCmd struct {
	DryRun bool     `default:"true" help:"Preview changes without applying them"`
	Ignore []string `help:"Ignore package@version (repeatable). Also merged with .rootioignore file." name:"ignore" sep:","`
}

// Run executes the dnf remediate command
func (cmd *DnfRemediateCmd) Run(ctx context.Context, _ *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting dnf remediation", slog.Bool("dry_run", cmd.DryRun))

	ignoreSet := common.LoadIgnoreList(".rootioignore", cmd.Ignore)
	app := rpm.NewApp(rpm.DnfManager(), cmd.DryRun, ignoreSet, logger)
	return app.Run(ctx)
}

// MicrodnfCmd handles microdnf-related commands
type MicrodnfCmd struct {
	Remediate MicrodnfRemediateCmd `cmd:"" help:"Upgrade installed packages to their latest version via microdnf"`
}

// MicrodnfRemediateCmd upgrades installed microdnf packages to the latest version.
// Root.io has no targeted patches for microdnf, so this only performs the broad
// upstream upgrade — it never calls the analyze API or installs patches.
type MicrodnfRemediateCmd struct {
	DryRun bool     `default:"true" help:"Preview changes without applying them"`
	Ignore []string `help:"Ignore package@version (repeatable). Also merged with .rootioignore file." name:"ignore" sep:","`
}

// Run executes the microdnf remediate command
func (cmd *MicrodnfRemediateCmd) Run(ctx context.Context, _ *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting microdnf remediation", slog.Bool("dry_run", cmd.DryRun))

	ignoreSet := common.LoadIgnoreList(".rootioignore", cmd.Ignore)
	app := rpm.NewApp(rpm.MicrodnfManager(), cmd.DryRun, ignoreSet, logger)
	return app.Run(ctx)
}

// PipCmd handles pip-related commands
type PipCmd struct {
	Remediate PipRemediateCmd `cmd:"" help:"Remediate Python packages (post-install patching)"`
}

// PipRemediateCmd remediates installed Python packages
type PipRemediateCmd struct {
	PythonPath string   `default:"python" help:"Path to Python interpreter"`
	DryRun     bool     `default:"true" help:"Preview changes without applying them"`
	UseAlias   bool     `default:"true" help:"Use Root.io aliased packages"`
	Ignore     []string `help:"Ignore package@version (repeatable). Also merged with .rootioignore file." name:"ignore" sep:","`
}

// NpmCmd handles npm-related commands
type NpmCmd struct {
	Remediate NpmRemediateCmd `cmd:"" help:"Remediate npm/yarn/pnpm packages (updates package.json with overrides)"`
}

// NpmRemediateCmd remediates npm packages by patching lock file and package.json
type NpmRemediateCmd struct {
	PackageManager string   `default:"npm" enum:"npm,yarn,pnpm" help:"Package manager to use (npm, yarn, or pnpm)"`
	Directory      string   `default:"." short:"C" help:"Project directory containing the lock file and package.json (defaults to current directory)"`
	DryRun         bool     `default:"true" help:"Preview changes without applying them"`
	Ignore         []string `help:"Ignore package@version (repeatable). Also merged with .rootioignore file." name:"ignore" sep:","`
}

// MavenCmd handles Maven-related commands
type MavenCmd struct {
	Remediate MavenRemediateCmd `cmd:"" help:"Remediate Maven packages (pre-install patching of pom.xml)"`
}

// MavenRemediateCmd remediates Maven packages by patching pom.xml
type MavenRemediateCmd struct {
	File     string   `default:"pom.xml" help:"Path to pom.xml"`
	DryRun   bool     `default:"true" help:"Preview changes without applying them"`
	UseAlias bool     `default:"true" help:"Use Root.io aliased packages (io.root.io.*); set false to keep original groupId"`
	Ignore   []string `help:"Ignore package@version (repeatable). Also merged with .rootioignore file." name:"ignore" sep:","`
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

	app := pip.NewApp(cfg, cmd.PythonPath, cmd.DryRun, cmd.UseAlias, cmd.Ignore, logger)
	return app.Run(ctx)
}

// Run executes the npm remediate command
func (cmd *NpmRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting npm remediation",
		slog.String("package_manager", cmd.PackageManager))

	dir, err := filepath.Abs(cmd.Directory)
	if err != nil {
		return fmt.Errorf("invalid directory: %w", err)
	}

	app := npm.NewApp(cfg.APIKey, cfg.APIURL, cmd.PackageManager, dir, cmd.DryRun, cmd.Ignore, logger)
	return app.Run(ctx)
}

// Run executes the maven remediate command
func (cmd *MavenRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting Maven remediation", slog.String("file", cmd.File), slog.Bool("use_alias", cmd.UseAlias))

	app := maven.NewApp(cfg.APIKey, cfg.APIURL, cmd.File, cmd.DryRun, cmd.UseAlias, cmd.Ignore, logger)
	return app.Run(ctx)
}

// GoCmd handles Go module commands
type GoCmd struct {
	Remediate GoRemediateCmd `cmd:"" help:"Remediate Go modules (pre-build patching of go.mod with replace directives)"`
}

// GoRemediateCmd remediates Go modules by adding replace directives to go.mod
type GoRemediateCmd struct {
	GoMod    string   `default:"go.mod" help:"Path to go.mod"`
	DryRun   bool     `default:"true" help:"Preview changes without applying them"`
	UseAlias bool     `default:"true" help:"Use Root.io aliased modules (pkg.root.io/*); set false to use original module paths"`
	Ignore   []string `help:"Ignore package@version (repeatable). Also merged with .rootioignore file." name:"ignore" sep:","`
}

// Run executes the go remediate command
func (cmd *GoRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting Go module remediation", slog.String("go_mod", cmd.GoMod), slog.Bool("use_alias", cmd.UseAlias))

	app := golang.NewApp(
		cfg.APIKey, cfg.APIURL, cfg.PKGURL, cmd.GoMod, cmd.DryRun, cmd.UseAlias, cmd.Ignore, logger,
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
	File      string   `help:"Path to a specific .csproj or packages.config file (overrides --directory)"`
	Directory string   `default:"." short:"C" help:"Project directory to auto-discover NuGet manifests (default: current directory)"`
	DryRun    bool     `default:"true" help:"Preview changes without applying them"`
	UseAlias  bool     `default:"true" help:"Use Root.io aliased packages; set false to keep original package names"`
	Ignore    []string `help:"Ignore package@version (repeatable). Also merged with .rootioignore file." name:"ignore" sep:","`
}

// ComposerCmd handles Composer-related commands
type ComposerCmd struct {
	Remediate ComposerRemediateCmd `cmd:"" help:"Remediate Composer packages (pre-install patching of composer.json)"`
}

// ComposerRemediateCmd remediates Composer packages by patching composer.json
type ComposerRemediateCmd struct {
	File     string   `default:"composer.json" help:"Path to composer.json"`
	DryRun   bool     `default:"true" help:"Preview changes without applying them"`
	UseAlias bool     `default:"false" help:"Use Root.io aliased packages"`
	Ignore   []string `help:"Ignore package@version (repeatable). Also merged with .rootioignore file." name:"ignore" sep:","`
}

// Run executes the composer remediate command
func (cmd *ComposerRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting Composer remediation", slog.String("file", cmd.File))

	filePath, err := filepath.Abs(cmd.File)
	if err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	app := composer.NewApp(cfg.APIKey, cfg.APIURL, cfg.PKGURL, filePath, cmd.DryRun, cmd.UseAlias, cmd.Ignore, logger)
	return app.Run(ctx)
}

// Run executes the nuget remediate command
func (cmd *NuGetRemediateCmd) Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "Starting NuGet remediation", slog.Bool("use_alias", cmd.UseAlias))

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

	app := nuget.NewApp(cfg.APIKey, cfg.APIURL, path, cmd.DryRun, cmd.UseAlias, cmd.Ignore, logger)
	return app.Run(ctx)
}
