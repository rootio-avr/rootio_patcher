package pip

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/config"
	"rootio_patcher/pkg/rootio"
)

// fakePython writes an executable that records its argv and prints out to stdout.
func fakePython(t *testing.T, out string) (python, argvFile string) {
	t.Helper()
	dir := t.TempDir()
	argvFile = filepath.Join(dir, "argv")
	python = filepath.Join(dir, "python")
	script := "#!/bin/sh\necho \"$@\" > " + argvFile + "\ncat <<'EOF'\n" + out + "\nEOF\n"
	if err := os.WriteFile(python, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return python, argvFile
}

// F1: pip 20-24.0 prints its self-version-check notice to stdout after the JSON.
func TestListPackages_TrailingNoticeAfterJSON(t *testing.T) {
	out := `[{"name": "idna", "version": "3.4", "location": "/usr/local/lib/python3.8/site-packages", "installer": "pip"}]

[notice] A new release of pip is available: 23.0.1 -> 25.3
[notice] To update, run: pip install --upgrade pip
Looking in indexes: https://example.invalid/simple`
	python, argvFile := fakePython(t, out)
	s := NewService(python, "", "", "", slog.New(slog.NewTextHandler(io.Discard, nil)))

	pkgs, err := s.ListPackages(context.Background())
	if err != nil {
		t.Fatalf("ListPackages: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "idna" || pkgs[0].Installer == nil || *pkgs[0].Installer != "pip" {
		t.Fatalf("unexpected packages: %+v", pkgs)
	}
	argv, _ := os.ReadFile(argvFile)
	if !strings.Contains(string(argv), "--disable-pip-version-check") {
		t.Fatalf("pip list not run with --disable-pip-version-check: %s", argv)
	}
}

// pip 9 has no location/installer keys: Installer stays nil.
func TestListPackages_OldPipNoInstaller(t *testing.T) {
	python, _ := fakePython(t, `[{"name": "six", "version": "1.16.0"}]`)
	s := NewService(python, "", "", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	pkgs, err := s.ListPackages(context.Background())
	if err != nil || len(pkgs) != 1 || pkgs[0].Installer != nil {
		t.Fatalf("pkgs=%+v err=%v", pkgs, err)
	}
}

func strp(s string) *string { return &s }

// F2 + F3: only installer=="pip" is patched (with PIP_BREAK_SYSTEM_PACKAGES); everything else is
// skipped; no installer field at all (old pip) patches as v2.18.0 did, without the env var.
func TestApplyPatches_InstallerOwnership(t *testing.T) {
	installed := []common.InstalledPackage{
		{Name: "idna", Installer: strp("pip")},
		{Name: "six", Installer: strp("")},                  // apt (bookworm: no INSTALLER)
		{Name: "pip", Installer: strp("debian")},            // apt-owned pip
		{Name: "setuptools", Installer: strp("rpm")},        // rpm
		{Name: "cryptography", Installer: strp("")},         // apk writes no INSTALLER
		{Name: "attrs", Installer: strp("uv")},              // not pip
		{Name: "typing_extensions", Installer: strp("pip")}, // pip's spelling
		{Name: "certifi"},                                   // old pip: no installer key
	}
	var patches []rootio.PackagePatch
	for _, n := range []string{"idna", "six", "pip", "setuptools", "cryptography", "attrs", "typing-extensions", "certifi"} {
		patches = append(patches, rootio.PackagePatch{PackageName: n, Patch: rootio.PatchInfo{Name: n, Version: "1+aikido.1"}})
	}
	svc := &MockPipService{}
	app := NewAppWithServices(&config.Config{}, "python", false, nil, slog.New(slog.NewTextHandler(io.Discard, nil)),
		svc, &MockAPIClient{}, common.NewReporter("", slog.New(slog.NewTextHandler(io.Discard, nil))))

	patched, err := app.applyPatches(context.Background(), patches, installed)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"idna": true, "typing-extensions": true, "certifi": false}
	if patched != len(want) || len(svc.BreakSystem) != len(want) {
		t.Fatalf("patched=%d applied=%v, want %v", patched, svc.BreakSystem, want)
	}
	for n, brk := range want {
		if got, ok := svc.BreakSystem[n]; !ok || got != brk {
			t.Errorf("%s: applied=%v breakSystem=%v, want breakSystem=%v", n, ok, got, brk)
		}
	}
}

// F3: the summary line counts only what was patched and reports skips separately.
func TestRun_SummaryExcludesSkipped(t *testing.T) {
	svc := &MockPipService{ListPackagesFunc: func(context.Context) ([]common.InstalledPackage, error) {
		return []common.InstalledPackage{{Name: "idna", Installer: strp("pip")}, {Name: "six", Installer: strp("debian")}}, nil
	}}
	api := &MockAPIClient{AnalyzePackagesFunc: func(context.Context, []rootio.Package, []rootio.Package, string) (*rootio.AnalyzePackagesResponse, error) {
		return &rootio.AnalyzePackagesResponse{Patches: []rootio.PackagePatch{
			{PackageName: "idna", Patch: rootio.PatchInfo{Name: "idna", Version: "3.4+aikido.1"}},
			{PackageName: "six", Patch: rootio.PatchInfo{Name: "six", Version: "1.16.0+aikido.1"}},
		}}, nil
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := NewAppWithServices(&config.Config{}, "python", false, nil, logger, svc, api, common.NewReporter("", logger))

	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	err := app.Run(context.Background())
	os.Stdout = stdout
	w.Close()
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Successfully patched 1 packages! Skipped 1 (not pip-owned).") {
		t.Fatalf("summary wrong:\n%s", out)
	}
}
