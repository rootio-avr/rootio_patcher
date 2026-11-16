package npm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1" //nolint:gosec // npm SRI may still be sha1 for old tarballs; it is verified, not trusted alone
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"rootio_patcher/cmd/rootio_patcher/common"
	"rootio_patcher/cmd/rootio_patcher/upgradeplan"
	"rootio_patcher/pkg/rootio"
)

// BundledApp patches an installed node_modules tree that has no lockfile: npm's own bundled
// dependencies (/usr/local/lib/node_modules/npm), a Lambda runtime's /var/lang/lib/node_modules,
// corepack, a vendored app tree. Lockfile remediation can't reach these: nothing ever runs
// `npm install` against them. Instead every installed package directory is listed, the Root API
// says which have a Root build, and each such directory is replaced in place by that build's
// tarball (integrity-checked), keeping its nested node_modules.
//
// A package that has a Root build but can't be swapped is an error, never a silent skip.
type BundledApp struct {
	trees           []string
	pkgURL          string
	apiKey          string
	dryRun          bool
	upgradePlanPath string
	ignoreSet       map[string]struct{}
	logger          *slog.Logger
	apiClient       common.APIClient
	httpClient      *http.Client
}

// installedPackage is one package directory found in a tree.
type installedPackage struct {
	Name    string
	Version string
	Dir     string
}

// BundledResult is what a run did, per package directory.
type BundledResult struct {
	Rotated []string // "name old -> new (dir)"
	Failed  []string // "name@version (dir): reason"
	Skipped []string // "name@version (dir): reason"
}

// NewBundledApp creates a bundled-tree remediation for the given tree roots.
func NewBundledApp(apiKey, apiURL, pkgURL string, trees []string, dryRun bool, ignoreEntries []string, logger *slog.Logger) *BundledApp {
	ignoreSet := map[string]struct{}{}
	for _, tree := range trees {
		for k := range common.LoadIgnoreList(filepath.Join(tree, ".rootioignore"), ignoreEntries) {
			ignoreSet[k] = struct{}{}
		}
	}
	return &BundledApp{
		trees:      trees,
		pkgURL:     strings.TrimRight(pkgURL, "/"),
		apiKey:     apiKey,
		dryRun:     dryRun,
		ignoreSet:  ignoreSet,
		logger:     logger,
		apiClient:  rootio.NewClient(apiURL, apiKey),
		httpClient: &http.Client{Timeout: 2 * time.Minute},
	}
}

// WithUpgradePlan sets an explicit upgrade plan; bundled trees have no default plan file.
func (a *BundledApp) WithUpgradePlan(path string) *BundledApp {
	a.upgradePlanPath = path
	return a
}

// Run lists, analyzes and swaps. Dry-run returns common.ErrPatchesAvailable when swaps are due.
func (a *BundledApp) Run(ctx context.Context) error {
	res, err := a.run(ctx)
	if err != nil {
		return err
	}
	for _, line := range res.Skipped {
		fmt.Printf("skipped %s\n", line)
	}
	verb := "rotated"
	if a.dryRun {
		verb = "would rotate"
	}
	for _, line := range res.Rotated {
		fmt.Printf("%s %s\n", verb, line)
	}
	for _, line := range res.Failed {
		fmt.Printf("NOT ROTATED %s\n", line)
	}
	fmt.Printf("\nbundled npm: %d %s, %d failed, %d skipped\n", len(res.Rotated), verb, len(res.Failed), len(res.Skipped))
	if len(res.Failed) > 0 {
		return fmt.Errorf("bundled npm: %d package(s) with a Root build could not be rotated", len(res.Failed))
	}
	if a.dryRun && len(res.Rotated) > 0 {
		return common.ErrPatchesAvailable
	}
	return nil
}

