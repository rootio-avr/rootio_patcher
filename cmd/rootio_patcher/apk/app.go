package apk

import (
	"context"
	"fmt"
	"log/slog"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/pkg/rootio"
)

// APIClient is an alias for common.OsAPIClient
type APIClient = common.OsAPIClient

// App orchestrates the apk remediate workflow
type App struct {
	inner *common.OsApp[OSInfo]
}

func NewApp(apiKey, apiURL, pkgURL string, dryRun, useAlias, verbose, skipUpgrades bool, ignoreSet map[string]struct{}, logger *slog.Logger) *App {
	client := rootio.NewClient(apiURL, apiKey)
	executor := NewExecutor(apiKey, pkgURL, logger, NewRealRunner())
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
	return &App{inner: common.NewOsApp(
		pkgURL, dryRun, useAlias, skipUpgrades, ignoreSet, logger,
		scanner, apiClient, executor,
		apkConfig(),
	)}
}

func (a *App) Run(ctx context.Context) error { return a.inner.Run(ctx) }

func apkConfig() common.OsAppConfig[OSInfo] {
	return common.OsAppConfig[OSInfo]{
		Name:          "apk",
		NoPackagesMsg: "No packages found via apk info -v",
		UpdateMsg:     "\nUpdating package index...",
		UpdateErrMsg:  "apk update failed",
		SetupMsg:      "\nSetting up Root.io APK repository...",
		CleanupMsg:    "\nCleaning up Root.io APK repository...",
		// PackageBlacklist: packages never offered for the broad upstream upgrade.
		// Keep in sync with backend/.../os/alpine.go.
		PackageBlacklist: map[string]bool{"alpine-baselayout": true},
		GetAPIParams: func(o *OSInfo) (endpoint, ecosystem, distroVersion string) {
			return "apk", "alpine", o.DistroVersion
		},
		GetRegistryURL: func(pkgURL string, o *OSInfo) string {
			return fmt.Sprintf("%s/alpine/%s", pkgURL, o.DistroVersion)
		},
		LogOsInfo: func(ctx context.Context, logger *slog.Logger, o *OSInfo) {
			logger.DebugContext(ctx, "Detected OS", slog.String("distro_version", o.DistroVersion))
		},
	}
}
