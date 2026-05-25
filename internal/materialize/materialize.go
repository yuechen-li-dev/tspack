package materialize

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/store"
)

type Materializer interface {
	Materialize(ctx context.Context, req Request) Result
}

type Request struct {
	WorkspaceRoot string
	Graph         *graph.WorkspaceGraph
	Lock          *lockfile.Lockfile
	Store         *store.Store
	Options       Options
}

type Options struct {
	Clean    bool
	LinkMode LinkMode
}

type LinkMode string

const (
	LinkModeCopy     LinkMode = "copy"
	LinkModeSymlink  LinkMode = "symlink"
	LinkModeHardlink LinkMode = "hardlink"
	LinkModeAuto     LinkMode = "auto"
)

type Result struct {
	Diagnostics []diag.Diagnostic
	Written     []WrittenPath
}

type WrittenPath struct {
	Path      string
	Kind      string
	PackageID string
}

type NodeModulesMaterializer struct{}

const markerFile = ".tspack-materialized"

func (m NodeModulesMaterializer) Materialize(ctx context.Context, req Request) Result {
	_ = ctx
	out := Result{}
	if req.Lock == nil || req.Store == nil || req.WorkspaceRoot == "" {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_WRITE_FAILED", Severity: diag.SeverityError, Message: "materialize request missing required fields"})
		return out
	}
	mode := req.Options.LinkMode
	if mode == "" || mode == LinkModeAuto {
		mode = LinkModeCopy
	}
	if mode != LinkModeCopy {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_UNSUPPORTED_LINK_MODE", Severity: diag.SeverityError, Message: fmt.Sprintf("link mode %q is not implemented in M10", mode)})
		return finalize(out)
	}
	nmRoot := filepath.Join(req.WorkspaceRoot, "node_modules")
	if req.Options.Clean {
		if err := cleanNodeModules(nmRoot); err != nil {
			out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_CLEAN_REFUSED", Severity: diag.SeverityError, File: nmRoot, Message: err.Error()})
			return finalize(out)
		}
	}
	if err := os.MkdirAll(nmRoot, 0o755); err != nil {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_WRITE_FAILED", Severity: diag.SeverityError, File: nmRoot, Message: err.Error()})
		return finalize(out)
	}
	_ = os.WriteFile(filepath.Join(nmRoot, markerFile), []byte("generated_by=tspack\nmaterializer=node_modules\nversion=1\n"), 0o644)

	pkgs := map[string]lockfile.Package{}
	for _, p := range req.Lock.Packages {
		pkgs[p.ID] = p
	}
	edgesByFrom := map[string][]lockfile.Edge{}
	for _, e := range req.Lock.Edges {
		edgesByFrom[e.From] = append(edgesByFrom[e.From], e)
	}
	for k := range edgesByFrom {
		sort.SliceStable(edgesByFrom[k], func(i, j int) bool {
			if edgesByFrom[k][i].To != edgesByFrom[k][j].To {
				return edgesByFrom[k][i].To < edgesByFrom[k][j].To
			}
			return edgesByFrom[k][i].Kind < edgesByFrom[k][j].Kind
		})
	}

	rootEdges := collectRootEdges(req.Lock.Edges)
	rootVisible := map[string]lockfile.Package{}
	seen := map[string]struct{}{}
	for _, e := range rootEdges {
		pkg, ok := pkgs[e.To]
		if !ok {
			out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_EDGE_UNKNOWN_PACKAGE", Severity: diag.SeverityError, Message: "edge points to unknown package", Details: []string{e.From, e.To}})
			continue
		}
		if _, err := safePackagePath(nmRoot, pkg.Name); err != nil {
			out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_INVALID_PACKAGE_NAME", Severity: diag.SeverityError, Message: err.Error(), Details: []string{pkg.ID, pkg.Name}})
			continue
		}
		rootVisible[pkg.ID] = pkg
		materializePkg(req, &out, pkgs, edgesByFrom, pkg, nmRoot, seen)
	}
	materializeRootBins(req, &out, nmRoot, rootVisible)
	return finalize(out)
}

type packageJSON struct {
	Name string      `json:"name"`
	Bin  interface{} `json:"bin"`
}

