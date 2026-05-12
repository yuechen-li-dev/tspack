package diag

import "sort"

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type Diagnostic struct {
	Code     string
	Severity Severity
	Message  string
	File     string
	Details  []string
	Fixes    []string
}

func SortDiagnostics(diags []Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		a, b := diags[i], diags[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Message != b.Message {
			return a.Message < b.Message
		}
		return a.Severity < b.Severity
	})
}
