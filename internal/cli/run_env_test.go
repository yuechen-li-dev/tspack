package cli

import (
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestRunTargetEnvContractDefaultsAndMissing(t *testing.T) {
	defaultPort := "3000"
	target := manifest.RunTarget{Name: "dev", Env: []manifest.RunTargetEnv{{Name: "DATABASE_URL", Required: true, Secret: true, Description: "db"}, {Name: "PORT", Default: &defaultPort}}}
	env, missing := applyRunTargetEnvContract([]string{"PATH=/bin"}, target)
	if len(missing) != 1 || missing[0].Name != "DATABASE_URL" {
		t.Fatalf("expected missing DATABASE_URL, got %#v", missing)
	}
	if !containsEnv(env, "PORT=3000") {
		t.Fatalf("default PORT was not injected: %#v", env)
	}
	err := missingRunTargetEnvError(target, missing)
	if err.code != "TSPACK_RUN_ENV_MISSING" || !strings.Contains(err.msg, "DATABASE_URL") || strings.Contains(err.msg, "postgres://secret") {
		t.Fatalf("unexpected missing env error: %#v", err)
	}
}

func TestRunTargetEnvContractHostOverridesDefault(t *testing.T) {
	defaultPort := "3000"
	target := manifest.RunTarget{Name: "dev", Env: []manifest.RunTargetEnv{{Name: "PORT", Default: &defaultPort}}}
	env, missing := applyRunTargetEnvContract([]string{"PORT=4000"}, target)
	if len(missing) != 0 {
		t.Fatalf("unexpected missing env: %#v", missing)
	}
	if !containsEnv(env, "PORT=4000") || containsEnv(env, "PORT=3000") {
		t.Fatalf("host env should override default: %#v", env)
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