func collectRootEdges(edges []lockfile.Edge) []lockfile.Edge {
	var out []lockfile.Edge
	for _, e := range edges {
		if strings.Contains(e.From, ":target:") || strings.HasSuffix(e.From, ":tool") {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func materializePkg(req Request, out *Result, pkgs map[string]lockfile.Package, edgesByFrom map[string][]lockfile.Edge, pkg lockfile.Package, parentNodeModules string, seen map[string]struct{}) {
	dest, err := safePackagePath(parentNodeModules, pkg.Name)
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_INVALID_DESTINATION", Severity: diag.SeverityError, Message: err.Error(), Details: []string{pkg.ID}})
		return
	}
	key := pkg.ID + "@" + dest
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	hash, ok := PackageStoreHash(pkg)
	if !ok {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_PACKAGE_HASH_MISSING", Severity: diag.SeverityError, Message: "package missing store hash", Details: []string{pkg.ID}})
		return
	}
	if d := req.Store.Verify(hash); len(d) > 0 {
		code := "TSPACK_MATERIALIZE_STORE_VERIFY_FAILED"
		if d[0].Code == "TSPACK_STORE_ARTIFACT_NOT_FOUND" {
			code = "TSPACK_MATERIALIZE_MISSING_STORE_ARTIFACT"
		}
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: "store artifact verification failed", Details: []string{pkg.ID, hash, d[0].Code}})
		return
	}
	ref, d := req.Store.Get(hash)
	if len(d) > 0 {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_MISSING_STORE_ARTIFACT", Severity: diag.SeverityError, Message: "missing store artifact", Details: []string{pkg.ID, hash}})
		return
	}
	if err := copyTree(ref.ExtractedPath, dest); err != nil {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_WRITE_FAILED", Severity: diag.SeverityError, Message: err.Error(), Details: []string{pkg.ID, dest}})
		return
	}
	out.Written = append(out.Written, WrittenPath{Path: dest, Kind: "package", PackageID: pkg.ID})

	childNM := filepath.Join(dest, "node_modules")
	for _, edge := range edgesByFrom[pkg.ID] {
		dep, ok := pkgs[edge.To]
		if !ok {
			out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_EDGE_UNKNOWN_PACKAGE", Severity: diag.SeverityError, Message: "edge points to unknown package", Details: []string{edge.From, edge.To}})
			continue
		}
		materializePkg(req, out, pkgs, edgesByFrom, dep, childNM, seen)
	}
}

func PackageStoreHash(pkg lockfile.Package) (string, bool) {
	if pkg.Hash != "" {
		return pkg.Hash, true
	}
	if pkg.TreeHash != "" {
		return pkg.TreeHash, true
	}
	return "", false
}

func safePackagePath(base, name string) (string, error) {
	if name == "" || strings.HasPrefix(name, ".") || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid package name %q", name)
	}
	parts := strings.Split(name, "/")
	if len(parts) > 2 || (strings.HasPrefix(name, "@") && len(parts) != 2) {
		return "", fmt.Errorf("invalid package name %q", name)
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.ContainsRune(p, filepath.Separator) {
			return "", fmt.Errorf("invalid package name %q", name)
		}
	}
	dest := filepath.Join(append([]string{base}, parts...)...)
	cleanBase := filepath.Clean(base)
	cleanDest := filepath.Clean(dest)
	if !strings.HasPrefix(cleanDest, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid destination for %q", name)
	}
	return dest, nil
}

func cleanNodeModules(nmRoot string) error {
	if _, err := os.Stat(nmRoot); os.IsNotExist(err) {
		return nil
	}
	marker := filepath.Join(nmRoot, markerFile)
	if _, err := os.Stat(marker); err != nil {
		return fmt.Errorf("refusing to clean unmanaged node_modules (missing %s)", markerFile)
	}
	return os.RemoveAll(nmRoot)
}

func copyTree(src, dest string) error {
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out := filepath.Join(dest, rel)
		if info.IsDir() {
			mode := info.Mode().Perm()
			if mode == 0 {
				mode = 0o755
			}
			return os.MkdirAll(out, mode)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		f, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, in)
		if err != nil {
			return err
		}
		return os.Chmod(out, mode)
	})
}

