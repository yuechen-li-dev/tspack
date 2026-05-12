package project

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tspack/tspack/internal/check"
	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/manifest"
	"github.com/tspack/tspack/internal/materialize"
	"github.com/tspack/tspack/internal/resolver"
	"github.com/tspack/tspack/internal/store"
)

type Options struct {
	RootDir, ManifestPath, LockfilePath, StoreRoot string
	ManifestIRPath                                 string
	FrontendCLIPath                                string
	ResolverClient                                 resolver.NPMRegistryClient
}
type Result struct {
	Diagnostics []diag.Diagnostic
	LockDiff    *lockfile.Diff
}

func DefaultOptions(root string) Options {
	if root == "" {
		root = "."
	}
	return Options{RootDir: root, ManifestPath: filepath.Join(root, "manifest.tsx"), LockfilePath: filepath.Join(root, "ts-lock.toml"), StoreRoot: filepath.Join(root, ".tspack", "store")}
}

func Check(opts Options) Result {
	ir, g, out := loadManifestAndGraph(opts)
	_ = ir
	out = append(out, check.CheckPackage(check.CheckOptions{RootDir: opts.RootDir, Graph: g}).Diagnostics...)
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		lf, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_CHECK_FAILED", "failed to read lockfile", e.Error()))
		} else {
			out = append(out, d...)
			out = append(out, lockfile.CheckGraphConsistency(g, lf).Diagnostics...)
		}
	} else if os.IsNotExist(err) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_CHECK_LOCKFILE_MISSING", Severity: diag.SeverityWarning, Message: "lockfile is missing"})
	}
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out}
}

func Update(opts Options) Result {
	_, g, out := loadManifestAndGraph(opts)
	out = append(out, check.CheckPackage(check.CheckOptions{RootDir: opts.RootDir, Graph: g}).Diagnostics...)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	var old *lockfile.Lockfile
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		lf, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_UPDATE_RESOLVE_FAILED", "failed to read existing lockfile", e.Error()))
			return Result{Diagnostics: out}
		}
		out = append(out, d...)
		old = lf
	}
	client := opts.ResolverClient
	if client == nil {
		client = noopRegistryClient{}
	}
	res := resolver.ResolveNPM(context.Background(), resolver.ResolverOptions{Mode: resolver.ResolveModeUpdate, Client: client}, resolver.ResolveRequest{Graph: g, ExistingLock: old})
	out = append(out, res.Diagnostics...)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	b, e := lockfile.Marshal(res.Lock)
	if e != nil {
		out = append(out, errDiag("TSPACK_UPDATE_WRITE_FAILED", "failed to encode lockfile", e.Error()))
		return Result{Diagnostics: out}
	}
	if e = os.MkdirAll(filepath.Dir(opts.LockfilePath), 0o755); e != nil {
		out = append(out, errDiag("TSPACK_UPDATE_WRITE_FAILED", "failed to create lockfile dir", e.Error()))
		return Result{Diagnostics: out}
	}
	if e = os.WriteFile(opts.LockfilePath, b, 0o644); e != nil {
		out = append(out, errDiag("TSPACK_UPDATE_WRITE_FAILED", "failed to write lockfile", e.Error()))
	}
	d := lockfile.DiffLockfiles(old, res.Lock)
	return Result{Diagnostics: out, LockDiff: &d}
}

func Sync(opts Options, clean bool) Result {
	_, g, out := loadManifestAndGraph(opts)
	_ = g
	lf, d, e := lockfile.LoadFile(opts.LockfilePath)
	if os.IsNotExist(e) {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_SYNC_LOCKFILE_MISSING", Severity: diag.SeverityError, Message: "lockfile is required; run tspack update"}}}
	}
	if e != nil {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_SYNC_LOCKFILE_STALE", "failed to read lockfile", e.Error())}}
	}
	out = append(out, d...)
	out = append(out, lockfile.CheckGraphConsistency(g, lf).Diagnostics...)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	st, err := store.Open(opts.StoreRoot)
	if err != nil {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_SYNC_STORE_ARTIFACT_MISSING", "failed to open store", err.Error())}}
	}
	mat := materialize.NodeModulesMaterializer{}
	mr := mat.Materialize(context.Background(), materialize.Request{WorkspaceRoot: opts.RootDir, Graph: g, Lock: lf, Store: st, Options: materialize.Options{Clean: clean}})
	out = append(out, mr.Diagnostics...)
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out}
}

func loadManifestAndGraph(opts Options) (*manifest.ManifestIR, *graph.WorkspaceGraph, []diag.Diagnostic) {
	ir, d := loadManifestIR(opts)
	if len(d) > 0 {
		return nil, nil, d
	}
	g, gd := graph.Build(ir)
	return ir, g, append([]diag.Diagnostic{}, gd...)
}

func loadManifestIR(opts Options) (*manifest.ManifestIR, []diag.Diagnostic) {
	if opts.ManifestIRPath != "" {
		b, err := os.ReadFile(opts.ManifestIRPath)
		if err != nil {
			return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "failed to read manifest IR", err.Error())}
		}
		return manifest.LoadBytes(opts.ManifestIRPath, b)
	}
	cliPath := opts.FrontendCLIPath
	if cliPath == "" {
		cliPath = filepath.Join("manifest-frontend", "dist", "src", "cli.js")
	}
	if _, err := os.Stat(cliPath); err != nil {
		return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "manifest frontend CLI not found; run `cd manifest-frontend && npm run build`", cliPath)}
	}
	cmd := exec.Command("node", cliPath, opts.ManifestPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, []diag.Diagnostic{{Code: "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", Severity: diag.SeverityError, Message: "manifest frontend failed", Details: []string{err.Error(), stderr.String()}}}
	}
	var parsed struct {
		OK          bool              `json:"ok"`
		IR          json.RawMessage   `json:"ir"`
		Diagnostics []diag.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "invalid frontend JSON", err.Error())}
	}
	if !parsed.OK {
		if len(parsed.Diagnostics) == 0 {
			return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "manifest frontend returned failure")}
		}
		return nil, parsed.Diagnostics
	}
	ir, d := manifest.LoadBytes(opts.ManifestPath, parsed.IR)
	return ir, d
}

func errDiag(code, msg string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: msg, Details: details}
}

func FormatResult(r Result) string { return fmt.Sprintf("diagnostics=%d", len(r.Diagnostics)) }

type noopRegistryClient struct{}

func (noopRegistryClient) PackageMetadata(_ context.Context, name string) (*resolver.PackageMetadata, error) {
	return nil, fmt.Errorf("npm registry client not configured for package %s", name)
}
func (noopRegistryClient) Tarball(_ context.Context, url string) ([]byte, error) {
	return nil, fmt.Errorf("npm tarball fetch not configured: %s", url)
}

func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}
