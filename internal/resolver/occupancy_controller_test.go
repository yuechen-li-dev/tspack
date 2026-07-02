package resolver

import (
	"reflect"
	"testing"
)

func TestResolveOccupancyControllerDecide(t *testing.T) {
	controller := NewResolveOccupancyController(ResolveControllerModeFeedforward, 24, 0)

	t.Run("zero work", func(t *testing.T) {
		decision := controller.Decide(FrontierInput{})
		if decision.TargetJobs != 0 {
			t.Fatalf("target=%d want 0", decision.TargetJobs)
		}
		if !reflect.DeepEqual(decision.ClampReasons, []string{"no_work"}) {
			t.Fatalf("clamp reasons=%v", decision.ClampReasons)
		}
	})

	t.Run("frontier width one", func(t *testing.T) {
		decision := controller.Decide(FrontierInput{WorkItems: 1, FrontierWidth: 1, MetadataItems: 1})
		if decision.TargetJobs != 1 {
			t.Fatalf("target=%d want 1", decision.TargetJobs)
		}
	})

	t.Run("frontier smaller than max", func(t *testing.T) {
		decision := controller.Decide(FrontierInput{WorkItems: 4, FrontierWidth: 4, MetadataItems: 4})
		if decision.TargetJobs != 4 {
			t.Fatalf("target=%d want 4", decision.TargetJobs)
		}
	})

	t.Run("frontier larger than max", func(t *testing.T) {
		decision := controller.Decide(FrontierInput{WorkItems: 32, FrontierWidth: 32, MetadataItems: 32})
		if decision.TargetJobs != 24 {
			t.Fatalf("target=%d want 24", decision.TargetJobs)
		}
		if !reflect.DeepEqual(decision.ClampReasons, []string{"frontier_width", "max_jobs"}) {
			t.Fatalf("clamp reasons=%v", decision.ClampReasons)
		}
	})

	t.Run("single host default does not clamp below fixed", func(t *testing.T) {
		fixedDecision := NewResolveOccupancyController(ResolveControllerModeFixed, 24, 0).Decide(FrontierInput{
			WorkItems:     32,
			FrontierWidth: 32,
			MetadataItems: 32,
			Hosts:         map[string]int{"registry.example.test": 32},
		})
		feedforwardDecision := controller.Decide(FrontierInput{
			WorkItems:     32,
			FrontierWidth: 32,
			MetadataItems: 32,
			Hosts:         map[string]int{"registry.example.test": 32},
		})
		if fixedDecision.TargetJobs != 24 {
			t.Fatalf("fixed target=%d want 24", fixedDecision.TargetJobs)
		}
		if feedforwardDecision.TargetJobs != fixedDecision.TargetJobs {
			t.Fatalf("feedforward target=%d want fixed target %d", feedforwardDecision.TargetJobs, fixedDecision.TargetJobs)
		}
		if hasClampReason(feedforwardDecision.ClampReasons, "host_budget") {
			t.Fatalf("default feedforward reported host_budget clamp: %v", feedforwardDecision.ClampReasons)
		}
	})

	t.Run("frontier width clamps", func(t *testing.T) {
		decision := controller.Decide(FrontierInput{
			WorkItems:     8,
			FrontierWidth: 8,
			MetadataItems: 8,
			Hosts:         map[string]int{"registry.example.test": 8},
		})
		if decision.TargetJobs != 8 {
			t.Fatalf("target=%d want 8", decision.TargetJobs)
		}
		if !reflect.DeepEqual(decision.ClampReasons, []string{"frontier_width"}) {
			t.Fatalf("clamp reasons=%v", decision.ClampReasons)
		}
	})

	t.Run("explicit host budget clamps", func(t *testing.T) {
		explicitController := NewResolveOccupancyController(ResolveControllerModeFeedforward, 24, 16)
		decision := explicitController.Decide(FrontierInput{
			WorkItems:     24,
			FrontierWidth: 24,
			MetadataItems: 24,
			Hosts:         map[string]int{"registry.example.test": 24},
		})
		if decision.TargetJobs != 16 {
			t.Fatalf("target=%d want 16", decision.TargetJobs)
		}
		if !reflect.DeepEqual(decision.ClampReasons, []string{"frontier_width", "host_budget"}) {
			t.Fatalf("clamp reasons=%v", decision.ClampReasons)
		}
	})

	t.Run("multi host default and explicit budgets", func(t *testing.T) {
		hosts := map[string]int{"a.example.test": 12, "b.example.test": 12}
		defaultDecision := controller.Decide(FrontierInput{WorkItems: 32, FrontierWidth: 32, MetadataItems: 32, Hosts: hosts})
		if defaultDecision.TargetJobs != 24 {
			t.Fatalf("default target=%d want 24", defaultDecision.TargetJobs)
		}
		explicitDecision := NewResolveOccupancyController(ResolveControllerModeFeedforward, 24, 8).Decide(FrontierInput{
			WorkItems:     32,
			FrontierWidth: 32,
			MetadataItems: 32,
			Hosts:         hosts,
		})
		if explicitDecision.TargetJobs != 16 {
			t.Fatalf("explicit target=%d want 16", explicitDecision.TargetJobs)
		}
		if !reflect.DeepEqual(explicitDecision.ClampReasons, []string{"frontier_width", "max_jobs", "host_budget"}) {
			t.Fatalf("explicit clamp reasons=%v", explicitDecision.ClampReasons)
		}
	})

	t.Run("serial forced", func(t *testing.T) {
		decision := NewResolveOccupancyController(ResolveControllerModeFeedforward, 1, 0).Decide(FrontierInput{
			WorkItems:     9,
			FrontierWidth: 9,
			MetadataItems: 9,
		})
		if decision.TargetJobs != 1 {
			t.Fatalf("target=%d want 1", decision.TargetJobs)
		}
		if !reflect.DeepEqual(decision.ClampReasons, []string{"serial_forced"}) {
			t.Fatalf("clamp reasons=%v", decision.ClampReasons)
		}
	})
}

func TestParseResolveControllerMode(t *testing.T) {
	cases := []struct {
		value string
		want  ResolveControllerMode
		ok    bool
	}{
		{value: "", want: ResolveControllerModeFixed, ok: true},
		{value: "feedforward", want: ResolveControllerModeFeedforward, ok: true},
		{value: "fixed", want: ResolveControllerModeFixed, ok: true},
		{value: "nope", ok: false},
	}

	for _, tc := range cases {
		got, ok := ParseResolveControllerMode(tc.value)
		if ok != tc.ok {
			t.Fatalf("value=%q ok=%v want %v", tc.value, ok, tc.ok)
		}
		if got != tc.want {
			t.Fatalf("value=%q mode=%q want %q", tc.value, got, tc.want)
		}
	}
}

func hasClampReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
