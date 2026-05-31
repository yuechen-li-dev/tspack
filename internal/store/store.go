package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/lockfile"
)

type Store struct{ Root string }

type ArtifactKind string

const (
	ArtifactNPMTarball ArtifactKind = "npm-tarball"
	ArtifactGitTree    ArtifactKind = "git-tree"
	ArtifactPathTree   ArtifactKind = "path-tree"
	ArtifactWorkspace  ArtifactKind = "workspace-tree"
)

type PackageMetadata struct {
	Name                 string                `json:"name"`
	Version              string                `json:"version"`
	Source               string                `json:"source,omitempty"`
	PackageID            string                `json:"packageId,omitempty"`
	Hash                 string                `json:"hash"`
	Integrity            string                `json:"integrity,omitempty"`
	Capabilities         []lockfile.Capability `json:"capabilities,omitempty"`
	Dependencies         map[string]string     `json:"dependencies,omitempty"`
	OptionalDependencies map[string]string     `json:"optionalDependencies,omitempty"`
	PeerDependencies     map[string]string     `json:"peerDependencies,omitempty"`
	Bin                  any                   `json:"bin,omitempty"`
}

type Artifact struct {
	ID, Name, Version, Source, Hash, Integrity string
	Kind                                       ArtifactKind
	Bytes                                      []byte
	RootDir                                    string
	Metadata                                   PackageMetadata
}

type StoreRef struct {
	ID, Hash, StorePath, ExtractedPath, MetadataPath string
	Kind                                             ArtifactKind
}

func Open(root string) (*Store, error) {
	for _, sub := range []string{"blobs/sha256", "extracted/sha256", "metadata/sha256"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return nil, err
		}
	}
	return &Store{Root: root}, nil
}

func (s *Store) Has(hash string) bool { _, ok := s.Get(normalizeHash(hash)); return ok == nil }

func (s *Store) PutArtifact(a Artifact) (StoreRef, []diag.Diagnostic) {
	hash, err := s.computeHash(a)
	if err != nil {
		return StoreRef{}, []diag.Diagnostic{errDiag("TSPACK_STORE_INVALID_HASH", err.Error())}
	}
	if a.Hash != "" && normalizeHash(a.Hash) != hash {
		return StoreRef{}, []diag.Diagnostic{errDiag("TSPACK_STORE_HASH_MISMATCH", "provided hash does not match artifact bytes")}
	}
	ref := s.ref(hash, a.Kind, a.ID)
	if a.Kind == ArtifactNPMTarball {
		if d := s.writeBlobIfMissing(ref.StorePath, a.Bytes); d != nil {
			return StoreRef{}, d
		}
		if d := extractTarGz(a.Bytes, ref.ExtractedPath); d != nil {
			return StoreRef{}, d
		}
	} else {
		if d := copyTree(a.RootDir, ref.ExtractedPath); d != nil {
			return StoreRef{}, d
		}
	}
	meta := a.Metadata
	meta.Hash = hash
	if meta.Name == "" {
		meta.Name = a.Name
	}
	if meta.Version == "" {
		meta.Version = a.Version
	}
	if meta.Source == "" {
		meta.Source = a.Source
	}
	if meta.Integrity == "" {
		meta.Integrity = a.Integrity
	}
	sortCaps(meta.Capabilities)
	if d := writeMetadata(ref.MetadataPath, meta); d != nil {
		return StoreRef{}, d
	}
	return ref, nil
}

func (s *Store) Get(hash string) (StoreRef, []diag.Diagnostic) {
	h := normalizeHash(hash)
	ref := s.ref(h, "", "")
	if _, err := os.Stat(ref.MetadataPath); err != nil {
		return StoreRef{}, []diag.Diagnostic{errDiag("TSPACK_STORE_ARTIFACT_NOT_FOUND", "artifact not found")}
	}
	return ref, nil
}

func (s *Store) Verify(hash string) []diag.Diagnostic {
	ref, d := s.Get(hash)
	if d != nil {
		return d
	}
	var md PackageMetadata
	b, err := os.ReadFile(ref.MetadataPath)
	if err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_READ_FAILED", err.Error())}
	}
	if err := json.Unmarshal(b, &md); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_METADATA_INVALID", err.Error())}
	}
	if md.Hash != normalizeHash(hash) {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_HASH_MISMATCH", "metadata hash mismatch")}
	}
	return nil
}

func (s *Store) computeHash(a Artifact) (string, error) {
	if len(a.Bytes) > 0 {
		sum := sha256.Sum256(a.Bytes)
		return "sha256:" + hex.EncodeToString(sum[:]), nil
	}
	if a.RootDir != "" {
		return hashDirectory(a.RootDir)
	}
	return "", fmt.Errorf("artifact has neither bytes nor rootDir")
}

func (s *Store) ref(hash string, kind ArtifactKind, id string) StoreRef {
	hexHash := strings.TrimPrefix(hash, "sha256:")
	prefix := hexHash[:2]
	return StoreRef{ID: id, Hash: hash, Kind: kind,
		StorePath:     filepath.Join(s.Root, "blobs", "sha256", prefix, hexHash+".tgz"),
		ExtractedPath: filepath.Join(s.Root, "extracted", "sha256", prefix, hexHash),
		MetadataPath:  filepath.Join(s.Root, "metadata", "sha256", prefix, hexHash+".json"),
	}
}

