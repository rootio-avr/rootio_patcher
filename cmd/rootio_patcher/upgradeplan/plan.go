// Package upgradeplan reads a fixer-written plan of package upgrades and merges
// it with the Root API result for one ecosystem.
package upgradeplan

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"rootio_patcher/pkg/rootio"
)

const (
	// DefaultFileName is looked up in the project directory when no explicit
	// plan path is given. A missing file keeps the Root API lookup.
	DefaultFileName = "root-upgrade-plan.json"

	// ActionUpgrade installs the version named in the plan entry.
	// It is the only action a plan accepts.
	ActionUpgrade = "upgrade"
)

// Entry is one per-package choice in the plan file.
type Entry struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	// From scopes the entry to the installed version it names. Empty applies to every installed version.
	From    string `json:"from,omitempty"`
	Action  string `json:"action"`
	Version string `json:"version,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// Plan is the JSON document the image-rebuilder fixer writes.
type Plan struct {
	Packages []Entry `json:"packages"`
}

// Package is an installed package the plan is matched against.
type Package struct {
	Name    string
	Version string
}

// Load reads a plan file. A missing path returns the os.ErrNotExist from the read.
func Load(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("upgrade plan %s is empty", path)
	}

	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("parse upgrade plan %s: %w", path, err)
	}
	if plan.Packages == nil {
		plan.Packages = []Entry{}
	}
	return &plan, nil
}

// Selection is the plan applied to one ecosystem's installed packages.
type Selection struct {
	// Patches move a planned package to the plan version, exactly as written in the plan.
	Patches []rootio.PackagePatch
	// Ignore lists every planned installed package as name@installed so the Root API skips it.
	Ignore []rootio.Package
	// Absent names plan entries for this ecosystem whose package is not installed.
	Absent []string
	// Current names planned packages already installed at the plan version.
	Current []string
}

// Split separates the installed packages the plan upgrades from the rest.
// Only entries for ecosystem are read; other ecosystems are neither validated nor used.
// A planned package already at the plan version gets no patch but is still ignored,
// so the Root API does not move it to a Root build.
func (p *Plan) Split(ecosystem string, installed []Package) (Selection, error) {
	if p == nil {
		return Selection{}, fmt.Errorf("upgrade plan is nil")
	}
	eco := canonicalEcosystem(ecosystem)
	ordered, err := p.choicesFor(eco)
	if err != nil {
		return Selection{}, err
	}
	choices := make(map[entryKey]Entry, len(ordered))
	for _, choice := range ordered {
		choices[choice.key()] = choice
	}

	var sel Selection
	matched := make(map[entryKey]struct{}, len(choices))
	current := make(map[string]struct{})
	seen := make(map[string]struct{})
	for _, pkg := range installed {
		name := strings.TrimSpace(pkg.Name)
		normalized := normalizeName(eco, name)
		choice, ok := choices[entryKey{name: normalized, from: pkg.Version}]
		if !ok {
			choice, ok = choices[entryKey{name: normalized}]
		}
		if !ok {
			continue
		}
		matched[choice.key()] = struct{}{}
		key := name + "\x00" + pkg.Version
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		sel.Ignore = append(sel.Ignore, rootio.Package{Name: name, Version: pkg.Version})
		if pkg.Version == choice.Version {
			if _, ok := current[name]; !ok {
				current[name] = struct{}{}
				sel.Current = append(sel.Current, name)
			}
			continue
		}
		sel.Patches = append(sel.Patches, rootio.PackagePatch{
			PackageName: name,
			Version:     pkg.Version,
			Patch:       rootio.PatchInfo{Name: name, Version: choice.Version},
		})
	}
	for _, choice := range ordered {
		if _, ok := matched[choice.key()]; !ok {
			sel.Absent = append(sel.Absent, choice.Name)
		}
	}
	return sel, nil
}

// canonicalEcosystem maps the rootio_patcher subcommand names pip and go onto the
// API ecosystem names pypi and golang, so a plan may use either.
func canonicalEcosystem(name string) string {
	switch eco := strings.ToLower(strings.TrimSpace(name)); eco {
	case "pip":
		return "pypi"
	case "go":
		return "golang"
	default:
		return eco
	}
}

// entryKey identifies a plan entry. An empty from applies to every installed version of the name.
type entryKey struct{ name, from string }

func (e Entry) key() entryKey {
	return entryKey{name: normalizeName(e.Ecosystem, e.Name), from: e.From}
}

// normalizeName reduces a package name to the form its ecosystem treats as identical.
// pypi follows PEP 503; nuget and composer ignore case; npm, golang, maven and ruby are exact.
func normalizeName(ecosystem, name string) string {
	switch ecosystem {
	case "pypi":
		return pep503Separators.ReplaceAllString(strings.ToLower(name), "-")
	case "nuget", "composer":
		return strings.ToLower(name)
	default:
		return name
	}
}

var pep503Separators = regexp.MustCompile(`[-_.]+`)

// choicesFor returns the last plan entry per (name, from) for ecosystem.
// Slice order is first-seen, so a later duplicate overrides the choice without moving it.
// Entries for other ecosystems are skipped without validation.
func (p *Plan) choicesFor(ecosystem string) ([]Entry, error) {
	var order []Entry
	index := make(map[entryKey]int)

	for i, entry := range p.Packages {
		if canonicalEcosystem(entry.Ecosystem) != ecosystem {
			continue
		}

		name := strings.TrimSpace(entry.Name)
		if name == "" {
			return nil, fmt.Errorf("upgrade plan entry %d: package name is empty", i)
		}
		action := strings.ToLower(strings.TrimSpace(entry.Action))
		if action != ActionUpgrade {
			return nil, fmt.Errorf("upgrade plan package %q: action %q is not supported, use %q", name, strings.TrimSpace(entry.Action), ActionUpgrade)
		}
		if strings.TrimSpace(entry.Version) == "" {
			return nil, fmt.Errorf("upgrade plan package %q: upgrade requires a version", name)
		}

		stored := Entry{
			Ecosystem: ecosystem,
			Name:      name,
			From:      strings.TrimSpace(entry.From),
			Action:    action,
			Version:   strings.TrimSpace(entry.Version),
			Reason:    strings.TrimSpace(entry.Reason),
		}
		if at, ok := index[stored.key()]; ok {
			order[at] = stored
		} else {
			index[stored.key()] = len(order)
			order = append(order, stored)
		}
	}

	return order, nil
}
