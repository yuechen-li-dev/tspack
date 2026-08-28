package compiler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDescriptorPreservesSemanticAndMaterializationIdentity(t *testing.T) {
	target := testTarget()
	target.Packages = []PackageBinding{
		{
			SemanticIdentity:    "jsr:@std/path",
			Version:             "1.0.8",
			MaterializationName: "@jsr/std__path",
			MaterializationPath: "/workspace/node_modules/@jsr/std__path",
			LocalName:           "std-path",
			Role:                "runtime",
		},
		{
			SemanticIdentity:    "npm:lodash",
			Version:             "4.17.21",
			MaterializationName: "lodash",
			MaterializationPath: "/workspace/node_modules/lodash",
			LocalName:           "underscore",
			Role:                "runtime",
		},
	}
	descriptor, err := NewDescriptor(target)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.Packages[0].SemanticIdentity != "jsr:@std/path" || descriptor.Packages[0].MaterializationName != "@jsr/std__path" {
		t.Fatalf("JSR identities collapsed: %#v", descriptor.Packages[0])
	}
	if descriptor.Packages[1].LocalName != "underscore" || descriptor.Packages[1].SemanticIdentity != "npm:lodash" {
		t.Fatalf("npm alias was not preserved: %#v", descriptor.Packages[1])
	}
}

func TestGenericDescriptorHasNoCompilerSpecificFields(t *testing.T) {
	descriptor, err := NewDescriptor(testTarget())
	if err != nil {
		t.Fatal(err)
	}
	contents, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"tsclProject", "copelandOwner", "csproj", "CopelandCompile"} {
		if strings.Contains(string(contents), forbidden) {
			t.Fatalf("generic descriptor contains compiler-specific field %q", forbidden)
		}
	}
}

func TestAdaptersKeepLanguageCompilerToolAndRuntimeSeparate(t *testing.T) {
	target := testTarget()
	target.Config = ConfigRef{Kind: "file", Path: "tsconfig.json", Fingerprint: "config"}
	target.Runtime = RuntimeIdentity{Family: "javascript", Name: "node"}
	target.Tool.Path = "/workspace/node_modules/.bin/tsc"

	invocation, err := (TSCAdapter{}).PrepareInvocation(target, "/ignored/descriptor.json")
	if err != nil {
		t.Fatal(err)
	}
	if invocation.Executable != target.Tool.Path || len(invocation.Arguments) != 2 || invocation.Arguments[1] != "tsconfig.json" {
		t.Fatalf("unexpected tsc invocation: %#v", invocation)
	}
	if target.Runtime.Name != "node" || target.Compiler.ID != "tsc" || target.Language.ID != "typescript" || target.Tool.Name != "typescript" {
		t.Fatalf("identity axes were collapsed: %#v", target)
	}
}

func TestCopelandAdapterRequiresVersionedPayload(t *testing.T) {
	target := testTarget()
	target.Language.ID = "copeland-ts"
	target.Compiler.ID = "tscl"
	target.Tool.Path = "/tools/tscl"
	target.Payload = Payload{Kind: "copeland-v1", SchemaVersion: 1, Data: json.RawMessage(`{"backend":"javascript","executionRuntime":"node"}`)}

	invocation, err := (CopelandAdapter{}).PrepareInvocation(target, "/workspace/app.request.json")
	if err != nil {
		t.Fatal(err)
	}
	if invocation.Arguments[len(invocation.Arguments)-1] != "/workspace/app" {
		t.Fatalf("unexpected Copeland result path: %#v", invocation)
	}
	target.Payload.SchemaVersion = 2
	if _, err := (CopelandAdapter{}).PrepareInvocation(target, "/workspace/app.request.json"); err == nil {
		t.Fatal("future Copeland payload version was accepted")
	}
}

