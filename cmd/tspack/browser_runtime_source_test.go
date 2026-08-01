package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserRuntimeSourceIsCheckedInAndContainsGeneratedHostContract(t *testing.T) {
	path := filepath.Join("runtime", "browser-v1", "index.js")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read canonical browser runtime %q: %v", path, err)
	}
	if !bytes.Equal(contents, browserRuntimeSource) {
		t.Fatal("embedded browser runtime differs from the checked-in canonical source")
	}

	for _, api := range []string{
		"export function scheduleRendererAttachment",
		"export function registerAttachmentPlans",
		"export function attachRenderer",
		"export function updateRenderer",
		"export function detachRenderer",
		"export function registerComponentFrames",
		"export function registerComponentFrameEnvelope",
		"export function recordLegacyComponentFrameContract",
		"export function dispatchComponentEvent",
		"export function shutdownAttachmentPlans",
		"export function shutdownComponentFrames",
		"export function registerRendererAdapter",
	} {
		if !strings.Contains(string(contents), api) {
			t.Fatalf("canonical browser runtime does not expose %q", api)
		}
	}
}
