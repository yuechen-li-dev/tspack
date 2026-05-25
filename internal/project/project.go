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
	"github.com/tspack/tspack/internal/pack"
	"github.com/tspack/tspack/internal/resolver"
	"github.com/tspack/tspack/internal/store"
	"github.com/tspack/tspack/internal/why"
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
	DryRun      *UpdateDryRunResult
	PackResult  *PackResult
	WhyResult   *why.Result
	Outdated    *OutdatedResult
}
type UpdateDryRunResult struct {
	Changed bool
	Summary UpdateDiffSummary
}
type UpdateDiffSummary struct {
	Added, Removed, Changed, Unchanged int
}

type WhyOptions struct {
	Query       string
	PackageName string
}

type PackOptions struct {
	OutputDir   string
	PackageName string
	DryRun      bool
}
type PackResult struct {
	Artifacts []PackArtifact
	Preview   []PackFile
}
type PackArtifact struct {
	PackageName string
	Version     string
	Path        string
	Hash        string
	Size        int64
}
type PackFile struct {
	PackageName string
	SourcePath  string
	ArchivePath string
	Size        int64
	Reason      string
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
			out = append(out, lockfile.CheckVersionConflicts(lf).Diagnostics...)
			for _, pkg := range lf.Packages {
				for _, cap := range pkg.Capabilities {
					if cap.Kind == "lifecycle-script" {
						out = append(out, diag.Diagnostic{Code: "TSPACK_CAPABILITY_LIFECYCLE_SCRIPT_PRESENT", Severity: diag.SeverityWarning, Message: "lockfile package has lifecycle-script capability", Details: []string{pkg.ID, cap.Detail}})
					}
				}
			}
		}
	} else if os.IsNotExist(err) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_CHECK_LOCKFILE_MISSING", Severity: diag.SeverityWarning, Message: "lockfile is missing"})
	}
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out}
}

func Update(opts Options) Result {
	return updateWithMode(opts, false)
}

func UpdateDryRun(opts Options) Result {
	return updateWithMode(opts, true)
}

func updateWithMode(opts Options, dryRun bool) Result {
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
		client = resolver.NewHTTPRegistryClient("")
	}
	res := resolver.Resolve(context.Background(), resolver.ResolverOptions{Mode: resolver.ResolveModeUpdate, Client: client, RootDir: opts.RootDir}, resolver.ResolveRequest{Graph: g, ExistingLock: old})
	out = append(out, res.Diagnostics...)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	d := lockfile.DiffLockfiles(old, res.Lock)
	if dryRun {
		summary := UpdateDiffSummary{
			Added:     len(d.PackagesAdded),
			Removed:   len(d.PackagesRemoved),
			Changed:   len(d.PackagesChanged),
			Unchanged: len(res.Lock.Packages) - len(d.PackagesAdded) - len(d.PackagesChanged),
		}
		if summary.Unchanged < 0 {
			summary.Unchanged = 0
		}
		return Result{Diagnostics: out, LockDiff: &d, DryRun: &UpdateDryRunResult{Changed: summary.Added > 0 || summary.Removed > 0 || summary.Changed > 0, Summary: summary}}
	}
	st, err := store.Open(opts.StoreRoot)
	if err != nil {
		out = append(out, errDiag("TSPACK_UPDATE_STORE_OPEN_FAILED", "failed to open store", err.Error()))
		return Result{Diagnostics: out}
	}
	for i := range res.Lock.Packages {
		pkg := &res.Lock.Packages[i]
		if pkg.Hash != "" && st.Has(pkg.Hash) {
			continue
		}
		switch pkg.Source {
		case "npm":
			body, fetchErr := client.Tarball(context.Background(), findTarballURL(pkg, client))
			if fetchErr != nil {
				out = append(out, errDiag("TSPACK_RESOLVE_NPM_TARBALL_FETCH_FAILED", "failed to fetch npm tarball", pkg.ID, fetchErr.Error()))
				continue
			}
			ref, diags := st.PutArtifact(store.Artifact{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, Hash: pkg.Hash, Integrity: pkg.Integrity, Kind: store.ArtifactNPMTarball, Bytes: body, Metadata: store.PackageMetadata{Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, PackageID: pkg.ID, Integrity: pkg.Integrity, Capabilities: pkg.Capabilities}})
			out = append(out, diags...)
			if len(diags) == 0 {
				pkg.Hash = ref.Hash
			}
		case "path", "workspace":
			abs := filepath.Join(opts.RootDir, filepath.FromSlash(pkg.Path))
			ref, diags := st.PutArtifact(store.Artifact{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, Hash: pkg.Hash, Kind: store.ArtifactPathTree, RootDir: abs, Metadata: store.PackageMetadata{Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, PackageID: pkg.ID, Capabilities: pkg.Capabilities}})
			out = append(out, diags...)
			if len(diags) == 0 {
				pkg.Hash = ref.Hash
			}
		}
	}
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
	return Result{Diagnostics: out, LockDiff: &d}
}

