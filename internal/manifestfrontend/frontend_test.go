package manifestfrontend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkerReusesBootstrapButReevaluatesChangedManifest(t *testing.T) {
	directory := t.TempDir()
	frontendPath := filepath.Join(directory, "frontend.mjs")
	manifestPath := filepath.Join(directory, "manifest.tsx")
	frontend := "import fs from \"node:fs\";\n" +
		"import readline from \"node:readline\";\n" +
		"const result = path => ({ok:true,ir:JSON.parse(fs.readFileSync(path, \"utf8\")),diagnostics:[]});\n" +
		"if (process.argv[2] === \"--stdio-worker\") {\n" +
		"  const lines = readline.createInterface({input:process.stdin});\n" +
		"  for await (const line of lines) {\n" +
		"    const request = JSON.parse(line);\n" +
		"    process.stdout.write(JSON.stringify({id:request.id,result:result(request.manifestPath)}) + \"\\n\");\n" +
		"  }\n" +
		"}\n"
	if err := os.WriteFile(frontendPath, []byte(frontend), 0o644); err != nil {
		t.Fatal(err)
	}
	writeManifestResult(t, manifestPath, "first")
	before := SnapshotStatistics()
	first, err := Execute(frontendPath, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	writeManifestResult(t, manifestPath, "second")
	second, err := Execute(frontendPath, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	after := SnapshotStatistics()
	t.Cleanup(CloseWorkers)

	if string(first.IR) == string(second.IR) {
		t.Fatalf("changed manifest reused stale result: %s", first.IR)
	}
	if got := after.WorkerStarts - before.WorkerStarts; got != 1 {
		t.Fatalf("worker starts = %d, want 1", got)
	}
	if got := after.WorkerRequests - before.WorkerRequests; got != 2 {
		t.Fatalf("worker requests = %d, want 2", got)
	}
}

func TestWorkerInvalidatesWhenFrontendArtifactChanges(t *testing.T) {
	directory := t.TempDir()
	frontendPath := filepath.Join(directory, "frontend.mjs")
	manifestPath := filepath.Join(directory, "manifest.tsx")
	writeFrontend := func(marker string) {
		t.Helper()
		source := "import readline from \"node:readline\";\n" +
			"if (process.argv[2] === \"--stdio-worker\") {\n" +
			"  const lines = readline.createInterface({input:process.stdin});\n" +
			"  for await (const line of lines) {\n" +
			"    const request = JSON.parse(line);\n" +
			"    process.stdout.write(JSON.stringify({id:request.id,result:{ok:true,ir:{format:1,workspace:{name:\"" + marker + "\"},packages:[]},diagnostics:[]}}) + \"\\n\");\n" +
			"  }\n" +
			"}\n"
		if err := os.WriteFile(frontendPath, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(manifestPath, []byte("unused"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFrontend("first")
	before := SnapshotStatistics()
	first, err := Execute(frontendPath, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFrontend("second-version")
	second, err := Execute(frontendPath, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	after := SnapshotStatistics()
	t.Cleanup(CloseWorkers)

	if string(first.IR) == string(second.IR) {
		t.Fatalf("changed frontend artifact reused stale worker: %s", first.IR)
	}
	if got := after.WorkerStarts - before.WorkerStarts; got != 2 {
		t.Fatalf("worker starts = %d, want 2", got)
	}
}

func writeManifestResult(t *testing.T, path string, name string) {
	t.Helper()
	contents := `{"format":1,"workspace":{"name":"` + name + `"},"packages":[]}`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
