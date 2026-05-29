package pack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
)

type Options struct {
	OutputDir string
	DryRun    bool
}
type Result struct {
	Diagnostics []diag.Diagnostic
	Artifacts   []Artifact
	Preview     []File
}
type Artifact struct {
	PackageName, Version, Path, Hash string
	Size                             int64
}
type File struct {
	PackageName, SourcePath, ArchivePath string
	Size                                 int64
	Reason                               string
}

type matchedFile struct {
	source, archive, reason string
	mode                    int64
	size                    int64
	data                    []byte
}

func Pack(root string, pkg *graph.PackageNode, opts Options) Result {
	var out Result
	if strings.TrimSpace(pkg.Version) == "" {
		out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_MISSING_PUBLISH_POLICY", "package version is required"))
		return out
	}
	if len(pkg.Publish.Include) == 0 {
		out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_MISSING_PUBLISH_POLICY", "publish.include is required"))
		return out
	}
	pkgRoot := filepath.Join(root, pkg.Root)
	files := map[string]matchedFile{}
	realFileCount := 0
	for _, inc := range pkg.Publish.Include {
		if badPath(inc) {
			out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_INVALID_PUBLISH_PATH", "invalid publish include path", inc))
			continue
		}
		matches, diags := resolvePattern(pkgRoot, inc)
		out.Diagnostics = append(out.Diagnostics, diags...)
		if len(matches) == 0 {
			out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_PACK_INCLUDE_MATCHED_NOTHING", Severity: diag.SeverityWarning, Message: "include matched nothing", Details: []string{inc}})
		}
		for _, m := range matches {
			realFileCount++
			files[m.archive] = m
		}
	}
	for _, exc := range pkg.Publish.Exclude {
		if badPath(exc) {
			out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_INVALID_PUBLISH_PATH", "invalid publish exclude path", exc))
			continue
		}
		em, _ := resolvePattern(pkgRoot, exc)
		for _, m := range em {
			delete(files, m.archive)
		}
	}
	for _, t := range pkg.Targets {
		if _, err := os.Stat(filepath.Join(pkgRoot, t.Runtime)); err != nil {
			out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_MISSING_RUNTIME_OUTPUT", "missing runtime output", t.Runtime))
		}
		if _, err := os.Stat(filepath.Join(pkgRoot, t.Types)); err != nil {
			out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_MISSING_TYPE_OUTPUT", "missing type output", t.Types))
		}
		out.Diagnostics = append(out.Diagnostics, unpublishablePeerDiagnostics(pkg, t)...)
	}
	if hasErrors(out.Diagnostics) {
		diag.SortDiagnostics(out.Diagnostics)
		return out
	}
	if realFileCount == 0 {
		out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_EMPTY_PACKAGE", "publish policy matched no files"))
		diag.SortDiagnostics(out.Diagnostics)
		return out
	}
	if _, ok := files["package.json"]; !ok {
		b := generatedPackageJSON(pkg)
		files["package.json"] = matchedFile{source: "<generated>", archive: "package.json", reason: "generated package.json", mode: 0o644, size: int64(len(b)), data: b}
	}
	if len(files) == 0 {
		out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_EMPTY_PACKAGE", "package content is empty"))
		return out
	}
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f := files[k]
		out.Preview = append(out.Preview, File{PackageName: pkg.Name, SourcePath: f.source, ArchivePath: "package/" + f.archive, Size: f.size, Reason: f.reason})
	}
	if opts.DryRun {
		return out
	}
	if opts.OutputDir == "" {
		opts.OutputDir = filepath.Join(root, "tspack-artifacts")
	}
	_ = os.MkdirAll(opts.OutputDir, 0o755)
	name := strings.ReplaceAll(strings.TrimPrefix(pkg.Name, "@"), "/", "-") + "-" + pkg.Version + ".tgz"
	outPath := filepath.Join(opts.OutputDir, name)
	buf := bytes.NewBuffer(nil)
	gz, _ := gzip.NewWriterLevel(buf, gzip.BestCompression)
	gz.Header.ModTime = time.Unix(0, 0)
	tw := tar.NewWriter(gz)
	for _, k := range keys {
		f := files[k]
		b := f.data
		if b == nil {
			b, _ = os.ReadFile(f.source)
		}
		hdr := &tar.Header{Name: "package/" + k, Mode: f.mode, Size: int64(len(b)), ModTime: time.Unix(0, 0), Format: tar.FormatPAX}
		_ = tw.WriteHeader(hdr)
		_, _ = tw.Write(b)
	}
	_ = tw.Close()
	_ = gz.Close()
	archive := buf.Bytes()
	_ = os.WriteFile(outPath, archive, 0o644)
	h := sha256.Sum256(archive)
	out.Artifacts = append(out.Artifacts, Artifact{PackageName: pkg.Name, Version: pkg.Version, Path: outPath, Hash: "sha256:" + hex.EncodeToString(h[:]), Size: int64(len(archive))})
	return out
}

