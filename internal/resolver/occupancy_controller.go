package resolver

import (
	"net/url"
	"sort"
	"strings"
)

type ResolveControllerMode string

const (
	ResolveControllerModeFixed       ResolveControllerMode = "fixed"
	ResolveControllerModeFeedforward ResolveControllerMode = "feedforward"
)

const defaultResolveControllerHostBudget = 16

type ResolveOccupancyController struct {
	Mode        ResolveControllerMode
	MaxJobs     int
	MinJobs     int
	PerHostJobs int
}

type FrontierInput struct {
	FrontierIndex int
	FrontierWidth int
	WorkItems     int
	MetadataItems int
	TarballItems  int
	Hosts         map[string]int
}

type FrontierDecision struct {
	Mode          ResolveControllerMode
	FrontierIndex int
	FrontierWidth int
	WorkItems     int
	MetadataItems int
	TarballItems  int
	MaxJobs       int
	TargetJobs    int
	ClampReasons  []string
	Hosts         []string
}

func NewResolveOccupancyController(mode ResolveControllerMode, maxJobs int, perHostJobs int) ResolveOccupancyController {
	if mode == "" {
		mode = ResolveControllerModeFixed
	}
	if maxJobs <= 0 {
		maxJobs = defaultResolveJobs
	}
	if perHostJobs <= 0 {
		perHostJobs = defaultResolveControllerHostBudget
	}
	return ResolveOccupancyController{
		Mode:        mode,
		MaxJobs:     maxJobs,
		MinJobs:     1,
		PerHostJobs: perHostJobs,
	}
}

func (c ResolveOccupancyController) Decide(input FrontierInput) FrontierDecision {
	decision := FrontierDecision{
		Mode:          c.Mode,
		FrontierIndex: input.FrontierIndex,
		FrontierWidth: input.FrontierWidth,
		WorkItems:     input.WorkItems,
		MetadataItems: input.MetadataItems,
		TarballItems:  input.TarballItems,
		MaxJobs:       c.MaxJobs,
		Hosts:         sortedHostKeys(input.Hosts),
	}

	if input.WorkItems <= 0 {
		decision.ClampReasons = []string{"no_work"}
		return decision
	}

	if c.MaxJobs <= 1 {
		decision.TargetJobs = 1
		decision.ClampReasons = []string{"serial_forced"}
		return decision
	}

	target := input.WorkItems
	addClampReason(&decision.ClampReasons, "frontier_width")

	if target > c.MaxJobs {
		target = c.MaxJobs
		addClampReason(&decision.ClampReasons, "max_jobs")
	}

	if c.Mode == ResolveControllerModeFeedforward && c.PerHostJobs > 0 && len(input.Hosts) > 0 {
		hostBudget := len(input.Hosts) * c.PerHostJobs
		if hostBudget < target {
			target = hostBudget
			addClampReason(&decision.ClampReasons, "host_budget")
		}
	}

	if target < c.MinJobs {
		target = c.MinJobs
	}
	if target > input.WorkItems {
		target = input.WorkItems
	}

	decision.TargetJobs = target
	if len(decision.ClampReasons) == 0 && target == input.WorkItems {
		addClampReason(&decision.ClampReasons, "frontier_width")
	}
	return decision
}

func ResolveControllerHostMap(registryURL string) map[string]int {
	registryURL = strings.TrimSpace(registryURL)
	if registryURL == "" {
		return nil
	}
	parsed, err := url.Parse(registryURL)
	if err != nil || parsed.Host == "" {
		return nil
	}
	return map[string]int{parsed.Host: 1}
}

func ParseResolveControllerMode(value string) (ResolveControllerMode, bool) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "":
		return ResolveControllerModeFixed, true
	case string(ResolveControllerModeFeedforward):
		return ResolveControllerModeFeedforward, true
	case string(ResolveControllerModeFixed):
		return ResolveControllerModeFixed, true
	default:
		return "", false
	}
}

func addClampReason(reasons *[]string, reason string) {
	for _, existing := range *reasons {
		if existing == reason {
			return
		}
	}
	*reasons = append(*reasons, reason)
}

func sortedHostKeys(hosts map[string]int) []string {
	if len(hosts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(hosts))
	for host := range hosts {
		keys = append(keys, host)
	}
	sort.Strings(keys)
	return keys
}
