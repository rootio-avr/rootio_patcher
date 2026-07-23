package rpm

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestExecutor_Refresh_WithRefreshArgs(t *testing.T) {
	runner := &MockRunner{}
	e := NewExecutor(YumManager(), testLogger(), runner)

	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.Calls) != 1 || runner.Calls[0].Name != "yum" || len(runner.Calls[0].Args) != 1 || runner.Calls[0].Args[0] != "makecache" {
		t.Errorf("expected `yum makecache`, got %+v", runner.Calls)
	}
}

func TestExecutor_Refresh_NoRefreshArgsIsNoop(t *testing.T) {
	runner := &MockRunner{}
	e := NewExecutor(MicrodnfManager(), testLogger(), runner)

	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.Calls) != 0 {
		t.Errorf("expected no calls, got %+v", runner.Calls)
	}
}

func TestExecutor_UpgradeAll_Dnf(t *testing.T) {
	runner := &MockRunner{}
	e := NewExecutor(DnfManager(), testLogger(), runner)

	if err := e.UpgradeAll(context.Background(), []string{"curl", "bash"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.Calls) != 1 {
		t.Fatalf("expected 1 call, got %+v", runner.Calls)
	}
	call := runner.Calls[0]
	if call.Name != "dnf" {
		t.Errorf("expected binary dnf, got %s", call.Name)
	}
	wantArgs := []string{"upgrade", "-y", "curl", "bash"}
	if len(call.Args) != len(wantArgs) {
		t.Fatalf("got args %+v, want %+v", call.Args, wantArgs)
	}
	for i, a := range wantArgs {
		if call.Args[i] != a {
			t.Errorf("arg[%d] = %q, want %q", i, call.Args[i], a)
		}
	}
}

func TestExecutor_UpgradeAll_Microdnf_IncludesRefreshFlag(t *testing.T) {
	runner := &MockRunner{}
	e := NewExecutor(MicrodnfManager(), testLogger(), runner)

	if err := e.UpgradeAll(context.Background(), []string{"curl"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	call := runner.Calls[0]
	wantArgs := []string{"upgrade", "-y", "--refresh", "curl"}
	if len(call.Args) != len(wantArgs) {
		t.Fatalf("got args %+v, want %+v", call.Args, wantArgs)
	}
	for i, a := range wantArgs {
		if call.Args[i] != a {
			t.Errorf("arg[%d] = %q, want %q", i, call.Args[i], a)
		}
	}
}

func TestExecutor_UpgradeAll_NoNamesIsNoop(t *testing.T) {
	runner := &MockRunner{}
	e := NewExecutor(YumManager(), testLogger(), runner)

	if err := e.UpgradeAll(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.Calls) != 0 {
		t.Errorf("expected no calls, got %+v", runner.Calls)
	}
}
