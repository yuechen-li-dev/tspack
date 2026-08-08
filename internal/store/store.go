package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

type Store struct {
	Root string

	artifactLocksMu sync.Mutex
	artifactLocks   map[string]*artifactLock
}

type artifactLock struct {
	mu   sync.Mutex
	refs int
}

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
	return &Store{
		Root:          root,
		artifactLocks: map[string]*artifactLock{},
	}, nil
}

func (s *Store) Has(hash string) bool { return len(s.Verify(normalizeHash(hash))) == 0 }

func (s *Store) PutArtifact(a Artifact) (StoreRef, []diag.Diagnostic) {
	hash, err := s.computeHash(a)
	if err != nil {
		return StoreRef{}, []diag.Diagnostic{errDiag("TSPACK_STORE_INVALID_HASH", err.Error())}
	}
	if a.Hash != "" && normalizeHash(a.Hash) != hash {
		return StoreRef{}, []diag.Diagnostic{errDiag("TSPACK_STORE_HASH_MISMATCH", "provided hash does not match artifact bytes")}
	}
	unlock := s.lockArtifact(hash)
	defer unlock()

	ref := s.ref(hash, a.Kind, a.ID)
	if a.Kind == ArtifactNPMTarball {
		if d := s.writeBlobIfMissing(ref.StorePath, a.Bytes); d != nil {
			return StoreRef{}, d
		}
		if !extractedArtifactHealthy(ref.ExtractedPath, "npm") {
			_ = os.RemoveAll(ref.ExtractedPath)
			if d := extractTarGz(a.Bytes, ref.ExtractedPath); d != nil {
				return StoreRef{}, d
			}
		}
	} else {
		if !extractedArtifactHealthy(ref.ExtractedPath, a.Source) {
			_ = os.RemoveAll(ref.ExtractedPath)
			if d := copyTree(a.RootDir, ref.ExtractedPath); d != nil {
				return StoreRef{}, d
			}
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

func (s *Store) lockArtifact(hash string) func() {
	s.artifactLocksMu.Lock()
	lock := s.artifactLocks[hash]
	if lock == nil {
		lock = &artifactLock{}
		s.artifactLocks[hash] = lock
	}
	lock.refs++
	s.artifactLocksMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()

		s.artifactLocksMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(s.artifactLocks, hash)
		}
		s.artifactLocksMu.Unlock()
	}
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
	if !extractedArtifactHealthy(ref.ExtractedPath, md.Source) {
		if md.Source == "npm" {
			return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACTED_ARTIFACT_INVALID", "npm extracted artifact is missing package.json")}
		}
		return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACTED_ARTIFACT_MISSING", "extracted artifact is missing")}
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
	if err := writeFileAtomic(path, b, 0o644); err != nil {
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
	if err := writeFileAtomic(path, data, 0o644); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return nil
		}
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}

func extractTarGz(data []byte, dest string) []diag.Diagnostic {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", err.Error())}
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dest), filepath.Base(dest)+".*.tmp")
	if err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", err.Error())}
	}
	defer os.RemoveAll(tmp)
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
		relSlash, skip, pathErr := tarballRelativePath(hdr.Name)
		if pathErr != nil {
			return []diag.Diagnostic{errDiag("TSPACK_STORE_TARBALL_PATH_TRAVERSAL", pathErr.Error())}
		}
		if skip {
			continue
		}
		out := filepath.Join(tmp, filepath.FromSlash(relSlash))
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
	if err := finalizeAtomicDirectory(tmp, dest); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_EXTRACT_FAILED", err.Error())}
	}
	return nil
}

func tarballRelativePath(name string) (string, bool, error) {
	slashPath := strings.ReplaceAll(name, "\\", "/")
	clean := path.Clean(slashPath)
	if clean == "." || clean == "/" {
		return "", true, nil
	}
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return "", false, fmt.Errorf("tarball path traversal detected")
	}
	parts := strings.Split(clean, "/")
	if len(parts) < 2 {
		return "", true, nil
	}
	rel := path.Clean(strings.Join(parts[1:], "/"))
	if rel == "." || rel == "" {
		return "", true, nil
	}
	if strings.HasPrefix(rel, "../") || rel == ".." {
		return "", false, fmt.Errorf("tarball path traversal detected")
	}
	return rel, false, nil
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
	tmpParent := filepath.Dir(dest)
	if err := os.MkdirAll(tmpParent, 0o755); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	tmp, tmpErr := os.MkdirTemp(tmpParent, filepath.Base(dest)+".*.tmp")
	if tmpErr != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", tmpErr.Error())}
	}
	defer os.RemoveAll(tmp)
	cleanTmp, tmpErr := filepath.Abs(filepath.Clean(tmp))
	if tmpErr != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", tmpErr.Error())}
	}
	if cleanRoot == cleanDest {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_SELF_COPY_DETECTED", "store copy source and destination are the same path")}
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
	if err := os.RemoveAll(dest); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	if err := finalizeAtomicDirectory(tmp, dest); err != nil {
		return []diag.Diagnostic{errDiag("TSPACK_STORE_WRITE_FAILED", err.Error())}
	}
	return nil
}

func finalizeAtomicDirectory(tmp string, dest string) error {
	const attempts = 10
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := os.Rename(tmp, dest); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if _, statErr := os.Stat(dest); statErr == nil {
			return nil
		}
		if !isRetryableRenameErr(lastErr) {
			return lastErr
		}
		time.Sleep(25 * time.Millisecond)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		return nil
	}
	return lastErr
}

func isRetryableRenameErr(err error) bool {
	return errors.Is(err, os.ErrPermission)
}

func extractedArtifactHealthy(root string, source string) bool {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return false
	}
	if source != "npm" {
		return true
	}
	packageJSON := filepath.Join(root, "package.json")
	info, err = os.Stat(packageJSON)
	return err == nil && !info.IsDir()
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
