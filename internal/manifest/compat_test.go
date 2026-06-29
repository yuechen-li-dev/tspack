package manifest

import (
	"encoding/json"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/diag"
)

func TestCompatFileValidationRejectsDuplicateUnsupportedAndBadPaths(t *testing.T) {
	ir := validCompatManifest()
	ir.CompatFiles = []CompatFile{
		{Path: "tsconfig.tspack.json", Format: "json", Value: json.RawMessage(`{"ok":true}`)},
		{Path: "tsconfig.tspack.json", Format: "json", Value: json.RawMessage(`{"ok":true}`)},
		{Path: "package.json", Format: "json", Value: json.RawMessage(`{"ok":true}`)},
		{Path: "../escape.json", Format: "json", Value: json.RawMessage(`{"ok":true}`)},
		{Path: "pnpm-lock.yaml", Format: "json", Value: json.RawMessage(`{"ok":true}`)},
	}
	diagnostics := Validate("manifest.tsx", &ir)
	assertHasDiagnostic(t, diagnostics, "TSPACK_COMPAT_DUPLICATE_FILE")
	assertHasDiagnostic(t, diagnostics, "TSPACK_COMPAT_UNSUPPORTED_FILE")
	assertHasDiagnostic(t, diagnostics, "TSPACK_COMPAT_PATH_INVALID")
}

func TestCompatFileValidationRejectsMissingValue(t *testing.T) {
	ir := validCompatManifest()
	ir.CompatFiles = []CompatFile{{Path: "settings.json", Format: "json"}}
	diagnostics := Validate("manifest.tsx", &ir)
	assertHasDiagnostic(t, diagnostics, "TSPACK_COMPAT_VALUE_INVALID")
}

func validCompatManifest() ManifestIR {
	return ManifestIR{
		Format:    1,
		Workspace: Workspace{Name: "workspace", Runtime: "nodejs"},
		Packages:  []Package{{Name: "app", Version: "0.1.0", Kind: "app", Publish: PublishPolicy{Include: []string{}}}},
	}
}

func assertHasDiagnostic(t *testing.T, diagnostics []diag.Diagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("missing diagnostic %s in %#v", code, diagnostics)
}
