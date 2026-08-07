package version

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRequirement(t *testing.T) {
	oldVersion := Version
	Version = "v0.1.8"
	t.Cleanup(func() { Version = oldVersion })

	dir := t.TempDir()
	if requirement, err := ReadRequirement(dir); err != nil || requirement != nil {
		t.Fatalf("missing requirement = %#v, %v", requirement, err)
	}
	if err := os.WriteFile(filepath.Join(dir, RequirementFile), []byte("v0.1.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	requirement, err := ReadRequirement(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !requirement.TooOld || requirement.Minimum != "v0.1.9" || requirement.Current != "v0.1.8" {
		t.Fatalf("unexpected requirement: %#v", requirement)
	}
}

func TestReadRequirementRejectsInvalidContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RequirementFile), []byte(">=0.1.8\nextra"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadRequirement(dir)
	if err == nil || !strings.Contains(err.Error(), "exactly one semantic version") {
		t.Fatalf("unexpected error: %v", err)
	}
}
