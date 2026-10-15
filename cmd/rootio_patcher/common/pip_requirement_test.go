package common

import (
	"testing"

	"rootio_patcher/pkg/rootio"
)

func TestPipRequirement(t *testing.T) {
	info := rootio.PatchInfo{Name: "ms", Version: "2.2.2"}
	if got := PipRequirement(rootio.PackagePatch{Patch: info}); got != "ms==2.2.2" {
		t.Fatalf("api patch = %q", got)
	}
	if got := PipRequirement(rootio.PackagePatch{Patch: info, Planned: true}); got != "ms===2.2.2" {
		t.Fatalf("planned patch = %q", got)
	}
}
