package rpm

import (
	"reflect"
	"testing"
)

func TestParseRpmQaOutput(t *testing.T) {
	out := []byte("curl\t8.5.0-1.el9\nbash\t5.1.8-6.el9\n\n")

	packages, err := parseRpmQaOutput(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []InstalledPackage{
		{Name: "curl", Version: "8.5.0-1.el9"},
		{Name: "bash", Version: "5.1.8-6.el9"},
	}
	if !reflect.DeepEqual(packages, want) {
		t.Errorf("got %+v, want %+v", packages, want)
	}
}

func TestParseRpmQaOutput_Empty(t *testing.T) {
	packages, err := parseRpmQaOutput([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(packages) != 0 {
		t.Errorf("expected no packages, got %+v", packages)
	}
}

func TestParseRpmQaOutput_GpgPubkeyExcluded(t *testing.T) {
	out := []byte("curl\t8.5.0-1.el9\ngpg-pubkey\t3.0-618f97d4\nbash\t5.1.8-6.el9\n")

	packages, err := parseRpmQaOutput(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []InstalledPackage{
		{Name: "curl", Version: "8.5.0-1.el9"},
		{Name: "bash", Version: "5.1.8-6.el9"},
	}
	if !reflect.DeepEqual(packages, want) {
		t.Errorf("got %+v, want %+v", packages, want)
	}
}

func TestParseRpmQaOutput_MalformedLineSkipped(t *testing.T) {
	out := []byte("curl\t8.5.0-1.el9\nnoversion\nbash\t5.1.8-6.el9\n")

	packages, err := parseRpmQaOutput(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []InstalledPackage{
		{Name: "curl", Version: "8.5.0-1.el9"},
		{Name: "bash", Version: "5.1.8-6.el9"},
	}
	if !reflect.DeepEqual(packages, want) {
		t.Errorf("got %+v, want %+v", packages, want)
	}
}
