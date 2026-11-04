package ruby

import (
	"fmt"
	"regexp"
	"strings"
)

// GemfileEditor edits a Gemfile with line-based scanning (no Ruby parser, so the CLI stays a
// pure-Go single binary). Declarations it cannot confidently rewrite yield an error rather
// than a guess.
type GemfileEditor struct {
	lines   []string
	newline string
}

// NewGemfileEditor creates an editor over raw Gemfile content.
func NewGemfileEditor(content string) *GemfileEditor {
	nl := "\n"
	if strings.Contains(content, "\r\n") {
		nl = "\r\n"
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return &GemfileEditor{lines: strings.Split(content, "\n"), newline: nl}
}

// String returns the edited Gemfile content.
func (g *GemfileEditor) String() string {
	return strings.Join(g.lines, g.newline)
}

// gemStmt is a (possibly multi-line) `gem "name", ...` statement: lines [start, end].
type gemStmt struct{ start, end int }

// arg is a top-level argument of a gem call: byte offsets into the statement text.
type arg struct{ start, end int }

// frame is an open block (`... do`, `if`, ...) found while scanning.
type frame struct {
	header     int // line index of the opener
	end        int // line index of the closing `end`
	sourceLike bool
}

var (
	blockKeywordRe = regexp.MustCompile(`^\s*(if|unless|case|begin|while|until|for|def|class|module)\b`)
	doEndRe        = regexp.MustCompile(`\bdo\b(\s*\|[^|]*\|)?\s*$`)
	endLineRe      = regexp.MustCompile(`^\s*end\b`)
	sourceBlockRe  = regexp.MustCompile(`^\s*(source|git|github|path)\b`)
	constraintRe   = regexp.MustCompile(`^\s*(~>|>=|<=|!=|>|<|=)?\s*\d+(\.[0-9A-Za-z]+)*\s*$`)
	versionRe      = regexp.MustCompile(`^\d+(\.\d+)*$`)
	gitKeys        = map[string]bool{"git": true, "github": true, "path": true, "tag": true, "branch": true, "ref": true, "submodules": true}
)

// stripComment removes a trailing `# comment` (quote-aware) from one line.
func stripComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '#':
			return line[:i]
		}
	}
	return line
}

func gemLineRe(name string) *regexp.Regexp {
	return regexp.MustCompile(`^\s*gem[\s(]+['"]` + regexp.QuoteMeta(name) + `['"]`)
}

// findGems returns all statements declaring the gem, in file order.
func (g *GemfileEditor) findGems(name string) []gemStmt {
	re := gemLineRe(name)
	var out []gemStmt
	for i := 0; i < len(g.lines); i++ {
		if !re.MatchString(g.lines[i]) {
			continue
		}
		end := i
		for end < len(g.lines)-1 {
			code := strings.TrimRight(stripComment(g.lines[end]), " \t")
			if !strings.HasSuffix(code, ",") && !strings.HasSuffix(code, "\\") {
				break
			}
			end++
		}
		out = append(out, gemStmt{i, end})
		i = end
	}
	return out
}

func (g *GemfileEditor) stmtText(s gemStmt) string {
	return strings.Join(g.lines[s.start:s.end+1], "\n")
}

func (g *GemfileEditor) setStmtText(s gemStmt, text string) {
	g.lines = append(g.lines[:s.start:s.start], append(strings.Split(text, "\n"), g.lines[s.end+1:]...)...)
}

