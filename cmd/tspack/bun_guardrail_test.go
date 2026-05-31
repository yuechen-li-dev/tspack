package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestM42RuntimeProfilesDoNotDelegatePackageManagers(t *testing.T) {
	repo := filepath.Join("..", "..")
	forbidden := []string{
		`exec.Command("bun", "install"`,
		`exec.Command("bun", "add"`,
		`exec.Command("bun", "pm"`,
		`exec.Command("deno", "task"`,
		`exec.Command("deno", "install"`,
		`exec.Command("deno", "add"`,
		`exec.Command("deno", "cache"`,
		`exec.Command("deno", "vendor"`,
		"bun." + "lockb",
		"deno." + "lock",
	}
	var matches []string
	walkErr := filepath.WalkDir(repo, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(path) == "bun_guardrail_test.go" {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(data)
		for _, needle := range forbidden {
			if strings.Contains(text, needle) {
				matches = append(matches, path+": "+needle)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	if len(matches) > 0 {
		t.Fatalf("runtime package-manager delegation guardrail found forbidden paths: %s", strings.Join(matches, "; "))
	}
}
