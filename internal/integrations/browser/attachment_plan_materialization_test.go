package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAttachmentPlanArtifactAcceptsVersionOneDeterministically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attachments.json")
	contents := `{"schemaVersion":1,"projectId":"demo","plans":[{"attachmentId":"a","componentInstanceId":"component","hostBoxId":"Page.host","hostSelector":"[data-machina-box='host']","adapterId":"CustomElement","lifecycle":{"mount":true,"update":true,"unmount":true}}]}`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := validateAttachmentPlanArtifact(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := validateAttachmentPlanArtifact(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 || first.PlanCount != 1 || first.SchemaVersion != 1 {
		t.Fatalf("unexpected deterministic artifact metadata: %#v %#v", first, second)
	}
}

func TestValidateAttachmentPlanArtifactRejectsUnsupportedVersionAndDuplicates(t *testing.T) {
	cases := map[string]struct {
		contents string
		want     string
	}{
		"future": {
			contents: `{"schemaVersion":2,"projectId":"demo","plans":[]}`,
			want:     "1001",
		},
		"duplicate": {
			contents: `{"schemaVersion":1,"projectId":"demo","plans":[{"attachmentId":"a","componentInstanceId":"c","hostBoxId":"Page.host","hostSelector":"#host","adapterId":"CustomElement","lifecycle":{"mount":true,"update":true,"unmount":true}},{"attachmentId":"a","componentInstanceId":"d","hostBoxId":"Page.host","hostSelector":"#host","adapterId":"CustomElement","lifecycle":{"mount":true,"update":true,"unmount":true}}]}`,
			want:     "1002",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "attachments.json")
			if err := os.WriteFile(path, []byte(testCase.contents), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := validateAttachmentPlanArtifact(path)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want diagnostic %s", err, testCase.want)
			}
		})
	}
}

func TestValidateComponentFrameArtifactAcceptsV1AndLegacyBridgeOnly(t *testing.T) {
	cases := map[string]struct {
		contents string
		valid    bool
	}{
		"v1": {
			contents: "export default { schemaVersion: 1, projectId: \"demo\", frameDefinitions: [], frameInstances: [] };\n",
			valid:    true,
		},
		"legacy": {
			contents: "import { registerComponentFrames } from \"@copeland/browser-v1\";\nregisterComponentFrames([]);\n",
			valid:    true,
		},
		"unknown": {
			contents: "export const notAFrameArtifact = true;\n",
			valid:    false,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "component-frames.js")
			if err := os.WriteFile(path, []byte(testCase.contents), 0o644); err != nil {
				t.Fatal(err)
			}
			artifact, err := validateComponentFrameArtifact(path)
			if testCase.valid && err != nil {
				t.Fatalf("validate component frame artifact: %v", err)
			}
			if !testCase.valid && err == nil {
				t.Fatal("expected unsupported component frame artifact to fail")
			}
			if artifact != nil && artifact.SchemaVersion != 1 {
				t.Fatalf("schema version = %d, want 1", artifact.SchemaVersion)
			}
		})
	}
}

func TestBrowserV1LegacyFrameFixtureAndLoaderPolicy(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "fixtures", "browser-v1-legacy-component-frames", "component-frames.js")
	artifact, err := validateComponentFrameArtifact(fixturePath)
	if err != nil {
		t.Fatalf("validate dedicated legacy fixture: %v", err)
	}
	if artifact == nil || artifact.Path != "component-frames.js" {
		t.Fatalf("legacy fixture metadata = %#v", artifact)
	}
	for _, expected := range []string{
		"recordLegacyComponentFrameContract",
		"COPE-COMPONENT-STATE-V1-1020",
		"COPE-COMPONENT-STATE-V1-1021",
		"registerComponentFrameEnvelope",
	} {
		if !strings.Contains(componentFrameLoaderModule, expected) {
			t.Fatalf("component-frame loader is missing compatibility policy %q", expected)
		}
	}
}
