package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
	bin := buildTspackBinary(t, repoRoot(t))

	command := exec.Command(bin, "audit", "--root", root, "--audit-level", "high", "--json")
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

	command = exec.Command(bin, "audit", "--root", root, "--json")
	command.Env = append(os.Environ(), "TSPACK_OSV_API="+server.URL)
	output, err = command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "GHSA-test") {
		t.Fatalf("default audit should fail with finding: %v\n%s", err, output)
	}
}
