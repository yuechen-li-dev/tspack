package cli

import (
	"fmt"
	"strings"
)

func renderPackageLockEvidenceSection(builder *strings.Builder, evidence packageLockEvidence) {
	builder.WriteString("## Lockfile evidence\n")
	builder.WriteString("This section reports npm lockfile evidence for review only. It is not translated to `ts-lock.toml` and is not treated as TSPack truth.\n\n")

	switch evidence.Status {
	case packageLockEvidenceFound:
		builder.WriteString("- lockfile path: `")
		builder.WriteString(evidence.Path)
		builder.WriteString("`\n")
		builder.WriteString(fmt.Sprintf("- lockfileVersion: `%d`\n", evidence.Version))
		builder.WriteString(fmt.Sprintf("- package count: `%d`\n", evidence.PackageCount))
	case packageLockEvidenceSkipped:
		builder.WriteString("- lock evidence: skipped by `--no-lock-evidence`\n")
		builder.WriteString("\n")
		return
	case packageLockEvidenceInvalid:
		builder.WriteString("- lockfile path: `")
		builder.WriteString(evidence.Path)
		builder.WriteString("`\n")
		builder.WriteString("- lock evidence: invalid; ignored\n")
		if evidence.StatusReason != "" {
			builder.WriteString("- reason: ")
			builder.WriteString(evidence.StatusReason)
			builder.WriteString("\n")
		}
		writeLockWarnings(builder, evidence.Warnings)
		builder.WriteString("\n")
		return
	default:
		builder.WriteString("- lock evidence: not found\n")
		builder.WriteString("\n")
		return
	}

	if evidence.UnsupportedVersion {
		builder.WriteString("- notable warning: unsupported lockfileVersion; evidence is best effort\n")
	}
	writeLockWarnings(builder, evidence.Warnings)
	builder.WriteString("\n")

	renderDirectLockEvidence(builder, evidence.Direct)
	renderFanoutEvidence(builder, evidence.Fanout)
	renderLifecycleLockEvidence(builder, evidence.LifecycleScripts)
	renderBinaryLockEvidence(builder, evidence.Binaries)
	renderPeerLockEvidence(builder, evidence.PeerPackages)
	renderPlatformLockEvidence(builder, evidence.PlatformPackages)
	renderMismatchLockEvidence(builder, evidence)
}

func renderDirectLockEvidence(builder *strings.Builder, direct []directLockEvidence) {
	builder.WriteString("### Direct resolved packages\n")
	if len(direct) == 0 {
		builder.WriteString("No direct package.json dependencies were present.\n\n")
		return
	}
	builder.WriteString("| name | declared | resolved | kind | lock evidence |\n")
	builder.WriteString("|---|---|---|---|---|\n")
	for _, row := range direct {
		resolved := "missing"
		lockEvidence := "missing from lock evidence"
		if row.Found {
			resolved = defaultString(row.Resolved, "<unversioned>")
			parts := []string{row.LockPath}
			if row.Integrity != "" {
				parts = append(parts, "integrity "+row.Integrity)
			}
			if row.ResolvedURL != "" {
				parts = append(parts, row.ResolvedURL)
			}
			lockEvidence = strings.Join(parts, "; ")
		}
		builder.WriteString("| `")
		builder.WriteString(row.Name)
		builder.WriteString("` | `")
		builder.WriteString(row.DeclaredRange)
		builder.WriteString("` | `")
		builder.WriteString(resolved)
		builder.WriteString("` | `")
		builder.WriteString(row.Kind)
		builder.WriteString("` | `")
		builder.WriteString(lockEvidence)
		builder.WriteString("` |\n")
	}
	builder.WriteString("\n")
}

func renderFanoutEvidence(builder *strings.Builder, fanout []fanoutEvidence) {
	builder.WriteString("### Transitive fanout\n")
	if len(fanout) == 0 {
		builder.WriteString("No direct lock entries were available for fanout evidence.\n\n")
		return
	}
	builder.WriteString("Approximate fanout is computed from lock dependency names and may over-count when multiple versions exist.\n\n")
	builder.WriteString("| package | reachable packages | top dependencies | notes |\n")
	builder.WriteString("|---|---:|---|---|\n")
	for _, row := range fanout {
		note := ""
		if row.Large {
			note = row.Name + " pulls in " + fmt.Sprintf("%d", row.ReachableCount) + " transitive packages"
		}
		builder.WriteString("| `")
		builder.WriteString(row.Name)
		builder.WriteString("` | ")
		builder.WriteString(fmt.Sprintf("%d", row.ReachableCount))
		builder.WriteString(" | `")
		builder.WriteString(strings.Join(row.TopDeps, "`, `"))
		builder.WriteString("` | ")
		builder.WriteString(note)
		builder.WriteString(" |\n")
	}
	builder.WriteString("\n")
}

