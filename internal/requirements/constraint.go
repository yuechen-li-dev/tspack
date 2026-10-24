package requirements

import (
	"strings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/yuechen-li-dev/tspack/internal/packageidentity"
)

// ParseConstraint keeps registry selection and requirement classification on the
// same range semantics. npm's loose OR handling discards invalid alternatives
// when a valid alternative remains (for example ">=3.0.0 || insiders"). A tag
// alone is not a version range, and other registries retain strict parsing.
func ParseConstraint(source, expression string) (*semver.Constraints, error) {
	constraint, err := semver.NewConstraint(expression)
	if err == nil || source != packageidentity.SourceNPM || !strings.Contains(expression, "||") {
		return constraint, err
	}

	valid := []string{}
	for _, alternative := range strings.Split(expression, "||") {
		alternative = strings.TrimSpace(alternative)
		if _, alternativeErr := semver.NewConstraint(alternative); alternativeErr == nil {
			valid = append(valid, alternative)
		}
	}
	if len(valid) == 0 {
		return nil, err
	}
	return semver.NewConstraint(strings.Join(valid, " || "))
}
