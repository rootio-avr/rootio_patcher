package main

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// parseCLI parses args without letting kong exit the test process.
func parseCLI(t *testing.T, args ...string) (*CLI, *kong.Context, error) {
	t.Helper()

	var cli CLI
	var out strings.Builder
	parser, err := kong.New(&cli,
		kong.Name("rootio_patcher"),
		kong.Vars{"version": "test"},
		kong.Writers(&out, &out),
		kong.Exit(func(int) { t.Fatalf("kong exited during parse of %v: %s", args, out.String()) }),
	)
	if err != nil {
		t.Fatalf("building parser: %v", err)
	}

	kongCtx, err := parser.Parse(args)
	return &cli, kongCtx, err
}

// The --use-alias flag was removed from pip and npm remediation (alias mode is
// gone), but pipelines still pass it. Accept and ignore it instead of failing
// with a usage error.
func TestDeprecatedUseAliasIsAccepted(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"pip bare", []string{"pip", "remediate", "--use-alias"}},
		{"pip false", []string{"pip", "remediate", "--use-alias=false"}},
		{"pip true", []string{"pip", "remediate", "--use-alias=true"}},
		{"npm bare", []string{"npm", "remediate", "--use-alias"}},
		{"npm false", []string{"npm", "remediate", "--use-alias=false"}},
		{"npm true", []string{"npm", "remediate", "--use-alias=true"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := parseCLI(t, tc.args...); err != nil {
				t.Fatalf("parsing %v: unexpected error: %v", tc.args, err)
			}
		})
	}
}

// Other flags must still be honored when --use-alias is present, and the
// deprecated value must not leak into behavior.
func TestDeprecatedUseAliasDoesNotAffectOtherFlags(t *testing.T) {
	cli, _, err := parseCLI(t, "npm", "remediate", "--use-alias=true", "--package-manager=pnpm", "--dry-run=false")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cli.Npm.Remediate.PackageManager; got != "pnpm" {
		t.Errorf("PackageManager = %q, want %q", got, "pnpm")
	}
	if cli.Npm.Remediate.DryRun {
		t.Error("DryRun = true, want false")
	}
	if cli.Npm.Remediate.UseAlias == nil || !*cli.Npm.Remediate.UseAlias {
		t.Error("UseAlias not recorded; the deprecation warning cannot be emitted")
	}
}

// Not passing the flag must leave it unset, so no spurious deprecation warning
// is emitted for the common case.
func TestUseAliasUnsetWhenOmitted(t *testing.T) {
	cli, _, err := parseCLI(t, "pip", "remediate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cli.Pip.Remediate.UseAlias != nil {
		t.Errorf("UseAlias = %v, want nil when the flag is omitted", *cli.Pip.Remediate.UseAlias)
	}
}

// The deprecated flag should not be advertised in help output.
func TestDeprecatedUseAliasHiddenFromHelp(t *testing.T) {
	var cli CLI
	var out strings.Builder
	parser, err := kong.New(&cli,
		kong.Name("rootio_patcher"),
		kong.Vars{"version": "test"},
		kong.Writers(&out, &out),
		kong.Exit(func(int) {}),
	)
	if err != nil {
		t.Fatalf("building parser: %v", err)
	}

	if _, err := parser.Parse([]string{"pip", "remediate", "--help"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out.String(), "use-alias") {
		t.Errorf("help output mentions the deprecated flag:\n%s", out.String())
	}
}
