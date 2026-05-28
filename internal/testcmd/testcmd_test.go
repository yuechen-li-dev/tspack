package testcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveXTestBridgeExplicitPath(t *testing.T) {
	root := t.TempDir()
	bridge := filepath.Join(root, "bridge.js")
	if err := os.WriteFile(bridge, []byte(""), 0o644); err != nil {
		t.Fatalf("write bridge: %v", err)
	}

	resolution := ResolveXTestBridge(bridge)
	if resolution.Path != bridge {
		t.Fatalf("expected explicit bridge path %q, got %#v", bridge, resolution)
	}
}

func TestResolveXTestBridgeMissingIncludesSearchContext(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.js")
	resolution := ResolveXTestBridge(missing)
	if resolution.Path != "" {
		t.Fatalf("expected missing bridge, got %#v", resolution)
	}

	diagnostic := missingBridgeDiagnostic(resolution)
	joined := strings.Join(diagnostic.Details, "\n")
	if !strings.Contains(joined, missing) {
		t.Fatalf("expected missing path in details, got %q", joined)
	}
	if !strings.Contains(joined, "cwd:") || !strings.Contains(joined, "executable:") {
		t.Fatalf("expected cwd and executable details, got %q", joined)
	}
}
