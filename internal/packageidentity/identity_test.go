package packageidentity

import "testing"

func TestNodeUsageSeparatesJSRSemanticAndCompatibilityIdentity(t *testing.T) {
	usage, err := NodeUsage(PackageIdentity{Source: SourceJSR, Name: "@luca/flag"})
	if err != nil {
		t.Fatal(err)
	}
	if usage.Semantic.Key() != "jsr:@luca/flag" {
		t.Fatalf("semantic identity = %#v", usage.Semantic)
	}
	if usage.MaterializedAs.Source != SourceNPMCompat || usage.MaterializedAs.Name != "@jsr/luca__flag" {
		t.Fatalf("materialization identity = %#v", usage.MaterializedAs)
	}
	if usage.Import.Runtime != RuntimeNode || usage.Import.Specifier != "@jsr/luca__flag" {
		t.Fatalf("import identity = %#v", usage.Import)
	}
}

func TestJSRCompatibilityNamesRoundTripAndRejectAmbiguity(t *testing.T) {
	valid := []string{
		"@scope/package",
		"@scope_name/package_name",
		"@scope-name/package-name",
	}
	for _, name := range valid {
		compatibilityName, err := JSRCompatibilityName(name)
		if err != nil {
			t.Fatalf("map %s: %v", name, err)
		}
		logicalName, err := LogicalJSRName(compatibilityName)
		if err != nil {
			t.Fatalf("reverse %s: %v", compatibilityName, err)
		}
		if logicalName != name {
			t.Fatalf("round trip %s = %s", name, logicalName)
		}
	}

	invalidSemantic := []string{"foo", "@scope", "@scope/pkg/extra", "@a__b/c", "@a/b__c"}
	for _, name := range invalidSemantic {
		if _, err := JSRCompatibilityName(name); err == nil {
			t.Fatalf("expected semantic name %q to fail", name)
		}
	}

	invalidCompatibility := []string{"@jsr/no-separator", "@jsr/a____b", "@jsr/__b", "@jsr/a__"}
	for _, name := range invalidCompatibility {
		if _, err := LogicalJSRName(name); err == nil {
			t.Fatalf("expected compatibility name %q to fail", name)
		}
	}
}
