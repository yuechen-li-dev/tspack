package concepts

import (
	"fmt"
	"reflect"
)

func Merge(fragments []Fragment) (*MergedConceptIR, error) {
	merger := &mergeState{ir: &MergedConceptIR{Projections: ProjectionContributions{Objects: map[string]map[string]string{}}}, owners: map[string]string{}, priority: map[string]int{}}
	for index, fragment := range fragments {
		merger.priority[fragment.Name] = index
		if err := merger.add(fragment); err != nil {
			return nil, err
		}
	}
	return merger.ir, nil
}

type mergeState struct {
	ir       *MergedConceptIR
	owners   map[string]string
	priority map[string]int
}

func (m *mergeState) add(fragment Fragment) error {
	m.addConcepts(fragment.Name, fragment.Provides, fragment.Manifest.Concepts)
	for _, variable := range fragment.Variables {
		if err := addUnique(m, "variables."+variable.Name, fragment.Name, variable.Name, variable, &m.ir.Variables); err != nil {
			return err
		}
	}
	if err := m.addDeps("manifest.dependencies", fragment.Name, "dependency", fragment.Manifest.Dependencies, &m.ir.Manifest.Dependencies); err != nil {
		return err
	}
	if err := m.addDeps("manifest.tools", fragment.Name, "tool", fragment.Manifest.Tools, &m.ir.Manifest.Tools); err != nil {
		return err
	}
	if err := m.addDeps("manifest.peers", fragment.Name, "peer", fragment.Manifest.Peers, &m.ir.Manifest.Peers); err != nil {
		return err
	}
	for _, target := range fragment.Manifest.Targets {
		if err := addUnique(m, "manifest.targets."+target.Name, fragment.Name, target.Name, target, &m.ir.Manifest.Targets); err != nil {
			return err
		}
	}
	for _, target := range fragment.Manifest.RunTargets {
		if err := addUnique(m, "manifest.runTargets."+target.Name, fragment.Name, target.Name, target, &m.ir.Manifest.RunTargets); err != nil {
			return err
		}
	}
	for _, env := range fragment.Manifest.Env {
		if err := addUnique(m, "manifest.env."+env.Name, fragment.Name, env.Name, env, &m.ir.Manifest.Env); err != nil {
			return err
		}
	}
	for _, service := range fragment.Manifest.Services {
		if err := addUnique(m, "manifest.services."+service.Name, fragment.Name, service.Name, service, &m.ir.Manifest.Services); err != nil {
			return err
		}
	}
	for _, policy := range fragment.Manifest.UpdatePolicy {
		if err := addUnique(m, "manifest.updatePolicy."+policy.Subject, fragment.Name, policy.Subject, policy, &m.ir.Manifest.UpdatePolicy); err != nil {
			return err
		}
	}
	for _, policy := range fragment.Manifest.SecurityPolicy {
		if err := addUnique(m, "manifest.securityPolicy."+policy.Subject, fragment.Name, policy.Subject, policy, &m.ir.Manifest.SecurityPolicy); err != nil {
			return err
		}
	}
	for _, file := range fragment.Files {
		if err := addUnique(m, "files."+file.Path, fragment.Name, file.Path, file, &m.ir.Files); err != nil {
			return err
		}
	}
	if err := m.mergeProjections(fragment); err != nil {
		return err
	}
	if err := m.mergeSlots(fragment); err != nil {
		return err
	}
	if fragment.Manifest.Workspace != nil {
		if m.ir.Manifest.Workspace != nil && !reflect.DeepEqual(*m.ir.Manifest.Workspace, *fragment.Manifest.Workspace) {
			return m.conflict("manifest.workspace", fragment.Name, "different workspace contribution")
		}
		value := *fragment.Manifest.Workspace
		m.ir.Manifest.Workspace = &value
	}
	if fragment.Manifest.Package != nil {
		if m.ir.Manifest.Package != nil && !reflect.DeepEqual(*m.ir.Manifest.Package, *fragment.Manifest.Package) {
			return m.conflict("manifest.package", fragment.Name, "different package contribution")
		}
		value := *fragment.Manifest.Package
		m.ir.Manifest.Package = &value
	}
	if fragment.Manifest.Pack != nil {
		if m.ir.Manifest.Pack != nil && !reflect.DeepEqual(*m.ir.Manifest.Pack, *fragment.Manifest.Pack) {
			return m.conflict("manifest.pack", fragment.Name, "different pack contribution")
		}
		value := *fragment.Manifest.Pack
		m.ir.Manifest.Pack = &value
	}
	m.ir.Warnings = append(m.ir.Warnings, fragment.Warnings...)
	return nil
}

