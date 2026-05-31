package common

import (
	"bufio"
	"os"
	"strings"

	"rootio_patcher/pkg/rootio"
)

// LoadIgnoreList reads .rootioignore at ignoreFilePath (silently skipped if absent)
// and merges with flagEntries. Each entry is "package@version".
// Lines starting with "#" and blank lines are ignored.
func LoadIgnoreList(ignoreFilePath string, flagEntries []string) map[string]struct{} {
	set := make(map[string]struct{})

	f, err := os.Open(ignoreFilePath)
	if err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			set[line] = struct{}{}
		}
	}

	for _, entry := range flagEntries {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			set[entry] = struct{}{}
		}
	}

	return set
}

// IgnoreListToPackages converts an ignore set (keys: "name@version") to a slice of Package
// suitable for sending to the API.
func IgnoreListToPackages(ignoreSet map[string]struct{}) []rootio.Package {
	if len(ignoreSet) == 0 {
		return nil
	}
	result := make([]rootio.Package, 0, len(ignoreSet))
	for key := range ignoreSet {
		at := strings.LastIndex(key, "@")
		if at < 0 {
			continue
		}
		result = append(result, rootio.Package{Name: key[:at], Version: key[at+1:]})
	}
	return result
}
