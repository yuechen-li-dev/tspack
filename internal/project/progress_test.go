package project

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestUpdateProgressReportsPhasesAndPackageFetches(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, progressNPMIR("dep-a", "dep-a", "1.0.0"))
	var progress bytes.Buffer
	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = buildRegistry()
	opts.Progress = Progress{Enabled: true, Writer: &progress}

	res := Update(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("update failed: %#v", res.Diagnostics)
	}

	text := progress.String()
	for _, want := range []string{
		"resolving packages...",
		"populating store...",
		"populating store: 2 packages with 2 workers",
		"fetching npm artifacts [1/2] dep-a@1.0.0",
		"fetching npm artifacts [2/2] left-pad@1.2.0",
		"writing lockfile...",
		"update complete",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("progress missing %q:\n%s", want, text)
		}
	}
}

func TestUpdateDryRunProgressReportsDiffAndCompletion(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, progressNPMIR("dep-a", "dep-a", "1.0.0"))
	var progress bytes.Buffer
	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = buildRegistry()
	opts.Progress = Progress{Enabled: true, Writer: &progress}

	res := UpdateDryRun(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("dry-run failed: %#v", res.Diagnostics)
	}

	text := progress.String()
	for _, want := range []string{
		"resolving packages...",
		"computing lockfile diff...",
		"dry run complete",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("progress missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "populating store") || strings.Contains(text, "writing lockfile") {
		t.Fatalf("dry-run progress should not include mutation phases:\n%s", text)
	}
}

func TestTargetedUpdateProgressReportsTargetContext(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, progressNPMIR("react", "react", "18.2.0"))
	var normal bytes.Buffer
	normalOpts := DefaultOptions(root)
	normalOpts.ManifestIRPath = irPath
	normalOpts.ResolverClient = buildRegistryForTargetedSelection(t)
	normalOpts.Progress = Progress{Enabled: true, Writer: &normal}

	res := UpdateWithOptions(normalOpts, UpdateOptions{Query: "react"})
	if hasErrors(res.Diagnostics) {
		t.Fatalf("targeted update failed: %#v", res.Diagnostics)
	}
	if !strings.Contains(normal.String(), "updating target dependency: react") {
		t.Fatalf("normal targeted progress missing query:\n%s", normal.String())
	}

	var dryRun bytes.Buffer
	dryRunOpts := DefaultOptions(root)
	dryRunOpts.ManifestIRPath = irPath
	dryRunOpts.ResolverClient = buildRegistryForTargetedSelection(t)
	dryRunOpts.Progress = Progress{Enabled: true, Writer: &dryRun}

	res = UpdateDryRunWithOptions(dryRunOpts, UpdateOptions{Query: "react"})
	if hasErrors(res.Diagnostics) {
		t.Fatalf("targeted dry-run failed: %#v", res.Diagnostics)
	}
	if !strings.Contains(dryRun.String(), "planning targeted update: react") {
		t.Fatalf("dry-run targeted progress missing query:\n%s", dryRun.String())
	}
}

func TestUpdateProgressStillAllowsDiagnosticOnTarballFailure(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, progressNPMIR("dep-a", "dep-a", "1.0.0"))
	client := buildRegistry()
	client.tar = map[string][]byte{}
	var progress bytes.Buffer
	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = client
	opts.Progress = Progress{Enabled: true, Writer: &progress}

	res := Update(opts)
	if !hasErrCode(res.Diagnostics, "TSPACK_RESOLVE_NPM_TARBALL_ERROR") {
		t.Fatalf("expected tarball diagnostic, got %#v", res.Diagnostics)
	}
	if !strings.Contains(progress.String(), "resolving packages...") {
		t.Fatalf("expected progress before failure, got %q", progress.String())
	}
}

func TestProgressDisabledByDefault(t *testing.T) {
	var progress bytes.Buffer
	Progress{Enabled: false, Writer: &progress}.Step("hidden")
	Progress{Enabled: true}.Step("hidden")
	if progress.String() != "" {
		t.Fatalf("expected disabled progress to stay silent, got %q", progress.String())
	}
}

