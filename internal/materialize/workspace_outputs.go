package materialize

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
)

// ProjectWorkspaceBuildArtifacts refreshes the materialized root copy of a
// workspace package after a successful build. Store snapshots deliberately
// exclude generated output, but downstream workspace targets resolve through
// node_modules and must observe the artifacts produced by their prerequisites.
func ProjectWorkspaceBuildArtifacts(workspaceRoot string, packageRoot string, packageName string, declaredPatterns []string, artifactPaths []string) []diag.Diagnostic {
	materializedRoot, err := safePackagePath(filepath.Join(workspaceRoot, "node_modules"), packageName)
	if err != nil {
		return workspaceProjectionDiagnostics(packageName, err)
	}
	info, err := os.Stat(materializedRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return workspaceProjectionDiagnostics(packageName, err)
	}
	if !info.IsDir() {
		return workspaceProjectionDiagnostics(packageName, fmt.Errorf("materialized workspace package is not a directory: %s", materializedRoot))
	}
	if sameDirectory(packageRoot, materializedRoot) {
		return nil
	}

	for _, pattern := range declaredPatterns {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		destinationPattern, pathErr := workspaceOutputPath(materializedRoot, pattern)
		if pathErr != nil {
			return workspaceProjectionDiagnostics(packageName, pathErr)
		}
		matches := []string{destinationPattern}
		if strings.ContainsAny(pattern, "*?[") {
			matches, err = filepath.Glob(destinationPattern)
			if err != nil {
				return workspaceProjectionDiagnostics(packageName, err)
			}
		}
		for _, match := range matches {
			matchInfo, statErr := os.Stat(match)
			if os.IsNotExist(statErr) {
				continue
			}
			if statErr != nil {
				return workspaceProjectionDiagnostics(packageName, statErr)
			}
			if matchInfo.IsDir() {
				return workspaceProjectionDiagnostics(packageName, fmt.Errorf("declared workspace artifact matched a directory: %s", match))
			}
			if err := os.Remove(match); err != nil {
				return workspaceProjectionDiagnostics(packageName, err)
			}
		}
	}

	for _, artifactPath := range artifactPaths {
		relativePath, relErr := filepath.Rel(packageRoot, artifactPath)
		if relErr != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			return workspaceProjectionDiagnostics(packageName, fmt.Errorf("artifact escapes workspace package root: %s", artifactPath))
		}
		destinationPath, pathErr := workspaceOutputPath(materializedRoot, relativePath)
		if pathErr != nil {
			return workspaceProjectionDiagnostics(packageName, pathErr)
		}
		if err := copyWorkspaceOutput(artifactPath, destinationPath); err != nil {
			return workspaceProjectionDiagnostics(packageName, err)
		}
	}
	return nil
}

func workspaceOutputPath(root string, relativePath string) (string, error) {
	cleanPath := filepath.Clean(filepath.FromSlash(relativePath))
	if filepath.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace artifact path escapes package root: %s", relativePath)
	}
	return filepath.Join(root, cleanPath), nil
}

func sameDirectory(left string, right string) bool {
	leftInfo, leftStatErr := os.Stat(left)
	rightInfo, rightStatErr := os.Stat(right)
	if leftStatErr == nil && rightStatErr == nil && os.SameFile(leftInfo, rightInfo) {
		return true
	}
	leftPath, leftErr := filepath.EvalSymlinks(left)
	rightPath, rightErr := filepath.EvalSymlinks(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return filepath.Clean(leftPath) == filepath.Clean(rightPath)
}

func copyWorkspaceOutput(source string, destination string) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	info, err := sourceFile.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("workspace build artifact is not a regular file: %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	destinationFile, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(destinationFile, sourceFile)
	closeErr := destinationFile.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func workspaceProjectionDiagnostics(packageName string, err error) []diag.Diagnostic {
	return []diag.Diagnostic{{
		Code:     "TSPACK_WORKSPACE_BUILD_PROJECTION_FAILED",
		Severity: diag.SeverityError,
		Message:  "could not project workspace build artifacts into the materialized dependency environment",
		Details:  []string{packageName, err.Error()},
	}}
}
