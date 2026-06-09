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
	inner *common.OsApp[OSInfo]
}

func NewApp(apiKey, apiURL, pkgURL string, dryRun, useAlias, verbose bool, logger *slog.Logger) *App {
	client := rootio.NewClient(apiURL, apiKey)
	executor := NewExecutor(apiKey, pkgURL, verbose, NewRealRunner())
	return NewAppWithServices(apiKey, pkgURL, dryRun, useAlias, verbose, logger, NewScanner(), client, executor)
}

func NewAppWithServices(
	apiKey, pkgURL string,
	dryRun, useAlias, verbose bool,
	logger *slog.Logger,
	scanner Scanner,
	apiClient APIClient,
	executor *Executor,
) *App {
	return &App{inner: common.NewOsApp(
		pkgURL, dryRun, useAlias, logger,
		scanner, apiClient, executor,
		aptConfig(),
	)}
}

func (a *App) Run(ctx context.Context) error { return a.inner.Run(ctx) }

func aptConfig() common.OsAppConfig[OSInfo] {
	return common.OsAppConfig[OSInfo]{
		Name:          "apt",
		NoPackagesMsg: "No packages found via dpkg-query",
		UpdateMsg:     "\nUpdating package lists...",
		UpdateErrMsg:  "apt-get update failed",
		SetupMsg:      "\nSetting up Root.io APT repository...",
		CleanupMsg:    "\nCleaning up Root.io APT repository...",
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