func TestCopelandAdapterRejectsInvalidBackendRuntimePairAndMissingNativeRID(t *testing.T) {
	target := testTarget()
	target.Language.ID = "copeland-ts"
	target.Compiler.ID = "tscl"
	target.Tool.Path = "/tools/tscl"
	target.Payload = Payload{Kind: "copeland-v1", SchemaVersion: 1, Data: json.RawMessage(`{"backend":"javascript","executionRuntime":"nativeaot"}`)}
	if _, err := (CopelandAdapter{}).PrepareInvocation(target, "/workspace/app.request.json"); err == nil || !strings.Contains(err.Error(), "invalid Copeland backend/runtime") {
		t.Fatalf("invalid pair error = %v", err)
	}
	target.Payload.Data = json.RawMessage(`{"backend":"csharp","executionRuntime":"nativeaot"}`)
	if _, err := (CopelandAdapter{}).PrepareInvocation(target, "/workspace/app.request.json"); err == nil || !strings.Contains(err.Error(), "runtimeIdentifier") {
		t.Fatalf("missing RID error = %v", err)
	}
}

func TestScriptCAdapterKeepsCompilerFlagsInVersionedPayload(t *testing.T) {
	target := testTarget()
	target.Language.ID = "scriptc"
	target.Compiler.ID = "scriptc"
	target.Tool = ToolIdentity{Source: "npm", Name: "scriptc", Version: "0.0.35", Path: "/tools/scriptc"}
	target.Outputs = []Output{{Kind: OutputNativeExecutable, Path: "dist/hotpath"}}
	target.Payload = Payload{
		Kind:          "scriptc-v1",
		SchemaVersion: 1,
		Data:          json.RawMessage(`{"entry":"src/hot/main.ts","output":"dist/hotpath","optimization":"dev"}`),
	}

	invocation, err := (ScriptCAdapter{}).PrepareInvocation(target, "/ignored/descriptor.json")
	if err != nil {
		t.Fatal(err)
	}
	want := "build src/hot/main.ts --out dist/hotpath --no-keep-c --optimization dev"
	if strings.Join(invocation.Arguments, " ") != want {
		t.Fatalf("arguments=%q, want %q", strings.Join(invocation.Arguments, " "), want)
	}
}

func TestPerryAdapterKeepsCompilerFlagsInVersionedPayload(t *testing.T) {
	target := testTarget()
	target.Language.ID = "perry-ts"
	target.Compiler.ID = "perry"
	target.Tool = ToolIdentity{Source: "npm", Name: "@perryts/perry", Version: "0.5.1220", Path: "/tools/perry"}
	target.Outputs = []Output{{Kind: OutputNativeExecutable, Path: "dist/hotpath"}}
	target.Payload = Payload{
		Kind:          "perry-v1",
		SchemaVersion: 1,
		Data:          json.RawMessage(`{"entry":"src/hot/main.ts","output":"dist/hotpath","target":"windows","fastMath":true,"fpContract":"fast","features":["simd"]}`),
	}

	invocation, err := (PerryAdapter{}).PrepareInvocation(target, "/ignored/descriptor.json")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(invocation.Arguments, " ")
	for _, expected := range []string{"compile", "src/hot/main.ts", "--target windows", "--fast-math", "--fp-contract fast", "--features simd"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("invocation %q is missing %q", joined, expected)
		}
	}
}

func testTarget() Target {
	return Target{
		ProjectRoot: "C:/workspace",
		Package:     "app",
		Name:        "main",
		Language:    LanguageIdentity{ID: "typescript"},
		Compiler:    CompilerIdentity{ID: "tsc", Version: "5.9.2"},
		Tool:        ToolIdentity{Source: "npm", Name: "typescript", Version: "5.9.2"},
		Inputs:      []Input{{LogicalPath: "src/main.ts", Path: "/workspace/src/main.ts"}},
	}
}

func BenchmarkDescriptorConstructionAndSerialization(b *testing.B) {
	target := testTarget()
	target.Config = ConfigRef{Kind: "file", Path: "tsconfig.json", Fingerprint: "config"}
	target.Runtime = RuntimeIdentity{Family: "javascript", Name: "node"}
	for index := 0; index < b.N; index++ {
		descriptor, err := NewDescriptor(target)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := json.Marshal(descriptor); err != nil {
			b.Fatal(err)
		}
	}
}
