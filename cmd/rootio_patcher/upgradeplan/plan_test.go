package upgradeplan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"rootio_patcher/pkg/rootio"
)

func planned(name, from, to string) rootio.PackagePatch {
	info := rootio.PatchInfo{Name: name, Version: to}
	return rootio.PackagePatch{PackageName: name, Version: from, Patch: info, PatchAlias: info, Planned: true}
}

func split(t *testing.T, plan *Plan, ecosystem string, installed []Package) Selection {
	t.Helper()
	got, err := plan.Split(ecosystem, installed)
	require.NoError(t, err)
	return got
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultFileName)

	t.Run("example", func(t *testing.T) {
		body := `{"packages":[
			{"ecosystem":"npm","name":"lodash","action":"upgrade","version":"4.17.21","reason":"safe"},
			{"ecosystem":"npm","name":"ms","from":"2.0.0","action":"upgrade","version":"2.2.2+root.io.1"}]}`
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		plan, err := Load(path)
		require.NoError(t, err)
		require.Equal(t, []Entry{
			{Ecosystem: "npm", Name: "lodash", Action: ActionUpgrade, Version: "4.17.21", Reason: "safe"},
			{Ecosystem: "npm", Name: "ms", From: "2.0.0", Action: ActionUpgrade, Version: "2.2.2+root.io.1"},
		}, plan.Packages)
	})
	t.Run("missing", func(t *testing.T) {
		_, err := Load(filepath.Join(dir, "missing.json"))
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("invalid", func(t *testing.T) {
		require.NoError(t, os.WriteFile(path, []byte("{"), 0o644))
		_, err := Load(path)
		require.Error(t, err)
	})
	t.Run("empty", func(t *testing.T) {
		require.NoError(t, os.WriteFile(path, []byte(" \n"), 0o644))
		_, err := Load(path)
		require.Error(t, err)
	})
}

func TestSplit_Errors(t *testing.T) {
	cases := []struct {
		name  string
		entry Entry
		want  string
	}{
		{"unknown action", Entry{Ecosystem: "npm", Name: "lodash", Action: "pin", Version: "1.0.0"}, `upgrade plan package "lodash": action "pin" is not supported, use "upgrade"`},
		{"leave", Entry{Ecosystem: "npm", Name: "lodash", Action: "leave"}, `upgrade plan package "lodash": action "leave" is not supported, use "upgrade"`},
		{"missing version", Entry{Ecosystem: "npm", Name: "lodash", Action: "upgrade"}, `upgrade plan package "lodash": upgrade requires a version`},
		{"empty name", Entry{Ecosystem: "npm", Name: "  ", Action: "upgrade", Version: "1"}, "upgrade plan entry 0: package name is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&Plan{Packages: []Entry{tc.entry}}).Split("npm", nil)
			require.EqualError(t, err, tc.want)
		})
	}
	_, err := (*Plan)(nil).Split("npm", nil)
	require.Error(t, err)
}

func TestSplit(t *testing.T) {
	installed := []Package{
		{Name: "lodash", Version: "4.17.20"},
		{Name: "ms", Version: "2.0.0"},
		{Name: "express", Version: "4.18.2"},
		{Name: "requests", Version: "2.0.0"},
	}
	got := split(t, &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21-aikido.2"},
		{Ecosystem: "npm", Name: "ms", Action: "upgrade", Version: "2.1.3", Reason: "safe"},
		{Ecosystem: "pypi", Name: "requests", Action: "upgrade", Version: "2.32.0"},
		{Ecosystem: "npm", Name: "absent", Action: "upgrade", Version: "1.0.0"},
		{Ecosystem: "maven", Name: "org.example:lib", Action: "pin"},
	}}, "npm", installed)
	require.Equal(t, []rootio.PackagePatch{
		planned("lodash", "4.17.20", "4.17.21-aikido.2"),
		planned("ms", "2.0.0", "2.1.3"),
	}, got.Patches)
	require.Equal(t, []rootio.Package{{Name: "lodash", Version: "4.17.20"}, {Name: "ms", Version: "2.0.0"}}, got.Ignore)
	require.Equal(t, []Unmatched{{Name: "absent"}}, got.Absent)

	got = split(t, &Plan{Packages: []Entry{{Ecosystem: "NPM", Name: "lodash", Action: "Upgrade", Version: " 4.17.21 "}}}, " Npm ", []Package{{Name: "lodash", Version: "4.17.20"}})
	require.Equal(t, "4.17.21", got.Patches[0].Patch.Version)

	got = split(t, &Plan{}, "npm", installed)
	require.Empty(t, got.Patches)
	require.Empty(t, got.Ignore)
	require.Empty(t, got.Absent)

	got = split(t, &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "ms", Action: "upgrade", Version: "2.1.3"}}}, "npm", []Package{{Name: "ms", Version: "2.1.3"}})
	require.Empty(t, got.Patches)
	require.Equal(t, []rootio.Package{{Name: "ms", Version: "2.1.3"}}, got.Ignore)
	require.Equal(t, []string{"ms"}, got.Current)

	// Last duplicate entry wins. A repeated installed row is planned once.
	got = split(t, &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.20"},
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21"},
	}}, "npm", []Package{{Name: "lodash", Version: "4.17.15"}, {Name: "lodash", Version: "4.17.15"}, {Name: "lodash", Version: "3.10.1"}})
	require.Equal(t, []rootio.PackagePatch{
		planned("lodash", "4.17.15", "4.17.21"),
		planned("lodash", "3.10.1", "4.17.21"),
	}, got.Patches)
}

