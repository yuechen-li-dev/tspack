package resolver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/capability"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

func (r *resolverState) resolveNonNPMDependency(ctx context.Context, dep *graph.DependencyNode, from, kind string) {
	switch dep.Source.Kind {
	case "path":
		r.resolvePathDependency(dep, from, kind)
	case "workspace":
		r.resolveWorkspaceDependency(dep, from, kind)
	case "git":
		r.resolveGitDependency(ctx, dep, from, kind)
	default:
		r.result.Diagnostics = append(r.result.Diagnostics, dWarn("TSPACK_RESOLVE_NON_NPM_SKIPPED", "non-npm dependency source skipped", dep.Source.Kind, dep.Key))
	}
}

func (r *resolverState) resolvePathDependency(dep *graph.DependencyNode, from, kind string) {
	if filepath.IsAbs(dep.Source.Path) {
		r.result.Diagnostics = append(r.result.Diagnostics, dErr("TSPACK_RESOLVE_PATH_INVALID", "absolute path is not allowed", dep.Source.Path))
		return
	}
	base := dep.Package.Root
	if base == "" {
		base = "."
	}
	resolved := filepath.Clean(filepath.Join(base, dep.Source.Path))
	if strings.HasPrefix(resolved, "..") {
		r.result.Diagnostics = append(r.result.Diagnostics, dErr("TSPACK_RESOLVE_PATH_OUTSIDE_WORKSPACE", "resolved path escapes workspace", dep.Source.Path, dep.Package.Name))
		return
	}
	absRoot := filepath.Clean(filepath.Join(r.opts.RootDir, base))
	absPath := filepath.Clean(filepath.Join(r.opts.RootDir, resolved))
	if !strings.HasPrefix(absPath, absRoot) {
		r.result.Diagnostics = append(r.result.Diagnostics, dErr("TSPACK_RESOLVE_PATH_OUTSIDE_WORKSPACE", "resolved path escapes workspace", dep.Source.Path, dep.Package.Name))
		return
	}
	pkg, ok := r.localPackageMetadata(absPath, dep.Key, "TSPACK_RESOLVE_PATH_PACKAGE_JSON_INVALID")
	if !ok {
		return
	}
	hash, ok := hashDirectory(absPath)
	if !ok {
		r.result.Diagnostics = append(r.result.Diagnostics, dErr("TSPACK_RESOLVE_PATH_NOT_FOUND", "path dependency not found", dep.Source.Path))
		return
	}
	rel := filepath.ToSlash(filepath.Clean(resolved))
	id := fmt.Sprintf("path:%s#%s", rel, hash)
	r.addPackageAndEdge(lockfile.Package{ID: id, Name: pkg.Name, Version: pkg.Version, Source: "path", Path: rel, Hash: "sha256:" + hash, Capabilities: capability.FromPackageJSONScripts(pkg.Scripts)}, from, kind, dep.Optional)
}

func (r *resolverState) resolveWorkspaceDependency(dep *graph.DependencyNode, from, kind string) {
	name := dep.Source.Name
	if name == "" {
		name = dep.Source.Package
	}
	if name == "" {
		name = dep.Key
	}
	target, ok := r.graph.Package(name)
	if !ok {
		r.result.Diagnostics = append(r.result.Diagnostics, dErr("TSPACK_RESOLVE_WORKSPACE_PACKAGE_NOT_FOUND", "workspace package not found", name))
		return
	}
	absRoot := filepath.Clean(filepath.Join(r.opts.RootDir, target.Root))
	hash, ok := hashDirectory(absRoot)
	if !ok {
		r.result.Diagnostics = append(r.result.Diagnostics, dErr("TSPACK_RESOLVE_WORKSPACE_ROOT_INVALID", "workspace package root is invalid", name, target.Root))
		return
	}
	id := fmt.Sprintf("workspace:%s#%s", target.Name, hash)
	r.addPackageAndEdge(lockfile.Package{ID: id, Name: target.Name, Version: target.Version, Source: "workspace", Workspace: target.Name, Path: filepath.ToSlash(filepath.Clean(target.Root)), Hash: "sha256:" + hash}, from, kind, dep.Optional)
}

