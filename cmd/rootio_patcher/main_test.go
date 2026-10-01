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

// --use-alias is not a flag on any command; kong must reject it.
func TestUseAliasRejected(t *testing.T) {
	for _, ecosystem := range []string{"apt", "apk", "pip", "npm", "maven", "go", "nuget", "composer"} {
		t.Run(ecosystem, func(t *testing.T) {
			if _, _, err := parseCLI(t, ecosystem, "remediate", "--use-alias"); err == nil {
				t.Fatalf("%s remediate --use-alias: expected unknown flag error", ecosystem)
			}
		})
	}
}
