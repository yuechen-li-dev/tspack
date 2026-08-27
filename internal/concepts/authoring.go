package concepts

import (
	"strconv"

	"github.com/yuechen-li-dev/tspack/internal/authoring"
)

// DependencyDeclarations lowers concept fragments before merge so duplicate
// and shadowed declarations remain available to the authoring tape. The first
// listed concept has the highest priority, matching Resolve's established
// explicit-stack contract, so lower-priority concepts are assigned earlier
// tape positions.
func DependencyDeclarations(fragments []Fragment) []authoring.DependencyDeclaration {
	var declarations []authoring.DependencyDeclaration
	order := 0
	for fragmentIndex, fragment := range fragments {
		layerOrder := len(fragments) - fragmentIndex
		declarations = appendConceptDependencies(
			declarations,
			fragment,
			fragment.Manifest.Dependencies,
			authoring.DependencyRuntime,
			layerOrder,
			&order,
		)
		declarations = appendConceptDependencies(
			declarations,
			fragment,
			fragment.Manifest.Tools,
			authoring.DependencyTool,
			layerOrder,
			&order,
		)
		declarations = appendConceptDependencies(
			declarations,
			fragment,
			fragment.Manifest.Peers,
			authoring.DependencyPeer,
			layerOrder,
			&order,
		)
	}
	return declarations
}

func appendConceptDependencies(
	declarations []authoring.DependencyDeclaration,
	fragment Fragment,
	dependencies []DependencyContribution,
	kind authoring.DependencyKind,
	layerOrder int,
	order *int,
) []authoring.DependencyDeclaration {
	for dependencyIndex, dependency := range dependencies {
		sourceKind := dependency.Source
		if sourceKind == "" {
			sourceKind = string(authoring.SourceNPM)
		}
		source := authoring.PackageSource{
			Kind:    sourceKind,
			Package: dependency.Name,
			Range:   dependency.Range,
		}
		declarations = append(declarations, authoring.DependencyDeclaration{
			ID:          conceptDeclarationID(fragment.Name, kind, dependency.Name, dependencyIndex),
			Identity:    source.Identity(),
			Source:      source,
			Constraint:  dependency.Range,
			Kind:        kind,
			Origin:      authoring.DeclarationOrigin{Kind: authoring.OriginConcept, Name: fragment.Name},
			Layer:       authoring.LayerConcept,
			LayerOrder:  layerOrder,
			Order:       *order,
			Authority:   authoring.AuthorityGenerated,
			Editability: authoring.EditabilityConceptOwned,
		})
		*order = *order + 1
	}
	return declarations
}

func conceptDeclarationID(concept string, kind authoring.DependencyKind, name string, index int) string {
	return concept + ":" + string(kind) + ":" + name + ":" + strconv.Itoa(index)
}
