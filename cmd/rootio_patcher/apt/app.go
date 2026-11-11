package apt

import (
	"context"
	"fmt"
	"log/slog"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// APIClient is an alias for common.OsAPIClient
type APIClient = common.OsAPIClient

// App orchestrates the apt remediate workflow
type App struct {
	inner    *common.OsApp[OSInfo]
	executor *Executor
}

func NewApp(apiKey, apiURL, pkgURL string, dryRun, useAlias, verbose, skipUpgrades bool, ignoreSet map[string]struct{}, logger *slog.Logger) *App {
	client := rootio.NewClient(apiURL, apiKey)
	executor := NewExecutor(apiKey, pkgURL, verbose, NewRealRunner())
	return NewAppWithServices(apiKey, pkgURL, dryRun, useAlias, verbose, skipUpgrades, ignoreSet, logger, NewScanner(), client, executor)
}

func NewAppWithServices(
	apiKey, pkgURL string,
	dryRun, useAlias, verbose, skipUpgrades bool,
	ignoreSet map[string]struct{},
	logger *slog.Logger,
	scanner Scanner,
	apiClient APIClient,
	executor *Executor,
) *App {
	executor.logger = logger
	return &App{inner: common.NewOsApp(
		pkgURL, dryRun, useAlias, skipUpgrades, ignoreSet, logger,
		scanner, apiClient, executor,
		aptConfig(),
	), executor: executor}
}

// Run runs the remediation, then prints the HELD_SKIPPED summary line when any
// patch was skipped because of a held package. Skips do not change the exit code.
func (a *App) Run(ctx context.Context) error {
	err := a.inner.Run(ctx)
	if line := a.executor.HeldSummary(); line != "" {
		fmt.Println(line)
	}
	return err
}

func aptConfig() common.OsAppConfig[OSInfo] {
	return common.OsAppConfig[OSInfo]{
		Name:          "apt",
		NoPackagesMsg: "No packages found via dpkg-query",
		UpdateMsg:     "\nUpdating package lists...",
		UpdateErrMsg:  "apt-get update failed",
		SetupMsg:      "\nSetting up Root.io APT repository...",
		CleanupMsg:    "\nCleaning up Root.io APT repository...",
		// PackageBlacklist: packages never offered for the broad upstream upgrade.
		// Empty for apt for now.
		PackageBlacklist: map[string]bool{},
		GetAPIParams: func(o *OSInfo) (endpoint, ecosystem, distroVersion string) {
			return "apt", o.Ecosystem, o.DistroVersion
		},
		GetRegistryURL: func(pkgURL string, o *OSInfo) string {
			return fmt.Sprintf("%s/%s/%s", pkgURL, o.Ecosystem, o.Codename)
		},
		LogOsInfo: func(ctx context.Context, logger *slog.Logger, o *OSInfo) {
			logger.DebugContext(ctx, "Detected OS",
				slog.String("ecosystem", o.Ecosystem),
				slog.String("distro_version", o.DistroVersion),
				slog.String("codename", o.Codename),
			)
		},
	}
}
