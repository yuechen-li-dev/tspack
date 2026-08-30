package lockfile

import "testing"

func TestRebuildModuleInstancesUsesPatchedSelectedPeerAndOptionalAbsence(t *testing.T) {
	patchedReact := "npm:react@19.0.0#patch=tspatch-v1.abc"
	lock := &Lockfile{
		Packages: []Package{
			{ID: "npm:plugin@1.0.0", Source: "npm", Name: "plugin", Version: "1.0.0"},
			{ID: patchedReact, Source: "npm", Name: "react", Version: "19.0.0", RealizationID: patchedReact},
		},
		Requirements: []Requirement{
			{ID: "peer-react", Kind: "peer", PackageID: "npm:plugin@1.0.0", TargetSource: "npm", TargetName: "react", Reference: "reactAlias", SelectedVersion: "19.0.0"},
			{ID: "peer-vue", Kind: "peer", PackageID: "npm:plugin@1.0.0", TargetSource: "npm", TargetName: "vue", Reference: "vue", Optional: true, Status: "optional-unsatisfied"},
		},
	}
	RebuildModuleInstances(lock)
	if len(lock.Instances) != 2 {
		t.Fatalf("instances=%d, want 2", len(lock.Instances))
	}
	var plugin ModuleInstance
	for _, instance := range lock.Instances {
		if instance.PackageID == "npm:plugin@1.0.0" {
			plugin = instance
		}
	}
	if len(plugin.Peers) != 2 {
		t.Fatalf("plugin peers=%#v", plugin.Peers)
	}
	if plugin.Peers[0].Reference != "reactAlias" || plugin.Peers[0].RealizationID != patchedReact || plugin.Peers[0].State != "present" {
		t.Fatalf("patched alias peer=%#v", plugin.Peers[0])
	}
	if plugin.Peers[1].Reference != "vue" || plugin.Peers[1].State != "absent" || !plugin.Peers[1].Optional {
		t.Fatalf("optional absent peer=%#v", plugin.Peers[1])
	}
}

func TestRebuildModuleInstancesSeparatesConsumerPeerContextsAndConvergesEquivalentOnes(t *testing.T) {
	lock := &Lockfile{
		Packages: []Package{
			{ID: "npm:plugin@1.0.0", Source: "npm", Name: "plugin", Version: "1.0.0"},
			{ID: "npm:react@18.3.1", Source: "npm", Name: "react", Version: "18.3.1"},
			{ID: "npm:react@19.0.0", Source: "npm", Name: "react", Version: "19.0.0"},
		},
		Edges: []Edge{
			{From: "consumer-a:target:client", To: "npm:plugin@1.0.0", Kind: "runtime", Reference: "plugin"},
			{From: "consumer-a:tool", To: "npm:react@18.3.1", Kind: "tool", Reference: "react"},
			{From: "consumer-b:target:client", To: "npm:plugin@1.0.0", Kind: "runtime", Reference: "plugin"},
			{From: "consumer-b:tool", To: "npm:react@19.0.0", Kind: "tool", Reference: "react"},
			{From: "consumer-c:target:client", To: "npm:plugin@1.0.0", Kind: "runtime", Reference: "plugin"},
			{From: "consumer-c:tool", To: "npm:react@18.3.1", Kind: "tool", Reference: "react"},
		},
		Requirements: []Requirement{{
			ID: "peer-plugin-react", Kind: "peer", PackageID: "npm:plugin@1.0.0",
			TargetSource: "npm", TargetName: "react", Reference: "react",
		}},
	}

	RebuildModuleInstances(lock)

	pluginInstances := map[string]ModuleInstance{}
	rootPluginInstances := map[string]string{}
	for _, instance := range lock.Instances {
		if instance.PackageID == "npm:plugin@1.0.0" {
			pluginInstances[instance.ID] = instance
		}
	}
	for _, root := range lock.RootInstances {
		if root.Reference == "plugin" {
			rootPluginInstances[moduleRootConsumer(root.From)] = root.InstanceID
		}
	}
	if len(pluginInstances) != 2 {
		t.Fatalf("plugin instances=%d, want 2: %#v", len(pluginInstances), pluginInstances)
	}
	if rootPluginInstances["consumer-a"] != rootPluginInstances["consumer-c"] {
		t.Fatalf("equivalent React 18 contexts did not converge: %#v", rootPluginInstances)
	}
	if rootPluginInstances["consumer-a"] == rootPluginInstances["consumer-b"] {
		t.Fatalf("React 18 and React 19 contexts collapsed: %#v", rootPluginInstances)
	}
}
