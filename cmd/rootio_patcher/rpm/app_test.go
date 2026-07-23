package rpm

import (
	"context"
	"errors"
	"testing"
)

func TestApp_Run_NoPackagesFound(t *testing.T) {
	scanner := &MockScanner{ListPackagesFunc: func(ctx context.Context) ([]InstalledPackage, error) {
		return nil, nil
	}}
	runner := &MockRunner{}
	app := NewAppWithServices(YumManager(), false, nil, testLogger(), scanner, NewExecutor(YumManager(), testLogger(), runner))

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.Calls) != 0 {
		t.Errorf("expected no commands run, got %+v", runner.Calls)
	}
}

func TestApp_Run_ScanError(t *testing.T) {
	scanner := &MockScanner{ListPackagesFunc: func(ctx context.Context) ([]InstalledPackage, error) {
		return nil, errors.New("boom")
	}}
	app := NewAppWithServices(YumManager(), false, nil, testLogger(), scanner, NewExecutor(YumManager(), testLogger(), &MockRunner{}))

	if err := app.Run(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestApp_Run_DryRunDoesNotExecute(t *testing.T) {
	scanner := &MockScanner{ListPackagesFunc: func(ctx context.Context) ([]InstalledPackage, error) {
		return []InstalledPackage{{Name: "curl", Version: "8.5.0"}}, nil
	}}
	runner := &MockRunner{}
	app := NewAppWithServices(DnfManager(), true, nil, testLogger(), scanner, NewExecutor(DnfManager(), testLogger(), runner))

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.Calls) != 0 {
		t.Errorf("dry-run must not execute commands, got %+v", runner.Calls)
	}
}

func TestApp_Run_UpgradesAllInstalledPackages(t *testing.T) {
	scanner := &MockScanner{ListPackagesFunc: func(ctx context.Context) ([]InstalledPackage, error) {
		return []InstalledPackage{
			{Name: "curl", Version: "8.5.0"},
			{Name: "bash", Version: "5.1.8"},
		}, nil
	}}
	runner := &MockRunner{}
	app := NewAppWithServices(YumManager(), false, nil, testLogger(), scanner, NewExecutor(YumManager(), testLogger(), runner))

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.Calls) != 2 {
		t.Fatalf("expected refresh + upgrade calls, got %+v", runner.Calls)
	}
	if runner.Calls[0].Name != "yum" || runner.Calls[0].Args[0] != "makecache" {
		t.Errorf("expected first call to be `yum makecache`, got %+v", runner.Calls[0])
	}
	upgrade := runner.Calls[1]
	if upgrade.Name != "yum" || upgrade.Args[0] != "update" {
		t.Errorf("expected second call to be `yum update -y ...`, got %+v", upgrade)
	}
	wantNames := map[string]bool{"curl": true, "bash": true}
	for _, a := range upgrade.Args[2:] {
		if !wantNames[a] {
			t.Errorf("unexpected package name in upgrade args: %s", a)
		}
		delete(wantNames, a)
	}
	if len(wantNames) != 0 {
		t.Errorf("missing package names in upgrade args: %+v", wantNames)
	}
}

func TestApp_Run_IgnoredPackagesExcluded(t *testing.T) {
	scanner := &MockScanner{ListPackagesFunc: func(ctx context.Context) ([]InstalledPackage, error) {
		return []InstalledPackage{
			{Name: "curl", Version: "8.5.0"},
			{Name: "bash", Version: "5.1.8"},
		}, nil
	}}
	runner := &MockRunner{}
	ignoreSet := map[string]struct{}{"bash": {}}
	app := NewAppWithServices(YumManager(), false, ignoreSet, testLogger(), scanner, NewExecutor(YumManager(), testLogger(), runner))

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	upgrade := runner.Calls[1]
	for _, a := range upgrade.Args {
		if a == "bash" {
			t.Errorf("ignored package bash should not be upgraded, got args %+v", upgrade.Args)
		}
	}
}

func TestApp_Run_AllPackagesIgnoredSkipsExecution(t *testing.T) {
	scanner := &MockScanner{ListPackagesFunc: func(ctx context.Context) ([]InstalledPackage, error) {
		return []InstalledPackage{{Name: "curl", Version: "8.5.0"}}, nil
	}}
	runner := &MockRunner{}
	ignoreSet := map[string]struct{}{"curl": {}}
	app := NewAppWithServices(YumManager(), false, ignoreSet, testLogger(), scanner, NewExecutor(YumManager(), testLogger(), runner))

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.Calls) != 0 {
		t.Errorf("expected no commands run when all packages ignored, got %+v", runner.Calls)
	}
}

func TestApp_Run_IgnoreByExactVersionKey(t *testing.T) {
	scanner := &MockScanner{ListPackagesFunc: func(ctx context.Context) ([]InstalledPackage, error) {
		return []InstalledPackage{
			{Name: "curl", Version: "8.5.0"},
			{Name: "bash", Version: "5.1.8"},
		}, nil
	}}
	runner := &MockRunner{}
	ignoreSet := map[string]struct{}{"curl@8.5.0": {}}
	app := NewAppWithServices(YumManager(), false, ignoreSet, testLogger(), scanner, NewExecutor(YumManager(), testLogger(), runner))

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	upgrade := runner.Calls[1]
	for _, a := range upgrade.Args {
		if a == "curl" {
			t.Errorf("ignored package curl should not be upgraded, got args %+v", upgrade.Args)
		}
	}
}

func TestApp_Run_UpgradeErrorPropagates(t *testing.T) {
	scanner := &MockScanner{ListPackagesFunc: func(ctx context.Context) ([]InstalledPackage, error) {
		return []InstalledPackage{{Name: "curl", Version: "8.5.0"}}, nil
	}}
	runner := &MockRunner{RunFunc: func(ctx context.Context, name string, args ...string) error {
		if name == "yum" && len(args) > 0 && args[0] == "update" {
			return errors.New("upgrade failed")
		}
		return nil
	}}
	app := NewAppWithServices(YumManager(), false, nil, testLogger(), scanner, NewExecutor(YumManager(), testLogger(), runner))

	if err := app.Run(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}
