package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	experimental "github.com/yuechen-li-dev/oct/experimental/octgen"
)

func TestConceptGenerationIsDeterministicAndChecksBothOutputs(t *testing.T) {
	generator := copyFixture(t, filepath.Join("..", "..", "internal", "concepts", "concepts.oct"))
	value, err := experimental.Execute(generator)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	decoded, err := decodeModel(value, generator)
	if err != nil {
		t.Fatalf("decodeModel: %v", err)
	}
	if len(decoded.Concepts) != 15 || len(decoded.Cases) != 3 {
		t.Fatalf("derived model sizes = %d concepts, %d cases", len(decoded.Concepts), len(decoded.Cases))
	}
	registry, err := renderRegistry(decoded)
	if err != nil {
		t.Fatalf("renderRegistry: %v", err)
	}
	expectations, err := renderExpectations(decoded)
	if err != nil {
		t.Fatalf("renderExpectations: %v", err)
	}
	secondRegistry, err := renderRegistry(decoded)
	if err != nil {
		t.Fatalf("renderRegistry again: %v", err)
	}
	if !bytes.Equal(registry, secondRegistry) {
		t.Fatal("registry rendering was not byte-identical")
	}
	output := filepath.Join(filepath.Dir(generator), "builtin_registry.generated.go")
	testOutput := filepath.Join(filepath.Dir(generator), "builtin_registry.generated_test.go")
	artifacts := []experimental.Artifact{{Path: output, Content: registry}, {Path: testOutput, Content: expectations}}
	if err := experimental.Write(generator, artifacts); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := experimental.Check(generator, artifacts); err != nil {
		t.Fatalf("fresh Check: %v", err)
	}
	if err := os.WriteFile(output, []byte("package concepts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := experimental.Check(generator, artifacts); err == nil || !strings.Contains(err.Error(), output) {
		t.Fatalf("production stale Check error = %v", err)
	}
	stale, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(stale) != "package concepts\n" {
		t.Fatal("check modified a stale output")
	}
	if err := experimental.Write(generator, artifacts); err != nil {
		t.Fatalf("restore production output: %v", err)
	}
	if err := os.WriteFile(testOutput, []byte("package concepts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := experimental.Check(generator, artifacts); err == nil || !strings.Contains(err.Error(), testOutput) {
		t.Fatalf("test stale Check error = %v", err)
	}
	stale, err = os.ReadFile(testOutput)
	if err != nil {
		t.Fatal(err)
	}
	if string(stale) != "package concepts\n" {
		t.Fatal("check modified a stale test output")
	}
}

func TestInvalidTSPackModelIncludesProvenance(t *testing.T) {
	generator := filepath.Join("testdata", "invalid_model.oct")
	value, err := experimental.Execute(generator)
	if err != nil {
		t.Fatalf("Execute invalid model: %v", err)
	}
	_, err = decodeModel(value, generator)
	if err == nil || !strings.Contains(err.Error(), "GeneratedTSPackConceptRegistry") || !strings.Contains(err.Error(), "invalid_model") {
		t.Fatalf("invalid-model error = %v", err)
	}
}

func copyFixture(t *testing.T, source string) string {
	t.Helper()
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "concepts.oct")
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