func normalizeHash(h string) string {
	if strings.HasPrefix(h, "sha256-") {
		return "sha256:" + strings.TrimPrefix(h, "sha256-")
	}
	if strings.HasPrefix(h, "sha256:") {
		return h
	}
	return "sha256:" + h
}
func errDiag(code, msg string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: msg}
}

func writeMetadata(path string, md PackageMetadata) []diag.Diagnostic {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	b, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_METADATA_INVALID", err.Error())}
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	if err := os.Rename(tmp, path); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	return nil
}
func (s *Store) writeBlobIfMissing(path string, data []byte) []diag.Diagnostic {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	if err := os.Rename(tmp, path); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	return nil
}

func extractTarGz(data []byte, dest string) []diag.Diagnostic {
	if err := os.RemoveAll(dest + ".tmp"); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", err.Error())}
	}
	tmp := dest + ".tmp"
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", err.Error())}
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", err.Error())}
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", err.Error())}
		}
		clean := filepath.Clean(hdr.Name)
		parts := strings.Split(clean, "/")
		if len(parts) < 2 {
			continue
		}
		clean = filepath.Clean(strings.Join(parts[1:], "/"))
		if clean == "." || clean == "" {
			continue
		}
		if filepath.IsAbs(clean) || strings.Contains(clean, "..") {
			return []diag.Diagnostic{errDiag("TSPACK_STORE_TARBALL_PATH_TRAVERSAL", "tarball path traversal detected")}
		}
		out := filepath.Join(tmp, clean)
		if !strings.HasPrefix(out, tmp+string(filepath.Separator)) && out != tmp {
			return []diag.Diagnostic{errDiag("TSPACK_STORE_TARBALL_PATH_TRAVERSAL", "tarball path traversal detected")}
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			mode := os.FileMode(hdr.Mode) & 0o777
			if mode == 0 {
				mode = 0o755
			}
			_ = os.MkdirAll(out, mode)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", err.Error())}
			}
			mode := os.FileMode(hdr.Mode) & 0o777
			if mode == 0 {
				mode = 0o644
			}
			f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", err.Error())}
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", err.Error())}
			}
			f.Close()
			if chmodErr := os.Chmod(out, mode); chmodErr != nil {
				return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", chmodErr.Error())}
			}
		}
	}
	_ = os.RemoveAll(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", err.Error())}
	}
	return nil
}

func hashDirectory(root string) (string, error) {
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
		return "", err
	}
	sort.Strings(files)
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", err
		}
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		h.Write(b)
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
func copyTree(root, dest string) []diag.Diagnostic {
	cleanRoot, rootErr := filepath.Abs(filepath.Clean(root))
	if rootErr != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", rootErr.Error())}
	}
	cleanDest, destErr := filepath.Abs(filepath.Clean(dest))
	if destErr != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", destErr.Error())}
	}
	tmp := dest + ".tmp"
	cleanTmp, tmpErr := filepath.Abs(filepath.Clean(tmp))
	if tmpErr != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", tmpErr.Error())}
	}
	if cleanRoot == cleanDest {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_SELF_COPY_DETECTED", "store copy source and destination are the same path")}
	}
	if err := os.RemoveAll(tmp); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	err := filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		pathAbs, absErr := filepath.Abs(filepath.Clean(path))
		if absErr != nil {
			return absErr
		}
		rel, _ := filepath.Rel(cleanRoot, pathAbs)
		if rel == "." {
			return nil
		}
		if d.IsDir() && (isSameOrWithin(pathAbs, cleanDest) || isSameOrWithin(pathAbs, cleanTmp)) {
			return filepath.SkipDir
		}
		if d.IsDir() && shouldSkipLocalArtifactDir(d.Name()) {
			return filepath.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.Type().IsRegular() && shouldSkipLocalArtifactFile(d.Name()) {
			return nil
		}
		out := filepath.Join(tmp, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		if d.Type().IsRegular() {
			b, er := os.ReadFile(path)
			if er != nil {
				return er
			}
			if er := os.MkdirAll(filepath.Dir(out), 0o755); er != nil {
				return er
			}
			info, er := os.Stat(path)
			if er != nil {
				return er
			}
			mode := info.Mode().Perm()
			if mode == 0 {
				mode = 0o644
			}
			return os.WriteFile(out, b, mode)
		}
		return nil
	})
	if err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	_ = os.RemoveAll(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	return nil
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

func isSameOrWithin(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func sortCaps(c []lockfile.Capability) {
	sort.SliceStable(c, func(i, j int) bool {
		if c[i].Kind != c[j].Kind {
			return c[i].Kind < c[j].Kind
		}
		if c[i].Script != c[j].Script {
			return c[i].Script < c[j].Script
		}
		return c[i].Command < c[j].Command
	})
}