func (a *BundledApp) run(ctx context.Context) (*BundledResult, error) {
	res := &BundledResult{}
	var installed []installedPackage
	var err error
	for _, tree := range a.trees {
		found, skipped, werr := walkTree(tree)
		if werr != nil {
			return nil, werr
		}
		installed = append(installed, found...)
		res.Skipped = append(res.Skipped, skipped...)
	}
	if len(installed) == 0 {
		return res, nil
	}

	var plan *upgradeplan.Plan
	if a.upgradePlanPath != "" { // a bundled tree has no default plan file
		if plan, err = upgradeplan.Resolve(a.upgradePlanPath, ""); err != nil {
			return nil, err
		}
	}
	unique := map[string]rootio.Package{}
	for _, p := range installed {
		unique[p.Name+"@"+p.Version] = rootio.Package{Name: p.Name, Version: p.Version}
	}
	pkgs := make([]rootio.Package, 0, len(unique))
	for _, p := range unique {
		pkgs = append(pkgs, p)
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name+pkgs[i].Version < pkgs[j].Name+pkgs[j].Version })

	// An unreachable API is an error: skipping would ship the image unpatched without a trace.
	resp, err := upgradeplan.Analyze(ctx, a.logger, a.apiClient, plan, "npm", pkgs, common.IgnoreListToPackages(a.ignoreSet))
	if err != nil {
		return nil, fmt.Errorf("analyze bundled npm packages: %w", err)
	}

	patches := map[string]rootio.PackagePatch{}
	for _, p := range resp.Patches {
		key := p.PackageName + "@" + p.Version
		if _, ignored := a.ignoreSet[key]; ignored {
			continue // enforced here too: the API's answer alone doesn't decide what an ignore means
		}
		patches[key] = p
	}
	tarballs := map[string][]byte{}
	for _, inst := range installed {
		patch, ok := patches[inst.Name+"@"+inst.Version]
		if !ok {
			continue
		}
		target := patch.Patch.Version
		if patch.Patch.Name != "" && patch.Patch.Name != inst.Name {
			res.Failed = append(res.Failed, fmt.Sprintf("%s@%s (%s): Root build is %s, a different package name",
				inst.Name, inst.Version, inst.Dir, patch.Patch.Name))
			continue
		}
		line := fmt.Sprintf("%s %s -> %s (%s)", inst.Name, inst.Version, target, inst.Dir)
		if a.dryRun {
			res.Rotated = append(res.Rotated, line)
			continue
		}
		key := inst.Name + "@" + target
		data, ok := tarballs[key]
		if !ok {
			if data, err = a.fetchTarball(ctx, inst.Name, target); err != nil {
				res.Failed = append(res.Failed, fmt.Sprintf("%s@%s (%s): %v", inst.Name, inst.Version, inst.Dir, err))
				continue
			}
			tarballs[key] = data
		}
		if err := swapPackageDir(inst.Dir, inst.Name, target, data); err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("%s@%s (%s): %v", inst.Name, inst.Version, inst.Dir, err))
			continue
		}
		res.Rotated = append(res.Rotated, line)
	}
	return res, nil
}

// walkTree lists every package directory under root: node_modules/<name> and node_modules/@scope/<name>
// whose package.json declares that name and a version. Symlinked directories are not followed. pnpm
// store entries (.pnpm/<name>@<version>/...) are reported as skipped: their path encodes the version.
func walkTree(root string) ([]installedPackage, []string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, nil, fmt.Errorf("bundled tree %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("bundled tree %s is not a directory", root)
	}
	var found []installedPackage
	var skipped []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || path == root {
			return nil
		}
		if strings.Contains(path, string(filepath.Separator)+".pnpm"+string(filepath.Separator)) {
			if name, ok := packageNameFromDir(path); ok {
				if pkg, ok := readPackageJSON(path, name); ok {
					skipped = append(skipped, fmt.Sprintf("%s@%s (%s): pnpm store entry", pkg.Name, pkg.Version, path))
				}
			}
			return nil
		}
		name, ok := packageNameFromDir(path)
		if !ok {
			return nil
		}
		if pkg, ok := readPackageJSON(path, name); ok {
			found = append(found, pkg)
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return found, skipped, nil
}

// packageNameFromDir returns the package name a directory holds when it sits directly in node_modules.
func packageNameFromDir(path string) (string, bool) {
	parent := filepath.Base(filepath.Dir(path))
	base := filepath.Base(path)
	if parent == "node_modules" && !strings.HasPrefix(base, "@") && !strings.HasPrefix(base, ".") {
		return base, true
	}
	if strings.HasPrefix(parent, "@") && filepath.Base(filepath.Dir(filepath.Dir(path))) == "node_modules" {
		return parent + "/" + base, true
	}
	return "", false
}

func readPackageJSON(dir, name string) (installedPackage, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return installedPackage{}, false
	}
	var pj struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &pj) != nil || pj.Name != name || pj.Version == "" {
		return installedPackage{}, false
	}
	return installedPackage{Name: pj.Name, Version: pj.Version, Dir: dir}, true
}