func Pack(opts Options, packOpts PackOptions) Result {
	_, g, out := loadManifestAndGraph(opts)
	out = append(out, check.CheckPackage(check.CheckOptions{RootDir: opts.RootDir, Graph: g}).Diagnostics...)
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		lf, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_PACK_LOCKFILE_STALE", "failed to read lockfile", e.Error()))
		} else {
			out = append(out, d...)
			out = append(out, lockfile.CheckGraphConsistency(g, lf).Diagnostics...)
		}
	} else if os.IsNotExist(err) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_CHECK_LOCKFILE_MISSING", Severity: diag.SeverityWarning, Message: "lockfile is missing"})
	}
	if hasErrors(out) {
		diag.SortDiagnostics(out)
		return Result{Diagnostics: out}
	}
	pkgs := g.AllPackages()
	if packOpts.PackageName != "" {
		p, ok := g.Package(packOpts.PackageName)
		if !ok {
			return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_PACK_PACKAGE_NOT_FOUND", "package not found", packOpts.PackageName)}}
		}
		pkgs = []*graph.PackageNode{p}
	}
	pr := &PackResult{}
	for _, p := range pkgs {
		r := pack.Pack(opts.RootDir, p, pack.Options{OutputDir: packOpts.OutputDir, DryRun: packOpts.DryRun})
		out = append(out, r.Diagnostics...)
		for _, a := range r.Artifacts {
			pr.Artifacts = append(pr.Artifacts, PackArtifact(a))
		}
		for _, f := range r.Preview {
			pr.Preview = append(pr.Preview, PackFile(f))
		}
	}
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out, PackResult: pr}
}

func Why(opts Options, whyOpts WhyOptions) Result {
	_, g, out := loadManifestAndGraph(opts)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	var lf *lockfile.Lockfile
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		parsed, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_WHY_LOCKFILE_INVALID", "failed to read lockfile", e.Error()))
		} else {
			lf = parsed
			out = append(out, d...)
			out = append(out, lockfile.CheckGraphConsistency(g, lf).Diagnostics...)
		}
	} else if os.IsNotExist(err) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_WHY_LOCKFILE_MISSING", Severity: diag.SeverityWarning, Message: "lockfile is missing"})
	}
	wr := why.Analyze(g, lf, why.Options{Query: whyOpts.Query, PackageName: whyOpts.PackageName})
	out = append(out, wr.Diagnostics...)
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out, WhyResult: &wr}
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

func findTarballURL(pkg *lockfile.Package, client resolver.NPMRegistryClient) string {
	meta, err := client.PackageMetadata(context.Background(), pkg.Name)
	if err != nil {
		return ""
	}
	pv, ok := meta.Versions[pkg.Version]
	if !ok {
		return ""
	}
	return pv.Dist.Tarball
}

func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}