func (r *resolverState) resolveGitDependency(ctx context.Context, dep *graph.DependencyNode, from, kind string) {
	repo := dep.Source.Repo
	if repo == "" {
		r.result.Diagnostics = append(r.result.Diagnostics, dErr("TSPACK_RESOLVE_GIT_REPO_INVALID", "git repo is required", dep.Key))
		return
	}
	if strings.HasPrefix(repo, "file://") {
		repo = strings.TrimPrefix(repo, "file://")
	}
	if _, err := os.Stat(repo); err != nil {
		r.result.Diagnostics = append(r.result.Diagnostics, dErr("TSPACK_RESOLVE_GIT_REPO_INVALID", "git repo path invalid", repo))
		return
	}
	ref := dep.Source.Rev
	if ref == "" {
		ref = dep.Source.Tag
	}
	if ref == "" {
		ref = dep.Source.Ref
	}
	if ref == "" {
		ref = dep.Source.Branch
	}
	if ref == "" {
		ref = "HEAD"
	}
	commit, err := gitOut(ctx, repo, "rev-parse", ref)
	if err != nil {
		r.result.Diagnostics = append(r.result.Diagnostics, dErr("TSPACK_RESOLVE_GIT_REF_NOT_FOUND", "git ref not found", ref, err.Error()))
		return
	}
	treeHash, err := gitOut(ctx, repo, "rev-parse", strings.TrimSpace(commit)+"^{tree}")
	if err != nil {
		r.result.Diagnostics = append(r.result.Diagnostics, dErr("TSPACK_RESOLVE_GIT_COMMAND_FAILED", "failed to resolve git tree hash", err.Error()))
		return
	}
	pkg, ok := r.localPackageMetadata(repo, dep.Key, "TSPACK_RESOLVE_GIT_PACKAGE_JSON_INVALID")
	if !ok {
		return
	}
	id := fmt.Sprintf("git:%s#%s", filepath.ToSlash(repo), strings.TrimSpace(commit))
	r.addPackageAndEdge(lockfile.Package{ID: id, Name: pkg.Name, Version: pkg.Version, Source: "git", Repo: filepath.ToSlash(repo), Rev: strings.TrimSpace(commit), TreeHash: strings.TrimSpace(treeHash), Capabilities: capability.FromPackageJSONScripts(pkg.Scripts)}, from, kind, dep.Optional)
}

type localPkgMeta struct {
	Name, Version string
	Scripts       map[string]string
}

func (r *resolverState) localPackageMetadata(root, fallbackName, invalidCode string) (localPkgMeta, bool) {
	b, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return localPkgMeta{Name: fallbackName}, true
	}
	var raw rawPackageJSON
	if err := json.Unmarshal(b, &raw); err != nil {
		r.result.Diagnostics = append(r.result.Diagnostics, dErr(invalidCode, "invalid package.json", root, err.Error()))
		return localPkgMeta{}, false
	}
	name := raw.Name
	if name == "" {
		name = fallbackName
	}
	return localPkgMeta{Name: name, Version: raw.Version, Scripts: stringScripts(raw.Scripts)}, true
}

func hashDirectory(root string) (string, bool) {
	h := sha256.New()
	files := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		if d.IsDir() && shouldSkipLocalArtifactDir(d.Name()) {
			return filepath.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.Type().IsRegular() {
			if shouldSkipLocalArtifactFile(d.Name()) {
				return nil
			}
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return "", false
	}
	sort.Strings(files)
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", false
		}
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

func shouldSkipLocalArtifactDir(name string) bool {
	switch name {
	case ".git", ".tspack", "node_modules", "tspack-artifacts":
		return true
	default:
		return false
	}
}

func shouldSkipLocalArtifactFile(name string) bool {
	switch name {
	case "ts-lock.toml":
		return true
	default:
		return false
	}
}

func (r *resolverState) addPackageAndEdge(pkg lockfile.Package, from, kind string, optional bool) {
	if !r.seenPkg[pkg.ID] {
		r.result.Lock.Packages = append(r.result.Lock.Packages, pkg)
		r.seenPkg[pkg.ID] = true
	}
	r.result.Lock.Edges = append(r.result.Lock.Edges, lockfile.Edge{From: from, To: pkg.ID, Kind: kind, Optional: optional})
}

func gitOut(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
