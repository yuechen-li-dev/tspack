package main

import (
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestRunReadyURLInterpolationUsesDefaultsAndOverlays(t *testing.T) {
	defaultPort := "3000"
	target := manifest.RunTarget{
		Name:    "dev",
		Runtime: "system",
		Command: []string{"node", "server.js"},
		URL:     "http://127.0.0.1:${PORT}",
		Ready:   &manifest.RunReadyCheck{Kind: "http", Path: "/health"},
		Env:     []manifest.RunTargetEnv{{Name: "PORT", Default: &defaultPort}},
	}

	resolved, _, runErr := prepareRunTargetForInvocation(t.TempDir(), target, runEnvOverlay{})
	if runErr != nil {
		t.Fatalf("unexpected interpolation error: %#v", runErr)
	}
	if resolved.URL != "http://127.0.0.1:3000" {
		t.Fatalf("default interpolation used wrong URL: %q", resolved.URL)
	}

	overlay, overlayErr := (runEnvOverlay{}).WithAssignment("PORT=4000")
	if overlayErr != nil {
		t.Fatalf("unexpected overlay error: %#v", overlayErr)
	}
	resolved, _, runErr = prepareRunTargetForInvocation(t.TempDir(), target, overlay)
	if runErr != nil {
		t.Fatalf("unexpected overlay interpolation error: %#v", runErr)
	}
	if resolved.URL != "http://127.0.0.1:4000" {
		t.Fatalf("overlay interpolation used wrong URL: %q", resolved.URL)
	}
}

func TestRunReadyURLInterpolationFailures(t *testing.T) {
	secret := manifest.RunTarget{
		Name:    "dev",
		Runtime: "system",
		Command: []string{"node", "server.js"},
		URL:     "http://127.0.0.1:${TOKEN}/health",
		Ready:   &manifest.RunReadyCheck{Kind: "http", Path: "/health"},
		Env:     []manifest.RunTargetEnv{{Name: "TOKEN", Required: true, Secret: true}},
	}
	overlay, overlayErr := (runEnvOverlay{}).WithAssignment("TOKEN=super-secret-token")
	if overlayErr != nil {
		t.Fatalf("unexpected overlay error: %#v", overlayErr)
	}
	_, _, runErr := prepareRunTargetForInvocation(t.TempDir(), secret, overlay)
	if runErr == nil || runErr.code != "TSPACK_RUN_READY_ENV_SECRET" {
		t.Fatalf("expected secret interpolation diagnostic, got %#v", runErr)
	}
	if strings.Contains(runErr.msg, "super-secret-token") {
		t.Fatalf("secret value leaked in diagnostic: %q", runErr.msg)
	}

	missing := secret
	missing.URL = "http://127.0.0.1:${PORT}"
	missing.Env = nil
	_, _, runErr = prepareRunTargetForInvocation(t.TempDir(), missing, runEnvOverlay{})
	if runErr == nil || runErr.code != "TSPACK_RUN_READY_ENV_MISSING" {
		t.Fatalf("expected missing interpolation diagnostic, got %#v", runErr)
	}

	invalid := missing
	invalid.URL = "http://127.0.0.1:${BAD-NAME}/health"
	_, _, runErr = prepareRunTargetForInvocation(t.TempDir(), invalid, runEnvOverlay{})
	if runErr == nil || runErr.code != "TSPACK_MANIFEST_READY_INVALID" {
		t.Fatalf("expected invalid interpolation diagnostic, got %#v", runErr)
	}

	malformed := missing
	malformed.URL = "http://127.0.0.1:${PORT/health"
	_, _, runErr = prepareRunTargetForInvocation(t.TempDir(), malformed, runEnvOverlay{})
	if runErr == nil || runErr.code != "TSPACK_MANIFEST_READY_INVALID" {
		t.Fatalf("expected malformed interpolation diagnostic, got %#v", runErr)
	}
}

func TestRunReadyURLInterpolationLeavesStaticURLUnchanged(t *testing.T) {
	target := manifest.RunTarget{
		Name:    "dev",
		Runtime: "system",
		Command: []string{"node", "server.js"},
		URL:     "http://127.0.0.1:3000",
		Ready:   &manifest.RunReadyCheck{Kind: "http", Path: "/health"},
	}
	resolved, _, runErr := prepareRunTargetForInvocation(t.TempDir(), target, runEnvOverlay{})
	if runErr != nil {
		t.Fatalf("unexpected static URL error: %#v", runErr)
	}
	if resolved.URL != "http://127.0.0.1:3000" {
		t.Fatalf("static URL changed: %q", resolved.URL)
	}
}
