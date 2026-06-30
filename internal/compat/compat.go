package compat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/manifesttypes"
)

type State string

const (
	StateMissing State = "missing"
	StateClean   State = "up-to-date"
	StateDrifted State = "drifted"
)

type FileStatus struct {
	Path         string
	Format       string
	State        State
	DesiredHash  string
	ExistingHash string
	Desired      []byte
	Existing     []byte
}

func Plan(root string, ir *manifest.ManifestIR) ([]FileStatus, error) {
	statuses := make([]FileStatus, 0, len(ir.CompatFiles)+1)
	for _, file := range ir.CompatFiles {
		desired, err := RenderJSON(file.Value)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", file.Path, err)
		}
		status := FileStatus{
			Path:        file.Path,
			Format:      file.Format,
			Desired:     desired,
			DesiredHash: hashBytes(desired),
		}
		if err := applyExistingFileStatus(root, &status); err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	if requiresManifestEditorTypes(ir) {
		status := FileStatus{
			Path:        ".tspack/types/tspack-manifest.d.ts",
			Format:      "text",
			Desired:     []byte(manifesttypes.TSPackManifestDTS),
			DesiredHash: hashBytes([]byte(manifesttypes.TSPackManifestDTS)),
		}
		if err := applyExistingFileStatus(root, &status); err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i int, j int) bool { return statuses[i].Path < statuses[j].Path })
	return statuses, nil
}

func applyExistingFileStatus(root string, status *FileStatus) error {
	existing, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(status.Path)))
	if os.IsNotExist(err) {
		status.State = StateMissing
		return nil
	}
	if err != nil {
		return err
	}
	status.Existing = existing
	status.ExistingHash = hashBytes(existing)
	if bytes.Equal(existing, status.Desired) {
		status.State = StateClean
		return nil
	}
	status.State = StateDrifted
	return nil
}

func requiresManifestEditorTypes(ir *manifest.ManifestIR) bool {
	for _, file := range ir.CompatFiles {
		if file.Path == "tsconfig.tspack.json" {
			return true
		}
	}
	return false
}

func Write(root string, statuses []FileStatus) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	for _, status := range statuses {
		if status.State == StateClean {
			continue
		}
		pathAbs := filepath.Join(rootAbs, filepath.FromSlash(status.Path))
		if err := os.MkdirAll(filepath.Dir(pathAbs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(pathAbs, status.Desired, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func RenderJSON(value json.RawMessage) ([]byte, error) {
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	writeStableJSON(buf, decoded, 0)
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func writeStableJSON(buf *bytes.Buffer, value any, indent int) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		if len(keys) > 0 {
			for index, key := range keys {
				if index == 0 {
					buf.WriteByte('\n')
				} else {
					buf.WriteString(",\n")
				}
				writeIndent(buf, indent+2)
				keyBytes, _ := json.Marshal(key)
				buf.Write(keyBytes)
				buf.WriteString(": ")
				writeStableJSON(buf, typed[key], indent+2)
			}
			buf.WriteByte('\n')
			writeIndent(buf, indent)
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		if len(typed) > 0 {
			for index, item := range typed {
				if index == 0 {
					buf.WriteByte('\n')
				} else {
					buf.WriteString(",\n")
				}
				writeIndent(buf, indent+2)
				writeStableJSON(buf, item, indent+2)
			}
			buf.WriteByte('\n')
			writeIndent(buf, indent)
		}
		buf.WriteByte(']')
	default:
		valueBytes, _ := json.Marshal(typed)
		buf.Write(valueBytes)
	}
}

func writeIndent(buf *bytes.Buffer, indent int) {
	for i := 0; i < indent; i++ {
		buf.WriteByte(' ')
	}
}

func hashBytes(contents []byte) string {
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}
