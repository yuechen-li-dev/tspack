package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
)

// CopelandArtifactManifest is deliberately narrow: TSPack receives already
// evaluated immutable bytes and owns the only filesystem materialization step.
type CopelandArtifactManifest struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Template      string                 `json:"template"`
	Files         []CopelandArtifactFile `json:"files"`
}

type CopelandArtifactFile struct {
	Path          string  `json:"path"`
	Kind          string  `json:"kind"`
	SHA256        string  `json:"sha256"`
	ContentBase64 *string `json:"contentBase64"`
}

func runMaterializeTreeCommand(args []string) {
	manifestPath := ""
	outputRoot := ""
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--manifest":
			index++
			if index >= len(args) {
				materializeTreeError("TSPACK_TEMPLATE_MANIFEST_REQUIRED", "--manifest requires a path")
			}
			manifestPath = args[index]
		case "--output":
			index++
			if index >= len(args) {
				materializeTreeError("TSPACK_TEMPLATE_OUTPUT_REQUIRED", "--output requires a path")
			}
			outputRoot = args[index]
		default:
			materializeTreeError("TSPACK_TEMPLATE_UNKNOWN_FLAG", "unknown materialize-tree flag: "+args[index])
		}
	}
	if manifestPath == "" || outputRoot == "" {
		materializeTreeError("TSPACK_TEMPLATE_ARGUMENTS_REQUIRED", "materialize-tree requires --manifest and --output")
	}

	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		materializeTreeError("TSPACK_TEMPLATE_MANIFEST_READ_FAILED", err.Error())
	}
	var manifest CopelandArtifactManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		materializeTreeError("TSPACK_TEMPLATE_MANIFEST_INVALID", err.Error())
	}
	if manifest.SchemaVersion != 1 || strings.TrimSpace(manifest.Template) == "" {
		materializeTreeError("TSPACK_TEMPLATE_MANIFEST_INVALID", "expected schemaVersion 1 and a non-empty template name")
	}

	output, err := filepath.Abs(outputRoot)
	if err != nil {
		materializeTreeError("TSPACK_TEMPLATE_OUTPUT_INVALID", err.Error())
	}
	if _, err := os.Stat(output); err == nil {
		materializeTreeError("TSPACK_TEMPLATE_OUTPUT_CONFLICT", "output root already exists; M0 materialization requires a new directory")
	} else if !os.IsNotExist(err) {
		materializeTreeError("TSPACK_TEMPLATE_OUTPUT_INVALID", err.Error())
	}

	sort.Slice(manifest.Files, func(left, right int) bool {
		return manifest.Files[left].Path < manifest.Files[right].Path
	})
	seen := map[string]bool{}
	for _, file := range manifest.Files {
		if !safeTemplateRelativePath(file.Path) {
			materializeTreeError("TSPACK_TEMPLATE_INVALID_PATH", "artifact path is not a safe relative path: "+file.Path)
		}
		if seen[file.Path] {
			materializeTreeError("TSPACK_TEMPLATE_DUPLICATE_PATH", "duplicate artifact path: "+file.Path)
		}
		seen[file.Path] = true
		if file.ContentBase64 == nil {
			materializeTreeError("TSPACK_TEMPLATE_CONTENT_REQUIRED", "artifact content is required for materialization: "+file.Path)
		}
		bytes, err := base64.StdEncoding.DecodeString(*file.ContentBase64)
		if err != nil {
			materializeTreeError("TSPACK_TEMPLATE_CONTENT_INVALID", file.Path+": "+err.Error())
		}
		hash := sha256.Sum256(bytes)
		if !strings.EqualFold(hex.EncodeToString(hash[:]), file.SHA256) {
			materializeTreeError("TSPACK_TEMPLATE_HASH_MISMATCH", "artifact content hash does not match manifest: "+file.Path)
		}
	}

	parent := filepath.Dir(output)
	stage, err := os.MkdirTemp(parent, ".tspack-template-stage-")
	if err != nil {
		materializeTreeError("TSPACK_TEMPLATE_STAGE_FAILED", err.Error())
	}
	defer os.RemoveAll(stage)
	for _, file := range manifest.Files {
		bytes, _ := base64.StdEncoding.DecodeString(*file.ContentBase64)
		target := filepath.Join(stage, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			materializeTreeError("TSPACK_TEMPLATE_STAGE_FAILED", err.Error())
		}
		if err := os.WriteFile(target, bytes, 0o644); err != nil {
			materializeTreeError("TSPACK_TEMPLATE_STAGE_FAILED", err.Error())
		}
	}
	if err := os.Rename(stage, output); err != nil {
		materializeTreeError("TSPACK_TEMPLATE_COMMIT_FAILED", err.Error())
	}
	fmt.Printf("materialized %d file(s) from Copeland template %s into %s\n", len(manifest.Files), manifest.Template, output)
}

func safeTemplateRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "\\") {
		return false
	}
	clean := pathpkg.Clean(path)
	return clean == path && clean != "." && !strings.HasPrefix(clean, "../") && clean != ".."
}

func materializeTreeError(code string, message string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", code, message)
	os.Exit(1)
}
