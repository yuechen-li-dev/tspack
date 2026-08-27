package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/audit"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

func runAuditCommand(args []string) {
	root, auditLevel := ".", "any"
	jsonOutput := false
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--root":
			index++
			if index >= len(args) {
				failAuditArgs("--root requires a value")
			}
			root = args[index]
		case "--audit-level":
			index++
			if index >= len(args) {
				failAuditArgs("--audit-level requires a value")
			}
			auditLevel = args[index]
		case "--json":
			jsonOutput = true
		default:
			failAuditArgs("unknown audit argument: " + args[index])
		}
	}
	threshold, err := audit.ParseThreshold(auditLevel)
	if err != nil {
		failAuditArgs(err.Error())
	}
	root = resolveWorkspaceRoot(root)
	lockPath := filepath.Join(root, "ts-lock.toml")
	lf, diagnostics, err := lockfile.LoadFile(lockPath)
	if err != nil {
		failAudit("TSPACK_AUDIT_LOCKFILE_FAILED", fmt.Sprintf("failed to read %s: %v", lockPath, err), jsonOutput)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" {
			failAudit("TSPACK_AUDIT_LOCKFILE_FAILED", diagnostic.Message+": "+strings.Join(diagnostic.Details, "; "), jsonOutput)
		}
	}
	report, err := audit.Scan(context.Background(), lf, &audit.HTTPClient{Endpoint: os.Getenv("TSPACK_OSV_API")})
	if err != nil {
		failAudit("TSPACK_AUDIT_SERVICE_FAILED", err.Error(), jsonOutput)
	}
	failing := 0
	for _, finding := range report.Findings {
		if audit.FailsThreshold(finding.Severity, threshold) {
			failing++
		}
	}
	if jsonOutput {
		payload := struct {
			OK         bool         `json:"ok"`
			Source     string       `json:"source"`
			AuditLevel string       `json:"auditLevel"`
			Failing    int          `json:"failing"`
			Report     audit.Report `json:"report"`
		}{failing == 0, "OSV.dev", auditLevel, failing, report}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
	} else {
		printAuditReport(report, auditLevel, failing)
	}
	if failing > 0 {
		exit(1)
	}
}

func printAuditReport(report audit.Report, auditLevel string, failing int) {
	fmt.Println("TSPack audit (OSV.dev)")
	fmt.Println()
	fmt.Printf("Locked packages: %d. Checked npm packages: %d.\n", report.LockedPackages, report.Packages)
	for _, coverage := range report.Coverage {
		switch coverage.Status {
		case audit.CoverageChecked:
			fmt.Printf("%s: %d checked.\n", coverage.Source, coverage.Packages)
		case audit.CoverageUnsupportedEcosystem:
			fmt.Printf("%s: %d not checked (unsupported ecosystem). %s\n", coverage.Source, coverage.Packages, coverage.Reason)
		case audit.CoverageUnknown:
			fmt.Printf("%s: %d not checked (coverage unknown). %s\n", coverage.Source, coverage.Packages, coverage.Reason)
		default:
			fmt.Printf("%s: %d not checked. %s\n", coverage.Source, coverage.Packages, coverage.Reason)
		}
	}
	if len(report.Findings) == 0 {
		if report.CoverageComplete {
			fmt.Println("No known vulnerabilities found in the fully checked lock graph.")
		} else {
			fmt.Println("No known vulnerabilities found in checked packages; audit coverage is incomplete.")
		}
		return
	}
	fmt.Printf("Found %d known vulnerabilities (%d meet audit level %s).\n", len(report.Findings), failing, auditLevel)
	for _, finding := range report.Findings {
		fmt.Printf("\n%s [%s] %s@%s\n", finding.ID, finding.Severity, finding.Package, finding.Version)
		if finding.Summary != "" {
			fmt.Printf("  %s\n", finding.Summary)
		}
		if len(finding.Aliases) > 0 {
			fmt.Printf("  aliases: %s\n", strings.Join(finding.Aliases, ", "))
		}
		if len(finding.Fixed) > 0 {
			fmt.Printf("  fixed: %s\n", strings.Join(finding.Fixed, ", "))
		} else {
			fmt.Println("  fixed: no fixed version reported")
		}
		for _, path := range finding.Paths {
			fmt.Printf("  path: %s\n", strings.Join(path, " -> "))
		}
		if len(finding.References) > 0 {
			fmt.Printf("  reference: %s\n", finding.References[0].URL)
		}
	}
}

func failAuditArgs(message string) {
	fmt.Fprintln(os.Stderr, "TSPACK_AUDIT_INVALID_ARGS:", message)
	fmt.Fprintln(os.Stderr, "usage: tspack audit [--root path] [--audit-level any|low|moderate|high|critical] [--json]")
	exit(2)
}

func failAudit(code, message string, jsonOutput bool) {
	if jsonOutput {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": false, "source": "OSV.dev", "diagnostics": []map[string]string{{"code": code, "severity": "error", "message": message}}})
	} else {
		fmt.Fprintf(os.Stderr, "%s: %s\n", code, message)
	}
	exit(1)
}