func TestSplit_From(t *testing.T) {
	got := split(t, &Plan{Packages: []Entry{
		{Ecosystem: "npm", Name: "lodash", Action: "upgrade", Version: "4.17.21"},
		{Ecosystem: "npm", Name: "lodash", From: " 3.10.1 ", Action: "upgrade", Version: "3.10.2"},
		{Ecosystem: "npm", Name: "lodash", From: "1.0.0", Action: "upgrade", Version: "1.0.1"},
		{Ecosystem: "npm", Name: "lodash", From: "1.0.0", Action: "upgrade", Version: "1.0.2"},
	}}, "npm", []Package{
		{Name: "lodash", Version: "3.10.1"},
		{Name: "lodash", Version: "4.17.15"},
		{Name: "lodash", Version: "1.0.0"},
	})
	require.Equal(t, []rootio.PackagePatch{
		planned("lodash", "3.10.1", "3.10.2"),
		planned("lodash", "4.17.15", "4.17.21"),
		planned("lodash", "1.0.0", "1.0.2"),
	}, got.Patches)
	require.Equal(t, []rootio.Package{
		{Name: "lodash", Version: "3.10.1"},
		{Name: "lodash", Version: "4.17.15"},
		{Name: "lodash", Version: "1.0.0"},
	}, got.Ignore)

	miss := split(t, &Plan{Packages: []Entry{{Ecosystem: "npm", Name: "lodash", From: "4.17.15", Action: "upgrade", Version: "4.17.21"}}}, "npm", []Package{{Name: "lodash", Version: "3.10.1"}})
	require.Empty(t, miss.Patches)
	require.Empty(t, miss.Ignore)
	require.Equal(t, []Unmatched{{Name: "lodash", From: "4.17.15"}}, miss.Absent)
}

func TestSplit_NamesMatchPerEcosystem(t *testing.T) {
	cases := []struct {
		ecosystem, planName, installed string
		matches                        bool
	}{
		{"pypi", "Jinja2", "jinja2", true},
		{"pypi", "typing_extensions", "typing-extensions", true},
		{"pypi", "ruamel.yaml", "ruamel-yaml", true},
		{"pypi", "a-_.b", "a--b", true},
		{"pypi", "jinja2", "jinja3", false},
		{"nuget", "newtonsoft.json", "Newtonsoft.Json", true},
		{"nuget", "Newtonsoft.Json", "Newtonsoft.Xml", false},
		{"composer", "Monolog/Monolog", "monolog/monolog", true},
		{"npm", "Lodash", "lodash", false},
		{"npm", "lodash", "lodash", true},
		{"golang", "github.com/Azure/go", "github.com/azure/go", false},
		{"maven", "Junit:junit", "junit:junit", false},
		{"ruby", "Rack", "rack", false},
	}
	for _, tc := range cases {
		t.Run(tc.ecosystem+"/"+tc.planName+"/"+tc.installed, func(t *testing.T) {
			got := split(t, &Plan{Packages: []Entry{{Ecosystem: tc.ecosystem, Name: tc.planName, Action: "upgrade", Version: "9.9.9"}}}, tc.ecosystem, []Package{{Name: tc.installed, Version: "1.0.0"}})
			if !tc.matches {
				require.Empty(t, got.Patches)
				require.Empty(t, got.Ignore)
				return
			}
			require.Equal(t, []rootio.PackagePatch{planned(tc.installed, "1.0.0", "9.9.9")}, got.Patches)
			require.Equal(t, []rootio.Package{{Name: tc.installed, Version: "1.0.0"}}, got.Ignore)
		})
	}

	got := split(t, &Plan{Packages: []Entry{
		{Ecosystem: "pypi", Name: "Jinja2", Action: "upgrade", Version: "3.1.3"},
		{Ecosystem: "pypi", Name: "jinja2", Action: "upgrade", Version: "3.1.4"},
	}}, "pypi", []Package{{Name: "Jinja2", Version: "3.0.0"}})
	require.Equal(t, "3.1.4", got.Patches[0].Patch.Version)

	for _, tc := range []struct{ entry, eco string }{{"pip", "pypi"}, {"pypi", "pypi"}, {"go", "golang"}, {"golang", "golang"}, {"PIP", " pypi "}} {
		got = split(t, &Plan{Packages: []Entry{{Ecosystem: tc.entry, Name: "pkg", Action: "upgrade", Version: "2"}}}, tc.eco, []Package{{Name: "pkg", Version: "1"}})
		require.Len(t, got.Patches, 1)
	}
	got = split(t, &Plan{Packages: []Entry{{Ecosystem: "go", Name: "pkg", Action: "upgrade", Version: "2"}}}, "pypi", []Package{{Name: "pkg", Version: "1"}})
	require.Empty(t, got.Patches)
}
