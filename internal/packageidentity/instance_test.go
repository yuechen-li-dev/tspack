package packageidentity

import "testing"

func TestModuleInstanceCanonicalizesEquivalentPeerContexts(t *testing.T) {
	left := NewModuleInstance("npm:foo@1.0.0", []PeerBinding{
		{Reference: "vue", Source: "npm", Name: "vue", RealizationID: "npm:vue@3.5.0", State: PeerBindingPresent},
		{Reference: "react", Source: "npm", Name: "react", RealizationID: "npm:react@19.0.0", State: PeerBindingPresent},
	})
	right := NewModuleInstance("npm:foo@1.0.0", []PeerBinding{
		{Reference: "react", Source: "npm", Name: "react", RealizationID: "npm:react@19.0.0", State: PeerBindingPresent},
		{Reference: "vue", Source: "npm", Name: "vue", RealizationID: "npm:vue@3.5.0", State: PeerBindingPresent},
	})
	if left.ID != right.ID || left.PeerContext.ID != right.PeerContext.ID {
		t.Fatalf("equivalent peer contexts diverged:\nleft=%#v\nright=%#v", left, right)
	}
}

func TestModuleInstanceSeparatesDifferentPeerRealizations(t *testing.T) {
	react18 := NewModuleInstance("npm:foo@1.0.0", []PeerBinding{{Reference: "react", Source: "npm", Name: "react", RealizationID: "npm:react@18.3.1", State: PeerBindingPresent}})
	react19 := NewModuleInstance("npm:foo@1.0.0", []PeerBinding{{Reference: "react", Source: "npm", Name: "react", RealizationID: "npm:react@19.0.0", State: PeerBindingPresent}})
	if react18.ID == react19.ID {
		t.Fatalf("different peer realizations collapsed to %s", react18.ID)
	}
}

func TestModuleInstanceSeparatesOptionalPeerAbsenceAndPresence(t *testing.T) {
	absent := NewModuleInstance("npm:foo@1.0.0", []PeerBinding{{Reference: "react", Source: "npm", Name: "react", Optional: true, State: PeerBindingAbsent}})
	present := NewModuleInstance("npm:foo@1.0.0", []PeerBinding{{Reference: "react", Source: "npm", Name: "react", RealizationID: "npm:react@19.0.0", Optional: true, State: PeerBindingPresent}})
	if absent.ID == present.ID {
		t.Fatalf("optional peer absence and presence collapsed to %s", absent.ID)
	}
}

func TestModuleInstanceUsesPatchedAndSourceQualifiedPeerIdentity(t *testing.T) {
	raw := NewModuleInstance("npm:foo@1.0.0", []PeerBinding{{Reference: "reactAlias", Source: "npm", Name: "react", RealizationID: "npm:react@19.0.0", State: PeerBindingPresent}})
	patched := NewModuleInstance("npm:foo@1.0.0", []PeerBinding{{Reference: "reactAlias", Source: "npm", Name: "react", RealizationID: "npm:react@19.0.0#patch=tspatch-v1.abc", State: PeerBindingPresent}})
	alternateSource := NewModuleInstance("npm:foo@1.0.0", []PeerBinding{{Reference: "reactAlias", Source: "jsr", Name: "@scope/react", RealizationID: "jsr:@scope/react@19.0.0", State: PeerBindingPresent}})
	if raw.ID == patched.ID || raw.ID == alternateSource.ID || patched.ID == alternateSource.ID {
		t.Fatalf("patched or source-qualified peers were collapsed: raw=%s patched=%s alternate=%s", raw.ID, patched.ID, alternateSource.ID)
	}
}