func renderLifecycleLockEvidence(builder *strings.Builder, scripts []lifecycleScriptEvidence) {
	builder.WriteString("### Lifecycle capabilities\n")
	if len(scripts) == 0 {
		builder.WriteString("No locked package lifecycle scripts were detected.\n\n")
		return
	}
	builder.WriteString("Lifecycle scripts are executable capabilities. TSPack does not execute them by default and this migration did not execute them.\n\n")
	builder.WriteString("| package | script | command | direct/transitive | lock path |\n")
	builder.WriteString("|---|---|---|---|---|\n")
	for _, row := range scripts {
		builder.WriteString("| `")
		builder.WriteString(packageVersionLabel(row.Name, row.Version))
		builder.WriteString("` | `")
		builder.WriteString(row.ScriptName)
		builder.WriteString("` | `")
		builder.WriteString(row.Command)
		builder.WriteString("` | `")
		builder.WriteString(directLabel(row.Direct))
		builder.WriteString("` | `")
		builder.WriteString(row.LockPath)
		builder.WriteString("` |\n")
	}
	builder.WriteString("\n")
}

func renderBinaryLockEvidence(builder *strings.Builder, binaries []binaryEvidence) {
	builder.WriteString("### Binary packages\n")
	if len(binaries) == 0 {
		builder.WriteString("No locked package `bin` fields were detected.\n\n")
		return
	}
	builder.WriteString("| package | bins | direct/transitive | lock path |\n")
	builder.WriteString("|---|---|---|---|\n")
	for _, row := range binaries {
		builder.WriteString("| `")
		builder.WriteString(packageVersionLabel(row.Name, row.Version))
		builder.WriteString("` | `")
		builder.WriteString(strings.Join(row.Bins, "`, `"))
		builder.WriteString("` | `")
		builder.WriteString(directLabel(row.Direct))
		builder.WriteString("` | `")
		builder.WriteString(row.LockPath)
		builder.WriteString("` |\n")
	}
	builder.WriteString("\n")
}

func renderPeerLockEvidence(builder *strings.Builder, peers []peerPackageEvidence) {
	builder.WriteString("### Peer evidence\n")
	if len(peers) == 0 {
		builder.WriteString("No locked package peer metadata was detected.\n\n")
		return
	}
	builder.WriteString("| package | peerDependencies | peerDependenciesMeta | direct/transitive | lock path |\n")
	builder.WriteString("|---|---|---|---|---|\n")
	for _, row := range peers {
		builder.WriteString("| `")
		builder.WriteString(packageVersionLabel(row.Name, row.Version))
		builder.WriteString("` | `")
		builder.WriteString(strings.Join(row.PeerDependencies, "`, `"))
		builder.WriteString("` | `")
		builder.WriteString(strings.Join(row.PeerDependenciesMeta, "`, `"))
		builder.WriteString("` | `")
		builder.WriteString(directLabel(row.Direct))
		builder.WriteString("` | `")
		builder.WriteString(row.LockPath)
		builder.WriteString("` |\n")
	}
	builder.WriteString("\n")
}

func renderPlatformLockEvidence(builder *strings.Builder, platforms []platformPackageEvidence) {
	builder.WriteString("### Platform/native package evidence\n")
	if len(platforms) == 0 {
		builder.WriteString("No likely platform/native locked packages were detected.\n\n")
		return
	}
	builder.WriteString("| package | reasons | direct/transitive | lock path |\n")
	builder.WriteString("|---|---|---|---|\n")
	for _, row := range platforms {
		builder.WriteString("| `")
		builder.WriteString(packageVersionLabel(row.Name, row.Version))
		builder.WriteString("` | `")
		builder.WriteString(strings.Join(row.Reasons, "`, `"))
		builder.WriteString("` | `")
		builder.WriteString(directLabel(row.Direct))
		builder.WriteString("` | `")
		builder.WriteString(row.LockPath)
		builder.WriteString("` |\n")
	}
	builder.WriteString("\n")
}