func materializeRootBins(req Request, out *Result, nmRoot string, pkgs map[string]lockfile.Package) {
	binsRoot := filepath.Join(nmRoot, ".bin")
	if err := os.RemoveAll(binsRoot); err != nil {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_BIN_WRITE_FAILED", Severity: diag.SeverityError, File: binsRoot, Message: err.Error()})
		return
	}
	if err := os.MkdirAll(binsRoot, 0o755); err != nil {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_BIN_WRITE_FAILED", Severity: diag.SeverityError, File: binsRoot, Message: err.Error()})
		return
	}
	type candidate struct{ pkgName, absPath, relPath string }
	candidates := map[string]candidate{}
	ids := make([]string, 0, len(pkgs))
	for id := range pkgs { ids = append(ids, id) }
	sort.Strings(ids)
	for _, id := range ids {
		pkg := pkgs[id]
		pkgRoot, err := safePackagePath(nmRoot, pkg.Name)
		if err != nil { continue }
		defs, diags := parsePackageBins(pkgRoot)
		if len(diags) > 0 {
			out.Diagnostics = append(out.Diagnostics, diags...)
			continue
		}
		sort.SliceStable(defs, func(i,j int) bool { return defs[i].name < defs[j].name })
		for _, def := range defs {
			target := filepath.Join(pkgRoot, filepath.FromSlash(def.relPath))
			if _, err := os.Stat(target); err != nil {
				out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_BIN_TARGET_MISSING", Severity: diag.SeverityError, File: filepath.Join(pkgRoot, "package.json"), Message: "bin target does not exist", Details: []string{def.name, def.relPath}})
				continue
			}
			if prev, ok := candidates[def.name]; ok && prev.absPath != target {
				out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_BIN_CONFLICT", Severity: diag.SeverityError, Message: "multiple packages expose the same bin", Details: []string{def.name, prev.pkgName, pkg.Name}})
				continue
			}
			candidates[def.name] = candidate{pkgName: pkg.Name, absPath: target, relPath: def.relPath}
		}
	}
	names := make([]string, 0, len(candidates))
	for name := range candidates { names = append(names, name) }
	sort.Strings(names)
	for _, name := range names {
		cand := candidates[name]
		targetRel := filepath.ToSlash(filepath.Join("..", cand.pkgName, filepath.FromSlash(cand.relPath)))
		binPath := filepath.Join(binsRoot, name)
		if runtime.GOOS == "windows" {
			content := "@ECHO off\r\nnode \"%~dp0\\" + strings.ReplaceAll(targetRel, "/", "\\") + "\" %*\r\n"
			if err := os.WriteFile(binPath+".cmd", []byte(content), 0o644); err != nil {
				out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_BIN_WRITE_FAILED", Severity: diag.SeverityError, File: binPath + ".cmd", Message: err.Error()})
			}
			continue
		}
		if err := os.Symlink(targetRel, binPath); err != nil {
			content := "#!/usr/bin/env sh\nDIR=\"$(CDPATH= cd -- \"$(dirname -- \"$0\")\" && pwd)\"\nexec \"$DIR/" + targetRel + "\" \"$@\"\n"
			if writeErr := os.WriteFile(binPath, []byte(content), 0o755); writeErr != nil {
				out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_BIN_WRITE_FAILED", Severity: diag.SeverityError, File: binPath, Message: writeErr.Error()})
			}
		}
		_ = os.Chmod(cand.absPath, 0o755)
	}
}

type binDef struct { name, relPath string }

func parsePackageBins(pkgRoot string) ([]binDef, []diag.Diagnostic) {
	p := filepath.Join(pkgRoot, "package.json")
	b, err := os.ReadFile(p)
	if err != nil { return nil, nil }
	var pkg packageJSON
	if err := json.Unmarshal(b, &pkg); err != nil { return nil, nil }
	switch v := pkg.Bin.(type) {
	case string:
		if pkg.Name == "" {
			return nil, []diag.Diagnostic{{Code: "TSPACK_MATERIALIZE_BIN_INVALID", Severity: diag.SeverityError, File: p, Message: "package bin string requires package name"}}
		}
		if err := validateBinPath(v); err != nil { return nil, []diag.Diagnostic{{Code: "TSPACK_MATERIALIZE_BIN_INVALID", Severity: diag.SeverityError, File: p, Message: err.Error(), Details: []string{pkg.Name, v}}} }
		return []binDef{{name: pkg.Name, relPath: v}}, nil
	case map[string]interface{}:
		out := []binDef{}
		for name, raw := range v {
			pathStr, ok := raw.(string)
			if !ok { return nil, []diag.Diagnostic{{Code: "TSPACK_MATERIALIZE_BIN_INVALID", Severity: diag.SeverityError, File: p, Message: "bin map entry must be a string", Details: []string{name}}} }
			if err := validateBinPath(pathStr); err != nil { return nil, []diag.Diagnostic{{Code: "TSPACK_MATERIALIZE_BIN_INVALID", Severity: diag.SeverityError, File: p, Message: err.Error(), Details: []string{name, pathStr}}} }
			out = append(out, binDef{name: name, relPath: pathStr})
		}
		return out, nil
	default:
		return nil, nil
	}
}

func validateBinPath(relPath string) error {
	if relPath == "" || filepath.IsAbs(relPath) {
		return fmt.Errorf("bin path must be relative")
	}
	clean := filepath.Clean(relPath)
	if clean == "." || strings.HasPrefix(clean, "..") || strings.Contains(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("bin path must stay inside package root")
	}
	return nil
}

func finalize(out Result) Result {
	sort.SliceStable(out.Written, func(i, j int) bool {
		if out.Written[i].Path != out.Written[j].Path {
			return out.Written[i].Path < out.Written[j].Path
		}
		return out.Written[i].PackageID < out.Written[j].PackageID
	})
	diag.SortDiagnostics(out.Diagnostics)
	return out
}
