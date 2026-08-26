package cli

import "github.com/yuechen-li-dev/tspack/internal/diag"

func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}

func exitForDiagnostics(diagnostics []diag.Diagnostic) {
	if hasErrors(diagnostics) {
		exit(1)
	}
}