func resolvePattern(root, pattern string) ([]matchedFile, []diag.Diagnostic) {
	var out []matchedFile
	glob := strings.Contains(pattern, "*")
	if strings.HasSuffix(pattern, "/**") {
		pattern = strings.TrimSuffix(pattern, "/**")
		glob = true
	}
	if !glob {
		p := filepath.Join(root, pattern)
		st, err := os.Lstat(p)
		if err != nil {
			return nil, nil
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, []diag.Diagnostic{dErr("TSPACK_PACK_SYMLINK_UNSUPPORTED", "symlink unsupported", pattern)}
		}
		if st.IsDir() {
			_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				info, _ := d.Info()
				if info.Mode()&os.ModeSymlink != 0 {
					return nil
				}
				rel, _ := filepath.Rel(root, path)
				out = append(out, matchedFile{source: path, archive: filepath.ToSlash(rel), reason: "matched include \"" + pattern + "\"", mode: 0o644, size: info.Size()})
				return nil
			})
			return out, nil
		}
		return []matchedFile{{source: p, archive: filepath.ToSlash(pattern), reason: "included explicit " + pattern, mode: 0o644, size: st.Size()}}, nil
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		m, _ := filepath.Match(strings.ReplaceAll(pattern, "**", "*"), rel)
		if strings.HasPrefix(rel, strings.TrimSuffix(pattern, "/*")) || m {
			info, _ := d.Info()
			if info.Mode()&os.ModeSymlink != 0 {
				out = append(out, matchedFile{source: path, archive: rel, reason: "symlink", mode: 0, size: 0})
				return nil
			}
			out = append(out, matchedFile{source: path, archive: rel, reason: "matched include \"" + pattern + "\"", mode: 0o644, size: info.Size()})
		}
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].archive < out[j].archive })
	for _, item := range out {
		if item.reason == "symlink" {
			return nil, []diag.Diagnostic{dErr("TSPACK_PACK_SYMLINK_UNSUPPORTED", "symlink unsupported", item.archive)}
		}
	}
	return out, nil
}
func generatedPackageJSON(pkg *graph.PackageNode) []byte {
	pj := map[string]any{
		"name":    pkg.Name,
		"version": pkg.Version,
	}

	if strings.TrimSpace(pkg.License) != "" {
		pj["license"] = pkg.License
	}

	if rootTarget := rootExportTarget(pkg); rootTarget != nil {
		pj["main"] = packageJSONPath(rootTarget.Runtime)
		if strings.TrimSpace(rootTarget.Types) != "" {
			pj["types"] = packageJSONPath(rootTarget.Types)
		}
	}

	peerDependencies, optionalPeers := packageJSONPeerDependencies(pkg)
	if len(peerDependencies) > 0 {
		pj["peerDependencies"] = peerDependencies
	}
	if len(optionalPeers) > 0 {
		peerDependenciesMeta := map[string]any{}
		for _, peerName := range optionalPeers {
			peerDependenciesMeta[peerName] = map[string]bool{"optional": true}
		}
		pj["peerDependenciesMeta"] = peerDependenciesMeta
	}

	exports := map[string]any{}
	for _, t := range pkg.Targets {
		entry := map[string]string{
			"default": packageJSONPath(t.Runtime),
			"types":   packageJSONPath(t.Types),
		}
		exports[t.Export] = entry
	}
	pj["exports"] = exports

	buf := bytes.NewBuffer(nil)
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(pj)
	return buf.Bytes()
}

func unpublishablePeerDiagnostics(pkg *graph.PackageNode, target *graph.TargetNode) []diag.Diagnostic {
	var diagnostics []diag.Diagnostic
	for _, dep := range target.PeerDeps {
		if dep.Source.Kind == "npm" {
			continue
		}
		diagnostics = append(diagnostics, dErr(
			"TSPACK_PACK_UNPUBLISHABLE_PEER_DEPENDENCY",
			"peer dependency cannot be represented in npm package.json",
			pkg.Name,
			target.Name,
			dep.Key,
			dep.Source.Kind,
		))
	}
	return diagnostics
}

func rootExportTarget(pkg *graph.PackageNode) *graph.TargetNode {
	for _, target := range pkg.Targets {
		if target.Export == "." {
			return target
		}
	}
	return nil
}

func packageJSONPath(value string) string {
	normalized := filepath.ToSlash(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "./")
	return "./" + normalized
}

func packageJSONPeerDependencies(pkg *graph.PackageNode) (map[string]string, []string) {
	peerDependencies := map[string]string{}
	optionalPeerNames := map[string]bool{}

	for _, target := range pkg.Targets {
		for _, dep := range target.PeerDeps {
			if dep.Source.Kind != "npm" {
				continue
			}
			if dep.Source.Package == "" || dep.Source.Range == "" {
				continue
			}
			peerDependencies[dep.Source.Package] = dep.Source.Range
			if dep.Optional {
				optionalPeerNames[dep.Source.Package] = true
			}
		}
	}

	optionalPeers := make([]string, 0, len(optionalPeerNames))
	for peerName := range optionalPeerNames {
		optionalPeers = append(optionalPeers, peerName)
	}
	sort.Strings(optionalPeers)

	return peerDependencies, optionalPeers
}
func badPath(p string) bool {
	return strings.TrimSpace(p) == "" || filepath.IsAbs(p) || strings.Contains(p, "..") || strings.Contains(p, "\\")
}
func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}
func dErr(code, msg string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: msg, Details: details}
}

func ReadTarEntries(tgz []byte) ([]string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := []string{}
	for {
		hdr, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		out = append(out, hdr.Name)
	}
	return out, nil
}
