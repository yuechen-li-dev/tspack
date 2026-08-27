package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

func TestAuditCommandReportsLockedVulnerabilityAndThreshold(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/v1/querybatch" {
			_, _ = writer.Write([]byte(`{"results":[{"vulns":[{"id":"GHSA-test"}]}]}`))
			return
		}
		if request.URL.Path == "/v1/vulns/GHSA-test" {
			_, _ = writer.Write([]byte(`{"id":"GHSA-test","summary":"test advisory","database_specific":{"severity":"MODERATE"},"affected":[{"package":{"ecosystem":"npm","name":"demo"},"ranges":[{"events":[{"fixed":"1.0.1"}]}]}]}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	root := t.TempDir()
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{{ID: "npm:demo@1.0.0", Name: "demo", Version: "1.0.0", Source: "npm", Integrity: "sha512-test"}}, Edges: []lockfile.Edge{{From: "app:target:web", To: "npm:demo@1.0.0", Kind: "runtime"}}}
	contents, err := lockfile.Marshal(lf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ts-lock.toml"), contents, 0o644); err != nil {
		t.Fatal(err)
	}

	command := newInProcessCommand("audit", "--root", root, "--audit-level", "high", "--json")
	command.Env = append(os.Environ(), "TSPACK_OSV_API="+server.URL)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("high threshold should pass: %v\n%s", err, output)
	}
	var report map[string]any
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatal(err)
	}
	if report["ok"] != true || report["failing"].(float64) != 0 {
		t.Fatalf("unexpected report: %#v", report)
	}

	command = newInProcessCommand("audit", "--root", root, "--json")
	command.Env = append(os.Environ(), "TSPACK_OSV_API="+server.URL)
	output, err = command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "GHSA-test") {
		t.Fatalf("default audit should fail with finding: %v\n%s", err, output)
	}
}

func TestAuditCommandMakesPartialMixedCoverageExplicit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/v1/querybatch" {
			_, _ = writer.Write([]byte(`{"results":[{}]}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	root := t.TempDir()
	locked := &lockfile.Lockfile{
		Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{
			{ID: "npm:demo@1.0.0", Name: "demo", Version: "1.0.0", Source: "npm", Integrity: "sha512-test"},
			{ID: "jsr:@std/path@1.1.6", Name: "@std/path", Version: "1.1.6", Source: "jsr", Integrity: "sha512-test"},
		},
	}
	contents, err := lockfile.Marshal(locked)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ts-lock.toml"), contents, 0o644); err != nil {
		t.Fatal(err)
	}

	command := newInProcessCommand("audit", "--root", root)
	command.Env = append(os.Environ(), "TSPACK_OSV_API="+server.URL)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("mixed audit failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{
		"npm: 1 checked.",
		"jsr: 1 not checked (unsupported ecosystem).",
		"No known vulnerabilities found in checked packages; audit coverage is incomplete.",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("audit output missing %q:\n%s", expected, text)
		}
	}
}