func renderMismatchLockEvidence(builder *strings.Builder, evidence packageLockEvidence) {
	builder.WriteString("### Notable warnings and mismatches\n")
	wrote := false
	if len(evidence.MissingDirect) > 0 {
		builder.WriteString("- package.json direct dependencies missing from lock evidence: `")
		builder.WriteString(strings.Join(evidence.MissingDirect, "`, `"))
		builder.WriteString("`.\n")
		wrote = true
	}
	if len(evidence.RootUndeclaredDirect) > 0 {
		builder.WriteString("- lock root dependencies not declared in package.json fields consumed by migrate: `")
		builder.WriteString(strings.Join(evidence.RootUndeclaredDirect, "`, `"))
		builder.WriteString("`.\n")
		wrote = true
	}
	for _, duplicate := range evidence.DuplicateVersions {
		builder.WriteString("- duplicate locked versions for `")
		builder.WriteString(duplicate.Name)
		builder.WriteString("`: `")
		builder.WriteString(strings.Join(duplicate.Versions, "`, `"))
		builder.WriteString("` at `")
		builder.WriteString(strings.Join(duplicate.Paths, "`, `"))
		builder.WriteString("`.\n")
		wrote = true
	}
	if len(evidence.TypePackageNames) > 0 {
		builder.WriteString("- lock evidence includes type packages needing type-policy review: `")
		builder.WriteString(strings.Join(evidence.TypePackageNames, "`, `"))
		builder.WriteString("`.\n")
		wrote = true
	}
	if !wrote {
		builder.WriteString("No direct missing packages, root declaration mismatches, duplicate versions, or @types package notes were detected.\n")
	}
	builder.WriteString("\n")
}

func writeLockWarnings(builder *strings.Builder, warnings []string) {
	for _, warning := range warnings {
		builder.WriteString("- notable warning: ")
		builder.WriteString(warning)
		builder.WriteString("\n")
	}
}

func packageVersionLabel(name string, version string) string {
	if version == "" {
		return name
	}
	return name + "@" + version
}

func directLabel(direct bool) string {
	if direct {
		return "direct"
	}
	return "transitive"
}

func migrationDiagnosticsFromLockEvidence(evidence packageLockEvidence) []migrationDiagnostic {
	var diagnostics []migrationDiagnostic
	if evidence.Status == packageLockEvidenceInvalid {
		diagnostics = append(diagnostics, migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_PACKAGE_LOCK_INVALID",
			Message: "implicit package-lock.json evidence could not be parsed and was ignored",
			Details: []string{
				"packageLockPath: " + evidence.Path,
				"reason: " + evidence.StatusReason,
			},
			Fixes: []string{"Regenerate package-lock with npm outside tspack or use --no-lock-evidence."},
		})
	}
	if evidence.UnsupportedVersion {
		diagnostics = append(diagnostics, migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_PACKAGE_LOCK_UNSUPPORTED_VERSION",
			Message: "package-lock.json lockfileVersion is not npm v2/v3; evidence is best effort",
			Details: []string{
				"packageLockPath: " + evidence.Path,
				fmt.Sprintf("lockfileVersion: %d", evidence.Version),
			},
			Fixes: []string{"Regenerate package-lock with npm outside tspack or use --no-lock-evidence."},
		})
	}
	return diagnostics
}

func migrationLockInputSummary(evidence packageLockEvidence) string {
	switch evidence.Status {
	case packageLockEvidenceFound:
		return "`" + evidence.Path + "`"
	case packageLockEvidenceSkipped:
		return "skipped by `--no-lock-evidence`"
	case packageLockEvidenceInvalid:
		return "`" + evidence.Path + "` invalid; ignored"
	default:
		return "not found"
	}
}

func hasLargeFanoutEvidence(fanout []fanoutEvidence) bool {
	for _, row := range fanout {
		if row.Large {
			return true
		}
	}
	return false
}

func printMigrationLockDryRun(evidence packageLockEvidence) {
	fmt.Println("Lock evidence:")
	switch evidence.Status {
	case packageLockEvidenceFound:
		fmt.Println("  package-lock.json: found")
		fmt.Printf("  lockfileVersion: %d\n", evidence.Version)
		fmt.Printf("  packages: %d\n", evidence.PackageCount)
		fmt.Printf("  lifecycle scripts: %d\n", len(evidence.LifecycleScripts))
		fmt.Printf("  package bins: %d\n", len(evidence.Binaries))
	case packageLockEvidenceSkipped:
		fmt.Println("  package-lock.json: skipped by --no-lock-evidence")
	case packageLockEvidenceInvalid:
		fmt.Println("  package-lock.json: invalid; ignored")
	default:
		fmt.Println("  package-lock.json: not found")
	}
}
