// Package upgradeplan reads a fixer-written plan of per-package choices and
// turns it into upgrade or leave decisions for one ecosystem.
package upgradeplan

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"rootio_patcher/pkg/rootio"
)

const (
	// DefaultFileName is looked up in the project directory when no explicit
	// plan path is given. A missing file keeps the Root API lookup.
	DefaultFileName = "root-upgrade-plan.json"

	// ActionUpgrade installs the version named in the plan entry.
	ActionUpgrade = "upgrade"
	// ActionLeave does not change the package.
	ActionLeave = "leave"
)

// Entry is one per-package choice in the plan file.
type Entry struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Action    string `json:"action"`
	Version   string `json:"version,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Plan is the JSON document the image-rebuilder fixer writes.
type Plan struct {
	Packages []Entry `json:"packages"`
}

// Package is an installed package the caller wants a decision for.
type Package struct {
	Name    string
	Version string
}

// Upgrade is a package that should be installed at Version.
type Upgrade struct {
	Name             string
	InstalledVersion string
	Version          string
	Reason           string
}

// Leave is an installed package the plan says not to change.
type Leave struct {
	Name             string
	InstalledVersion string
	Reason           string
}

// Result is the decision for one ecosystem.
// Entries for every other ecosystem are listed in IgnoredEcosystems and do not fail the run.
type Result struct {
	Upgrades           []Upgrade
	Leaves             []Leave
	IgnoredEcosystems  []string
	AbsentPackageNames []string
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

// Decide applies the plan to installed packages for ecosystem.
// Only entries for that ecosystem are validated. Other ecosystems are ignored.
// action upgrade installs that version; action leave does not change the package.
// A package already at the upgrade version produces no upgrade.
func (p *Plan) Decide(ecosystem string, installed []Package) (Result, error) {
	if p == nil {
		return Result{}, fmt.Errorf("upgrade plan is nil")
	}

	want := strings.ToLower(strings.TrimSpace(ecosystem))
	ordered, ignored, err := p.choicesFor(want)
	if err != nil {
		return Result{}, err
	}
	choices := make(map[string]Entry, len(ordered))
	for _, choice := range ordered {
		choices[choice.Name] = choice
	}

	result := Result{IgnoredEcosystems: ignored}
	matched := make(map[string]struct{}, len(choices))
	seen := make(map[string]struct{})

	for _, pkg := range installed {
		name := strings.TrimSpace(pkg.Name)
		choice, ok := choices[name]
		if !ok {
			continue
		}
		matched[name] = struct{}{}
		key := name + "\x00" + pkg.Version
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		switch choice.Action {
		case ActionLeave:
			result.Leaves = append(result.Leaves, Leave{
				Name:             name,
				InstalledVersion: pkg.Version,
				Reason:           choice.Reason,
			})
		case ActionUpgrade:
			if pkg.Version == choice.Version {
				continue
			}
			result.Upgrades = append(result.Upgrades, Upgrade{
				Name:             name,
				InstalledVersion: pkg.Version,
				Version:          choice.Version,
				Reason:           choice.Reason,
			})
		}
	}

	for _, choice := range ordered {
		if _, ok := matched[choice.Name]; ok {
			continue
		}
		result.AbsentPackageNames = append(result.AbsentPackageNames, choice.Name)
	}
	return result, nil
}

// AnalyzeResponse converts upgrade decisions into the remediate response shape.
// Leave decisions are reported as skipped and are not patches.
func (r Result) AnalyzeResponse() *rootio.AnalyzePackagesResponse {
	resp := &rootio.AnalyzePackagesResponse{
		Patches: make([]rootio.PackagePatch, 0, len(r.Upgrades)),
		Skipped: make([]rootio.SkippedPackage, 0, len(r.Leaves)),
	}
	for _, upgrade := range r.Upgrades {
		resp.Patches = append(resp.Patches, rootio.PackagePatch{
			PackageName: upgrade.Name,
			Version:     upgrade.InstalledVersion,
			Patch: rootio.PatchInfo{
				Name:    upgrade.Name,
				Version: upgrade.Version,
			},
		})
	}
	for _, leave := range r.Leaves {
		reason := "leave"
		if leave.Reason != "" {
			reason = "leave: " + leave.Reason
		}
		resp.Skipped = append(resp.Skipped, rootio.SkippedPackage{
			PackageName: leave.Name,
			Reason:      reason,
		})
	}
	return resp
}

// choicesFor returns the last plan entry per package name for ecosystem.
// Slice order is first-seen, so a later duplicate overrides the choice without moving it.
func (p *Plan) choicesFor(ecosystem string) ([]Entry, []string, error) {
	var order []Entry
	index := make(map[string]int)
	var ignored []string
	seenEco := make(map[string]struct{})

	for i, entry := range p.Packages {
		eco := strings.ToLower(strings.TrimSpace(entry.Ecosystem))
		if eco != ecosystem {
			if eco == "" {
				continue
			}
			if _, ok := seenEco[eco]; !ok {
				seenEco[eco] = struct{}{}
				ignored = append(ignored, eco)
			}
			continue
		}

		name := strings.TrimSpace(entry.Name)
		if name == "" {
			return nil, nil, fmt.Errorf("upgrade plan entry %d: package name is empty", i)
		}
		action := strings.ToLower(strings.TrimSpace(entry.Action))
		switch action {
		case ActionUpgrade:
			if strings.TrimSpace(entry.Version) == "" {
				return nil, nil, fmt.Errorf("upgrade plan package %q: upgrade requires a version", name)
			}
		case ActionLeave:
		default:
			return nil, nil, fmt.Errorf("upgrade plan package %q: action %q is not upgrade or leave", name, strings.TrimSpace(entry.Action))
		}

		stored := Entry{
			Ecosystem: eco,
			Name:      name,
			Action:    action,
			Version:   strings.TrimSpace(entry.Version),
			Reason:    strings.TrimSpace(entry.Reason),
		}
		if at, ok := index[name]; ok {
			order[at] = stored
		} else {
			index[name] = len(order)
			order = append(order, stored)
		}
	}

	return order, ignored, nil
}
