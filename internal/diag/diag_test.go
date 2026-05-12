package diag

import "testing"

func TestDiagnosticCreation(t *testing.T) {
	d := Diagnostic{
		Code:     "M0-001",
		Severity: SeverityError,
		Message:  "example",
		File:     "manifest.tsx",
		Details:  []string{"detail"},
		Fixes:    []string{"fix"},
	}

	if d.Code == "" || d.Message == "" {
		t.Fatalf("diagnostic not initialized: %#v", d)
	}
	if d.Severity != SeverityError {
		t.Fatalf("expected SeverityError, got %q", d.Severity)
	}
}

func TestSortDiagnosticsDeterministic(t *testing.T) {
	in := []Diagnostic{
		{Code: "B", File: "b.ts", Message: "2", Severity: SeverityWarning},
		{Code: "A", File: "c.ts", Message: "1", Severity: SeverityInfo},
		{Code: "A", File: "a.ts", Message: "3", Severity: SeverityError},
	}

	SortDiagnostics(in)

	want := []Diagnostic{
		{Code: "A", File: "a.ts", Message: "3", Severity: SeverityError},
		{Code: "A", File: "c.ts", Message: "1", Severity: SeverityInfo},
		{Code: "B", File: "b.ts", Message: "2", Severity: SeverityWarning},
	}

	for i := range want {
		if in[i].Code != want[i].Code || in[i].File != want[i].File || in[i].Message != want[i].Message || in[i].Severity != want[i].Severity {
			t.Fatalf("at index %d: got %#v want %#v", i, in[i], want[i])
		}
	}
}
