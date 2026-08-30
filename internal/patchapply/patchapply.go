// Package patchapply deterministically applies the bounded unified-text patch
// form used by authoritative package realizations.
package patchapply

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const Algorithm = "unified-text-v1-exact"

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

type filePatch struct {
	oldPath string
	newPath string
	hunks   []hunk
}

type hunk struct {
	oldStart int
	oldCount int
	newStart int
	newCount int
	lines    []string
}

func CanonicalBytes(contents []byte) []byte {
	return bytes.ReplaceAll(contents, []byte("\r\n"), []byte("\n"))
}

func Digest(contents []byte) string {
	sum := sha256.Sum256(CanonicalBytes(contents))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func Materialize(sourceRoot string, destinationRoot string, patchBytes []byte) error {
	patches, err := parse(CanonicalBytes(patchBytes))
	if err != nil {
		return err
	}
	if err := copyTree(sourceRoot, destinationRoot); err != nil {
		return err
	}
	for _, file := range patches {
		if err := applyFile(destinationRoot, file); err != nil {
			return err
		}
	}
	return nil
}

func parse(contents []byte) ([]filePatch, error) {
	lines := strings.Split(string(contents), "\n")
	var files []filePatch
	for index := 0; index < len(lines); {
		if strings.HasPrefix(lines[index], "diff --git ") || strings.HasPrefix(lines[index], "index ") || lines[index] == "" {
			index++
			continue
		}
		if !strings.HasPrefix(lines[index], "--- ") || index+1 >= len(lines) || !strings.HasPrefix(lines[index+1], "+++ ") {
			return nil, fmt.Errorf("unsupported patch record at line %d", index+1)
		}
		oldPath := strings.Fields(strings.TrimPrefix(lines[index], "--- "))[0]
		newPath := strings.Fields(strings.TrimPrefix(lines[index+1], "+++ "))[0]
		if oldPath == "/dev/null" || newPath == "/dev/null" || oldPath != "a/"+strings.TrimPrefix(newPath, "b/") {
			return nil, fmt.Errorf("file create, delete, or rename is unsupported: %s -> %s", oldPath, newPath)
		}
		file := filePatch{oldPath: strings.TrimPrefix(oldPath, "a/"), newPath: strings.TrimPrefix(newPath, "b/")}
		index += 2
		for index < len(lines) && !strings.HasPrefix(lines[index], "diff --git ") && !strings.HasPrefix(lines[index], "--- ") {
			if lines[index] == "" {
				index++
				continue
			}
			match := hunkHeader.FindStringSubmatch(lines[index])
			if match == nil {
				return nil, fmt.Errorf("unsupported patch metadata at line %d", index+1)
			}
			parsed := hunk{oldStart: number(match[1]), oldCount: count(match[2]), newStart: number(match[3]), newCount: count(match[4])}
			index++
			for index < len(lines) {
				line := lines[index]
				if strings.HasPrefix(line, "@@ ") || strings.HasPrefix(line, "diff --git ") || strings.HasPrefix(line, "--- ") {
					break
				}
				if strings.HasPrefix(line, "\\ No newline at end of file") {
					return nil, fmt.Errorf("no-newline markers are unsupported")
				}
				if line == "" {
					break
				}
				if line[0] != ' ' && line[0] != '+' && line[0] != '-' {
					return nil, fmt.Errorf("invalid hunk line at line %d", index+1)
				}
				parsed.lines = append(parsed.lines, line)
				index++
			}
			file.hunks = append(file.hunks, parsed)
		}
		if len(file.hunks) == 0 {
			return nil, fmt.Errorf("patch for %s has no hunks", file.newPath)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("patch contains no file changes")
	}
	return files, nil
}

func number(value string) int {
	n, _ := strconv.Atoi(value)
	return n
}

func count(value string) int {
	if value == "" {
		return 1
	}
	return number(value)
}

func applyFile(root string, patch filePatch) error {
	if !safePath(patch.newPath) {
		return fmt.Errorf("unsafe patch path %q", patch.newPath)
	}
	filename := filepath.Join(root, filepath.FromSlash(patch.newPath))
	if err := rejectSymlinkPath(root, filename); err != nil {
		return err
	}
	contents, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read patch target %s: %w", patch.newPath, err)
	}
	lines := strings.Split(string(CanonicalBytes(contents)), "\n")
	delta := 0
	for _, current := range patch.hunks {
		position := current.oldStart - 1 + delta
		if position < 0 || position > len(lines) {
			return fmt.Errorf("hunk position is outside %s", patch.newPath)
		}
		cursor := position
		replacement := make([]string, 0, current.newCount)
		oldSeen := 0
		newSeen := 0
		for _, patchLine := range current.lines {
			value := patchLine[1:]
			switch patchLine[0] {
			case ' ':
				if cursor >= len(lines) || lines[cursor] != value {
					return fmt.Errorf("hunk context mismatch in %s at source line %d", patch.newPath, cursor+1)
				}
				replacement = append(replacement, value)
				cursor++
				oldSeen++
				newSeen++
			case '-':
				if cursor >= len(lines) || lines[cursor] != value {
					return fmt.Errorf("hunk removal mismatch in %s at source line %d", patch.newPath, cursor+1)
				}
				cursor++
				oldSeen++
			case '+':
				replacement = append(replacement, value)
				newSeen++
			}
		}
		if oldSeen != current.oldCount || newSeen != current.newCount {
			return fmt.Errorf("hunk line count mismatch in %s", patch.newPath)
		}
		lines = append(lines[:position], append(replacement, lines[cursor:]...)...)
		delta += current.newCount - current.oldCount
	}
	return os.WriteFile(filename, []byte(strings.Join(lines, "\n")), 0o644)
}

func safePath(value string) bool {
	clean := path.Clean(strings.ReplaceAll(value, "\\", "/"))
	hasWindowsVolume := len(clean) >= 2 && clean[1] == ':'
	return clean == value && clean != "." && !path.IsAbs(clean) && !hasWindowsVolume && clean != ".." && !strings.HasPrefix(clean, "../")
}

func rejectSymlinkPath(root string, filename string) error {
	current := root
	relative, _ := filepath.Rel(root, filename)
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("patch target traverses symlink %s", current)
		}
	}
	return nil
}

func copyTree(sourceRoot string, destinationRoot string) error {
	return filepath.WalkDir(sourceRoot, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceRoot, filename)
		if err != nil || relative == "." {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("patched package trees cannot contain symlinks: %s", relative)
		}
		destination := filepath.Join(destinationRoot, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported package entry %s", relative)
		}
		contents, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.WriteFile(destination, contents, 0o644)
	})
}
