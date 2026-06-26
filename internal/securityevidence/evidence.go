package securityevidence

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

const (
	StatusNone    = "none"
	StatusPresent = "present"
	StatusMissing = "missing"
	StatusInvalid = "invalid"
)

type Evidence struct {
	BehaviorFixture       string
	BehaviorFixtureStatus string
	BehaviorReport        string
	BehaviorReportStatus  string
}

func Evaluate(root string, acknowledgement manifest.AcknowledgedCapability) Evidence {
	evidence := Evidence{
		BehaviorFixture:       acknowledgement.BehaviorFixture,
		BehaviorFixtureStatus: StatusNone,
		BehaviorReport:        acknowledgement.BehaviorReport,
		BehaviorReportStatus:  StatusNone,
	}
	if acknowledgement.BehaviorFixture != "" {
		evidence.BehaviorFixtureStatus = fileStatus(root, acknowledgement.BehaviorFixture)
	}
	if acknowledgement.BehaviorReport != "" {
		evidence.BehaviorReportStatus = reportStatus(root, acknowledgement.BehaviorReport)
	}
	return evidence
}

func Diagnostics(root string, acknowledgements []manifest.AcknowledgedCapability) []diag.Diagnostic {
	diagnostics := []diag.Diagnostic{}
	for _, acknowledgement := range acknowledgements {
		evidence := Evaluate(root, acknowledgement)
		if evidence.BehaviorFixtureStatus == StatusMissing {
			diagnostics = append(diagnostics, diag.Diagnostic{
				Code:     "TSPACK_SECURITY_BEHAVIOR_FIXTURE_MISSING",
				Severity: diag.SeverityWarning,
				Message:  "acknowledged lifecycle behavior fixture is missing",
				Details: []string{
					"package: " + acknowledgement.Package,
					"script: " + acknowledgement.Script,
					"command: " + acknowledgement.Command,
					"behaviorFixture: " + acknowledgement.BehaviorFixture,
					"execution: not run by check, doctor, why, update, sync, or materialization",
				},
			})
		}
		if evidence.BehaviorReportStatus == StatusMissing {
			diagnostics = append(diagnostics, diag.Diagnostic{
				Code:     "TSPACK_SECURITY_BEHAVIOR_REPORT_MISSING",
				Severity: diag.SeverityWarning,
				Message:  "acknowledged lifecycle behavior report is missing",
				Details: []string{
					"package: " + acknowledgement.Package,
					"script: " + acknowledgement.Script,
					"command: " + acknowledgement.Command,
					"behaviorReport: " + acknowledgement.BehaviorReport,
					"execution: not run by check, doctor, why, update, sync, or materialization",
				},
			})
		}
		if evidence.BehaviorReportStatus == StatusInvalid {
			diagnostics = append(diagnostics, diag.Diagnostic{
				Code:     "TSPACK_SECURITY_BEHAVIOR_REPORT_INVALID",
				Severity: diag.SeverityWarning,
				Message:  "acknowledged lifecycle behavior report is not valid JSON",
				Details: []string{
					"package: " + acknowledgement.Package,
					"script: " + acknowledgement.Script,
					"command: " + acknowledgement.Command,
					"behaviorReport: " + acknowledgement.BehaviorReport,
				},
			})
		}
	}
	diag.SortDiagnostics(diagnostics)
	return diagnostics
}

func fileStatus(root string, relativePath string) string {
	filePath := filepath.Join(root, filepath.FromSlash(relativePath))
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return StatusMissing
		}
		return StatusMissing
	}
	if info.IsDir() {
		return StatusMissing
	}
	return StatusPresent
}

func reportStatus(root string, relativePath string) string {
	filePath := filepath.Join(root, filepath.FromSlash(relativePath))
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return StatusMissing
		}
		return StatusMissing
	}
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return StatusInvalid
	}
	return StatusPresent
}