// splitArgs returns the top-level comma-separated arguments of the statement text that follow
// the `gem` keyword (comments are skipped). The first arg is the gem name.
func splitArgs(text string) ([]arg, error) {
	i := strings.Index(text, "gem") + 3
	var args []arg
	depth, startAt := 0, -1
	var quote byte
	flush := func(end int) {
		if startAt >= 0 {
			e := end
			for e > startAt && strings.ContainsRune(" \t\n\\", rune(text[e-1])) {
				e--
			}
			if e > startAt {
				args = append(args, arg{startAt, e})
			}
		}
		startAt = -1
	}
	for ; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch {
		case c == '#':
			flush(i)
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case c == '\'' || c == '"':
			quote = c
			if startAt < 0 {
				startAt = i
			}
		case c == '(' || c == '[' || c == '{':
			depth++
			if startAt < 0 {
				startAt = i
			}
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == ',' && depth == 0:
			flush(i)
		case strings.ContainsRune(" \t\n\\", rune(c)):
		default:
			if startAt < 0 {
				startAt = i
			}
		}
	}
	flush(len(text))
	if len(args) == 0 || strings.HasPrefix(text[args[0].start:], "(") {
		return nil, fmt.Errorf("unsupported gem declaration: %q", firstLine(text))
	}
	return args, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// unquote returns the content of a bare quoted string argument.
func unquote(s string) (string, bool) {
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1], true
	}
	return "", false
}

var pairKeyRe = regexp.MustCompile(`^(?::(\w+)\s*=>|(\w+):|["'](\w+)["']\s*=>)`)

// pairKey returns the option key of an argument like `source: 'x'` / `:source => 'x'`.
func pairKey(a string) string {
	m := pairKeyRe.FindStringSubmatch(a)
	if m == nil {
		return ""
	}
	return m[1] + m[2] + m[3]
}

// PatchDependency raises (or adds) the version constraint of every declaration of gem so it
// resolves to installVersion. A gem not declared in the Gemfile is appended as a direct
// dependency (with source when non-empty), since Bundler needs it to pin a transitive gem.
func (g *GemfileEditor) PatchDependency(name, installVersion string) error {
	return g.rewriteConstraints(name, ">= "+installVersion, func(cons []string) []string {
		return determineConstraints(cons, installVersion)
	})
}

// PinDependency makes every declaration of gem require exactly version, replacing its other
// version constraints, so a bundle update cannot resolve to anything newer. A gem not declared
// in the Gemfile is appended with the exact pin.
func (g *GemfileEditor) PinDependency(name, version string) error {
	exact := "= " + version
	return g.rewriteConstraints(name, exact, func([]string) []string { return []string{exact} })
}

// rewriteConstraints replaces the version constraints of every declaration of gem with
// newConstraints(existing). A gem that is not declared is appended with addConstraint.
func (g *GemfileEditor) rewriteConstraints(name, addConstraint string, newConstraints func(existing []string) []string) error {
	stmts := g.findGems(name)
	if len(stmts) == 0 {
		g.addGem(name, addConstraint, "")
		return nil
	}
	quote := g.quoteStyle()
	for i := len(stmts) - 1; i >= 0; i-- {
		text := g.stmtText(stmts[i])
		args, err := splitArgs(text)
		if err != nil {
			return err
		}
		var consIdx []int
		var cons []string
		for j, a := range args[1:] {
			if c, ok := unquote(text[a.start:a.end]); ok {
				consIdx = append(consIdx, j+1)
				for _, part := range strings.Split(c, ",") {
					cons = append(cons, strings.TrimSpace(part))
				}
			}
		}
		for k := 1; k < len(consIdx); k++ {
			if consIdx[k] != consIdx[k-1]+1 {
				return fmt.Errorf("unsupported version constraints on gem %q: %q", name, firstLine(text))
			}
		}
		newCons := strings.Join(quoteAll(newConstraints(cons), quote), ", ")
		if len(consIdx) == 0 {
			at := args[0].end
			text = text[:at] + ", " + newCons + text[at:]
		} else {
			first, last := args[consIdx[0]], args[consIdx[len(consIdx)-1]]
			text = text[:first.start] + newCons + text[last.end:]
		}
		g.setStmtText(stmts[i], text)
	}
	return nil
}

func quoteAll(cs []string, q string) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = q + c + q
	}
	return out
}