func (m *mergeState) addConcepts(groups ...interface{}) {
	seen := map[string]bool{}
	for _, c := range m.ir.Concepts {
		seen[c] = true
	}
	for _, group := range groups {
		switch values := group.(type) {
		case string:
			if values != "" && !seen[values] {
				m.ir.Concepts = append(m.ir.Concepts, values)
				seen[values] = true
			}
		case []string:
			for _, value := range values {
				if value != "" && !seen[value] {
					m.ir.Concepts = append(m.ir.Concepts, value)
					seen[value] = true
				}
			}
		}
	}
}

func (m *mergeState) addDeps(path, concept, kind string, values []DependencyContribution, target *[]DependencyContribution) error {
	for _, value := range values {
		if err := m.crossKindDependencyConflict(value.Name, kind, concept); err != nil {
			return err
		}
		if err := addUnique(m, path+"."+value.Name, concept, value.Name, value, target); err != nil {
			return err
		}
	}
	return nil
}

func (m *mergeState) crossKindDependencyConflict(name, kind, concept string) error {
	for _, prefix := range []string{"dependency", "tool", "peer"} {
		owner, ok := m.owners["depkind."+prefix+"."+name]
		if ok && prefix != kind {
			return m.makeConflict("manifest.dependencies."+name, owner, concept, "same package declared as "+prefix+" and "+kind)
		}
	}
	m.owners["depkind."+kind+"."+name] = concept
	return nil
}

func addUnique[T any](m *mergeState, path, concept, key string, value T, target *[]T) error {
	ownerKey := "owner." + path
	for _, existing := range *target {
		if sameKey(existing, key) {
			if reflect.DeepEqual(existing, value) {
				return nil
			}
			return m.conflict(path, concept, "different contribution")
		}
	}
	*target = append(*target, value)
	m.owners[ownerKey] = concept
	return nil
}
func sameKey(value any, key string) bool {
	reflected := reflect.ValueOf(value)
	field := reflected.FieldByName("Name")
	if !field.IsValid() {
		field = reflected.FieldByName("Path")
	}
	if !field.IsValid() {
		field = reflected.FieldByName("Subject")
	}
	return field.IsValid() && field.Kind() == reflect.String && field.String() == key
}
func (m *mergeState) conflict(path, concept, reason string) error {
	return m.makeConflict(path, m.owners["owner."+path], concept, reason)
}

func (m *mergeState) makeConflict(path, conceptA, conceptB, reason string) Conflict {
	higher := conceptA
	lower := conceptB
	if m.priority[conceptB] < m.priority[conceptA] {
		higher = conceptB
		lower = conceptA
	}
	priorityReason := fmt.Sprintf(
		"this contribution path cannot be resolved by priority because %s",
		reason,
	)
	return Conflict{
		Path:                  path,
		ConceptA:              conceptA,
		ConceptB:              conceptB,
		Reason:                priorityReason,
		HigherPriorityConcept: higher,
		LowerPriorityConcept:  lower,
	}
}

func (m *mergeState) mergeProjections(fragment Fragment) error {
	for object, fields := range fragment.Projections.Objects {
		if m.ir.Projections.Objects[object] == nil {
			m.ir.Projections.Objects[object] = map[string]string{}
		}
		for key, value := range fields {
			path := "projections." + object + "." + key
			if existing, ok := m.ir.Projections.Objects[object][key]; ok && existing != value {
				return m.conflict(path, fragment.Name, fmt.Sprintf("scalar differs: %q vs %q", existing, value))
			}
			m.ir.Projections.Objects[object][key] = value
			m.owners["owner."+path] = fragment.Name
		}
	}
	return nil
}
func (m *mergeState) mergeSlots(fragment Fragment) error {
	for _, slot := range fragment.Slots {
		path := "slots." + slot.Name
		if slot.Mode == SlotAppendOrdered {
			m.ir.Slots = append(m.ir.Slots, slot)
			continue
		}
		for _, existing := range m.ir.Slots {
			if existing.Name == slot.Name {
				if reflect.DeepEqual(existing, slot) {
					return nil
				}
				return m.conflict(path, fragment.Name, "slot ownership differs")
			}
		}
		m.ir.Slots = append(m.ir.Slots, slot)
		m.owners["owner."+path] = fragment.Name
	}
	return nil
}

func BuildConceptIR(concepts []string, kind string, registry Registry) (*MergedConceptIR, error) {
	result, err := ResolveWithRegistry(registry, concepts, kind)
	if err != nil {
		return nil, err
	}
	return Merge(result.Fragments)
}