func TestUpdateProgressReportsStoreTarballFailure(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, progressNPMIR("dep-a", "dep-a", "1.0.0"))
	client := buildRegistry()
	clientWithStoreFailure := &storeFailureClient{fakeClient: client, failAfterResolve: true}
	var progress bytes.Buffer
	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = clientWithStoreFailure
	opts.Progress = Progress{Enabled: true, Writer: &progress}

	res := Update(opts)
	if !hasErrCode(res.Diagnostics, "TSPACK_RESOLVE_NPM_TARBALL_FETCH_FAILED") {
		t.Fatalf("expected store tarball fetch diagnostic, got %#v", res.Diagnostics)
	}
	if !strings.Contains(progress.String(), "populating store: 2 packages with 2 workers") {
		t.Fatalf("expected deterministic store population progress before store failure, got %q", progress.String())
	}
}

func TestSyncProgressReportsHydrationAndColdMaterialization(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, progressNPMIR("dep-a", "dep-a", "1.0.0"))
	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = buildRegistry()

	updateResult := Update(opts)
	if hasErrors(updateResult.Diagnostics) {
		t.Fatalf("update failed: %#v", updateResult.Diagnostics)
	}
	if err := os.RemoveAll(opts.StoreRoot); err != nil {
		t.Fatalf("remove store: %v", err)
	}

	var progress bytes.Buffer
	opts.Progress = Progress{Enabled: true, Writer: &progress}
	syncResult := Sync(opts, false)
	if hasErrors(syncResult.Diagnostics) {
		t.Fatalf("sync failed: %#v", syncResult.Diagnostics)
	}

	text := progress.String()
	for _, want := range []string{
		"fetching npm artifacts [1/2] dep-a@1.0.0",
		"fetching npm artifacts [2/2] left-pad@1.2.0",
		"materializing packages [1/2] dep-a@1.0.0",
		"materializing packages [2/2] left-pad@1.2.0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("sync progress missing %q:\n%s", want, text)
		}
	}
}

func TestSyncProgressStaysQuietWhenAlreadyMaterialized(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, progressNPMIR("dep-a", "dep-a", "1.0.0"))
	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = buildRegistry()

	updateResult := Update(opts)
	if hasErrors(updateResult.Diagnostics) {
		t.Fatalf("update failed: %#v", updateResult.Diagnostics)
	}
	firstSync := Sync(opts, false)
	if hasErrors(firstSync.Diagnostics) {
		t.Fatalf("first sync failed: %#v", firstSync.Diagnostics)
	}

	var progress bytes.Buffer
	opts.Progress = Progress{Enabled: true, Writer: &progress}
	secondSync := Sync(opts, false)
	if hasErrors(secondSync.Diagnostics) {
		t.Fatalf("second sync failed: %#v", secondSync.Diagnostics)
	}
	if strings.Contains(progress.String(), "fetching npm artifacts [") || strings.Contains(progress.String(), "materializing packages [") {
		t.Fatalf("expected warm sync to avoid indexed progress noise, got:\n%s", progress.String())
	}
}

func progressNPMIR(key string, packageName string, versionRange string) map[string]any {
	return progressIR([]map[string]any{
		{
			"key":  key,
			"kind": "dep",
			"source": map[string]any{
				"kind":    "npm",
				"package": packageName,
				"range":   versionRange,
			},
		},
	}, []string{key})
}

func progressIR(dependencies []map[string]any, targetDeps []string) map[string]any {
	return map[string]any{
		"format": 1,
		"workspace": map[string]any{
			"name": "ws",
		},
		"packages": []map[string]any{
			{
				"name":         "app",
				"version":      "1.0.0",
				"kind":         "library",
				"dependencies": dependencies,
				"targets": []map[string]any{
					{
						"name":    "core",
						"export":  ".",
						"entry":   "src/index.ts",
						"runtime": "src/index.ts",
						"types":   "dist/index.d.ts",
						"deps":    targetDeps,
						"peers":   []string{},
					},
				},
				"tools":      []string{},
				"boundaries": []any{},
				"publish": map[string]any{
					"include": []string{"dist/**"},
					"exclude": []string{},
				},
				"policies": map[string]any{
					"types":      map[string]any{},
					"boundaries": map[string]any{},
				},
			},
		},
	}
}

type storeFailureClient struct {
	*fakeClient
	failAfterResolve bool
}

func (c *storeFailureClient) Tarball(ctx context.Context, url string) ([]byte, error) {
	c.fakeClient.mu.Lock()
	shouldFail := c.failAfterResolve && strings.Contains(url, "dep-a-1.0.0.tgz") && len(c.tarCalls) >= 2
	c.fakeClient.mu.Unlock()
	if shouldFail {
		return nil, errors.New("store fetch failed")
	}
	return c.fakeClient.Tarball(ctx, url)
}