// determineConstraints computes the new constraint list:
//   - exact pin ("= 1.0") stays exact; bare "1.0" becomes the bare new version
//   - pessimistic ("~> 1.0") stays pessimistic
//   - otherwise ">= new"
//   - upper bounds are kept unless below the new version, then bumped to the next major
//   - other constraints (e.g. "!= 1.3") are preserved
func determineConstraints(cons []string, installVersion string) []string {
	var uppers, extras []string
	hasPess, hasLoose, exact, bare := false, false, false, false
	for _, c := range cons {
		switch {
		case c == "":
		case strings.HasPrefix(c, "~>"):
			hasPess = true
		case strings.HasPrefix(c, "<"):
			if v := strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(c, "<="), "<"), " "); v != "" && compareVersions(v, installVersion) < 0 {
				uppers = append(uppers, "< "+nextMajor(installVersion))
			} else {
				uppers = append(uppers, c)
			}
		case strings.HasPrefix(c, ">"):
			hasLoose = true
		case strings.HasPrefix(c, "="):
			exact = true
		case versionRe.MatchString(c):
			bare = true
		default:
			extras = append(extras, c)
		}
	}
	switch {
	case exact && len(uppers) == 0 && !hasPess && !hasLoose:
		return append([]string{"= " + installVersion}, extras...)
	case hasPess:
		return append(append([]string{"~> " + installVersion}, uppers...), extras...)
	case bare && !hasLoose && len(uppers) == 0:
		return append([]string{installVersion}, extras...)
	}
	return append(append([]string{">= " + installVersion}, uppers...), extras...)
}

func nextMajor(v string) string {
	var major int
	_, _ = fmt.Sscanf(v, "%d", &major)
	return fmt.Sprintf("%d.0", major+1)
}

