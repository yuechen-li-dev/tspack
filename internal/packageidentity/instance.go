package packageidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

const (
	PeerBindingPresent = "present"
	PeerBindingAbsent  = "absent"
)

// PeerBinding identifies the effective package realization bound to one peer
// dependency. Reference preserves the consumer-visible dependency key while
// Source and Name preserve the semantic peer identity.
type PeerBinding struct {
	Reference     string `json:"reference"`
	Source        string `json:"source"`
	Name          string `json:"name"`
	RealizationID string `json:"realizationIdentity,omitempty"`
	InstanceID    string `json:"moduleInstanceIdentity,omitempty"`
	Optional      bool   `json:"optional,omitempty"`
	State         string `json:"state"`
}

// PeerContext is a canonical, path-independent description of the peer
// environment that can affect one realized package at runtime.
type PeerContext struct {
	ID       string        `json:"id"`
	Bindings []PeerBinding `json:"bindings,omitempty"`
}

// ModuleInstance identifies a package realization instantiated under one
// effective peer context. Store identity deliberately remains the realization
// identity; this identity is for dependency graph and workspace projection use.
type ModuleInstance struct {
	ID            string      `json:"id"`
	RealizationID string      `json:"realizationIdentity"`
	PeerContext   PeerContext `json:"peerContext"`
}

func NewModuleInstance(realizationID string, bindings []PeerBinding) ModuleInstance {
	canonicalBindings := CanonicalPeerBindings(bindings)
	contextDigest := peerContextDigest(canonicalBindings)
	contextID := "peer-context:sha256:" + contextDigest
	return ModuleInstance{
		ID:            realizationID + "#peers=" + contextDigest,
		RealizationID: realizationID,
		PeerContext: PeerContext{
			ID:       contextID,
			Bindings: canonicalBindings,
		},
	}
}

func CanonicalPeerBindings(bindings []PeerBinding) []PeerBinding {
	out := append([]PeerBinding(nil), bindings...)
	for index := range out {
		binding := &out[index]
		binding.Reference = strings.TrimSpace(binding.Reference)
		binding.Source = strings.TrimSpace(binding.Source)
		binding.Name = strings.TrimSpace(binding.Name)
		binding.RealizationID = strings.TrimSpace(binding.RealizationID)
		binding.InstanceID = strings.TrimSpace(binding.InstanceID)
		if binding.State == "" {
			if binding.RealizationID == "" {
				binding.State = PeerBindingAbsent
			} else {
				binding.State = PeerBindingPresent
			}
		}
	}
	sort.SliceStable(out, func(left, right int) bool {
		return peerBindingKey(out[left]) < peerBindingKey(out[right])
	})
	return out
}

func peerContextDigest(bindings []PeerBinding) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("tspack-peer-context-v1\n"))
	for _, binding := range bindings {
		_, _ = hash.Write([]byte(peerBindingKey(binding)))
		_, _ = hash.Write([]byte("\n"))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func peerBindingKey(binding PeerBinding) string {
	optional := "required"
	if binding.Optional {
		optional = "optional"
	}
	return strings.Join([]string{
		binding.Reference,
		binding.Source,
		binding.Name,
		binding.State,
		optional,
		binding.RealizationID,
		binding.InstanceID,
	}, "\x00")
}