// fetchTarball reads the package's packument from the Root registry and downloads that version's tarball,
// checking its integrity (SRI) or shasum.
func (a *BundledApp) fetchTarball(ctx context.Context, name, version string) ([]byte, error) {
	packument, err := a.get(ctx, a.pkgURL+"/npm/"+strings.Replace(name, "/", "%2f", 1)) // @scope%2fname
	if err != nil {
		return nil, fmt.Errorf("packument: %w", err)
	}
	var doc struct {
		Versions map[string]struct {
			Dist struct {
				Tarball   string `json:"tarball"`
				Integrity string `json:"integrity"`
				Shasum    string `json:"shasum"`
			} `json:"dist"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(packument, &doc); err != nil {
		return nil, fmt.Errorf("packument: %w", err)
	}
	v, ok := doc.Versions[version]
	if !ok || v.Dist.Tarball == "" {
		return nil, fmt.Errorf("version %s not in the Root registry", version)
	}
	data, err := a.get(ctx, v.Dist.Tarball)
	if err != nil {
		return nil, fmt.Errorf("tarball: %w", err)
	}
	if err := verifyIntegrity(data, v.Dist.Integrity, v.Dist.Shasum); err != nil {
		return nil, err
	}
	return data, nil
}

func (a *BundledApp) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if a.sameOrigin(req.URL) { // the key only goes to the Root registry, not to a tarball host a packument names
		req.SetBasicAuth("root", a.apiKey)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", rawURL, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 512<<20))
}

func (a *BundledApp) sameOrigin(u *url.URL) bool {
	reg, err := url.Parse(a.pkgURL)
	return err == nil && strings.EqualFold(u.Scheme, reg.Scheme) && strings.EqualFold(u.Host, reg.Host)
}

// sriAlgos are the SRI hash algorithms checked, strongest first.
var sriAlgos = []struct {
	name string
	new  func() hash.Hash
}{{"sha512", sha512.New}, {"sha256", sha256.New}, {"sha1", sha1.New}} //nolint:gosec // see import

// verifyIntegrity checks an SRI string ("sha512-<b64>", possibly several space-separated entries) the way SRI does:
// only the strongest algorithm listed counts, and any of its entries matching passes. With no supported SRI entry,
// the hex sha1 shasum is checked. One of them is required: a tarball nothing vouches for is not swapped in.
func verifyIntegrity(data []byte, integrity, shasum string) error {
	entries := map[string][]string{}
	for _, sri := range strings.Fields(integrity) {
		if algo, want, ok := strings.Cut(sri, "-"); ok {
			entries[algo] = append(entries[algo], want)
		}
	}
	for _, algo := range sriAlgos {
		wants, ok := entries[algo.name]
		if !ok {
			continue
		}
		h := algo.new()
		h.Write(data)
		got := base64.StdEncoding.EncodeToString(h.Sum(nil))
		for _, want := range wants {
			if got == want {
				return nil
			}
		}
		return fmt.Errorf("integrity mismatch (%s)", algo.name)
	}
	if shasum != "" {
		sum := sha1.Sum(data) //nolint:gosec // see import
		if fmt.Sprintf("%x", sum) == strings.ToLower(shasum) {
			return nil
		}
		return errors.New("shasum mismatch")
	}
	return errors.New("no integrity or shasum to verify the tarball")
}

// swapPackageDir replaces dir's contents with the tarball's package, keeping dir's nested node_modules, after
// checking the tarball declares name@version. dir itself is never renamed: in an image build it usually comes from
// a lower layer, and overlayfs refuses to rename such a directory (EXDEV). Its files are removed in place and the
// new ones, extracted into a sibling temp dir in the top layer, are moved in.
func swapPackageDir(dir, name, version string, tgz []byte) error {
	tmp, err := os.MkdirTemp(filepath.Dir(dir), "."+filepath.Base(dir)+".rootio-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := extractPackage(tgz, tmp); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	if pkg, ok := readPackageJSON(tmp, name); !ok || pkg.Version != version {
		return fmt.Errorf("tarball does not declare %s@%s", name, version)
	}
	old, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range old {
		if e.Name() == "node_modules" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("remove old %s: %w", e.Name(), err)
		}
	}
	fresh, err := os.ReadDir(tmp)
	if err != nil {
		return err
	}
	for _, e := range fresh {
		if e.Name() == "node_modules" {
			continue // the installed tree's own nested node_modules stays
		}
		if err := os.Rename(filepath.Join(tmp, e.Name()), filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("move in %s: %w", e.Name(), err)
		}
	}
	return nil
}

// extractPackage writes an npm tarball (entries under one top directory, usually "package/") into dest,
// dropping that top directory. Absolute paths, "..", and links are refused.
func extractPackage(tgz []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		_, rel, ok := strings.Cut(strings.TrimPrefix(hdr.Name, "./"), "/")
		if !ok || rel == "" {
			continue
		}
		clean := filepath.Clean(rel)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path %q", hdr.Name)
		}
		target := filepath.Join(dest, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if hdr.Mode&0o111 != 0 {
				mode = 0o755
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, io.LimitReader(tr, 256<<20)); err != nil {
				_ = f.Close() // the copy error is the one to report
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("link in package tarball: %q", hdr.Name)
		}
	}
}