func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			_, _ = fmt.Sscanf(pa[i], "%d", &x)
		}
		if i < len(pb) {
			_, _ = fmt.Sscanf(pb[i], "%d", &y)
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// quoteStyle returns the quote char used by the majority of gem declarations.
func (g *GemfileEditor) quoteStyle() string {
	single, double := 0, 0
	re := regexp.MustCompile(`^\s*gem\s+(['"])`)
	for _, l := range g.lines {
		if m := re.FindStringSubmatch(l); m != nil {
			if m[1] == "'" {
				single++
			} else {
				double++
			}
		}
	}
	if double > single {
		return `"`
	}
	return "'"
}

func (g *GemfileEditor) addGem(name, constraint, source string) {
	q := g.quoteStyle()
	line := fmt.Sprintf("gem %s%s%s, %s%s%s", q, name, q, q, constraint, q)
	if source != "" {
		line += fmt.Sprintf(", source: %s%s%s", q, source, q)
	}
	for len(g.lines) > 0 && strings.TrimSpace(g.lines[len(g.lines)-1]) == "" {
		g.lines = g.lines[:len(g.lines)-1]
	}
	g.lines = append(g.lines, "", line, "")
}

// frames scans the Gemfile for block structure.
func (g *GemfileEditor) frames() []frame {
	var stack, done []frame
	for i, line := range g.lines {
		code := stripComment(line)
		if endLineRe.MatchString(code) {
			if n := len(stack); n > 0 {
				f := stack[n-1]
				f.end = i
				done = append(done, f)
				stack = stack[:n-1]
			}
			continue
		}
		if doEndRe.MatchString(strings.TrimRight(code, " \t")) || blockKeywordRe.MatchString(code) {
			stack = append(stack, frame{header: i, sourceLike: doEndRe.MatchString(strings.TrimRight(code, " \t")) && sourceBlockRe.MatchString(code)})
		}
	}
	return done
}

// SetGemSource pins every declaration of gem to sourceURL via the inline `source:` option.
// Bundler rejects a gem with both a block-scoped and an inline source, so a declaration inside
// a `source ... do` block is first hoisted out (keeping its group/platforms wrappers). git:/
// github:/path: options are dropped: that ref can never hold the patched version.
func (g *GemfileEditor) SetGemSource(name, sourceURL string) error {
	stmts := g.findGems(name)
	if len(stmts) == 0 {
		g.addGem(name, ">= 0", sourceURL)
		return nil
	}
	for i := len(stmts) - 1; i >= 0; i-- {
		stmt := stmts[i]
		if err := g.stripGitArgs(stmt); err != nil {
			return err
		}
		stmt = g.findGems(name)[i]
		stmt = g.hoist(stmt, name, i)
		if err := g.applySource(stmt, sourceURL); err != nil {
			return err
		}
	}
	return nil
}

func (g *GemfileEditor) stripGitArgs(stmt gemStmt) error {
	text := g.stmtText(stmt)
	args, err := splitArgs(text)
	if err != nil {
		return err
	}
	for j := len(args) - 1; j >= 1; j-- {
		if !gitKeys[pairKey(text[args[j].start:])] {
			continue
		}
		from, to := args[j].start, args[j].end
		if j == len(args)-1 {
			from = args[j-1].end // drop the preceding comma instead
		} else {
			to = args[j+1].start
		}
		text = text[:from] + text[to:]
		if args, err = splitArgs(text); err != nil {
			return err
		}
	}
	g.setStmtText(stmt, text)
	return nil
}

// hoist moves the idx-th declaration of name out of any enclosing source-like block and
// returns its new location.
func (g *GemfileEditor) hoist(stmt gemStmt, name string, idx int) gemStmt {
	var enclosing []frame
	for _, f := range g.frames() {
		if f.header < stmt.start && f.end > stmt.end {
			enclosing = append(enclosing, f)
		}
	}
	// outermost first
	for i := 0; i < len(enclosing); i++ {
		for j := i + 1; j < len(enclosing); j++ {
			if enclosing[j].header < enclosing[i].header {
				enclosing[i], enclosing[j] = enclosing[j], enclosing[i]
			}
		}
	}
	outer := -1
	for i, f := range enclosing {
		if f.sourceLike {
			outer = i
			break
		}
	}
	if outer < 0 {
		return stmt
	}
	block := enclosing[outer]

	indent := len(g.lines[stmt.start]) - len(strings.TrimLeft(g.lines[stmt.start], " \t"))
	var body []string
	for _, l := range g.lines[stmt.start : stmt.end+1] {
		if len(l) >= indent {
			l = l[indent:]
		}
		body = append(body, l)
	}
	// wrap in copies of non-source wrappers between the source block and the gem, innermost first
	for i := len(enclosing) - 1; i > outer; i-- {
		if enclosing[i].sourceLike {
			continue
		}
		wrapped := []string{strings.TrimSpace(g.lines[enclosing[i].header])}
		for _, l := range body {
			wrapped = append(wrapped, "  "+l)
		}
		body = append(wrapped, "end")
	}

	// remove the original, then insert after the block's closing `end`
	removed := stmt.end - stmt.start + 1
	g.lines = append(g.lines[:stmt.start], g.lines[stmt.end+1:]...)
	at := block.end - removed + 1
	ins := append([]string{""}, body...)
	g.lines = append(g.lines[:at], append(ins, g.lines[at:]...)...)
	return g.findGems(name)[idx]
}

func (g *GemfileEditor) applySource(stmt gemStmt, sourceURL string) error {
	text := g.stmtText(stmt)
	args, err := splitArgs(text)
	if err != nil {
		return err
	}
	q := g.quoteStyle()
	for _, a := range args[1:] {
		raw := text[a.start:a.end]
		if pairKey(raw) != "source" {
			continue
		}
		if k := strings.LastIndexAny(raw, `'"`); k >= 0 {
			if j := strings.IndexAny(raw, `'"`); j >= 0 && j < k {
				text = text[:a.start+j] + q + sourceURL + q + text[a.start+k+1:]
				g.setStmtText(stmt, text)
				return nil
			}
		}
		return fmt.Errorf("unsupported source option in gem declaration: %q", firstLine(text))
	}
	at := args[len(args)-1].end
	text = text[:at] + fmt.Sprintf(", source: %s%s%s", q, sourceURL, q) + text[at:]
	g.setStmtText(stmt, text)
	return nil
}
