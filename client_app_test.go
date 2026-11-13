package claude

import (
	"strings"
	"testing"
)

func envValues(env []string, key string) []string {
	var out []string
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, key+"="); ok {
			out = append(out, v)
		}
	}
	return out
}

func TestBuildEnv_ClientAppWinsOverEnv(t *testing.T) {
	t.Setenv(clientAppEnv, "inherited/1")
	tr := &SubprocessTransport{options: &Options{
		ClientApp: "my-app/1.0.0",
		Env:       map[string]string{clientAppEnv: "from-env/2"},
	}}
	got := envValues(tr.buildEnv(), clientAppEnv)
	if len(got) == 0 || got[len(got)-1] != "my-app/1.0.0" {
		t.Fatalf("last %s = %v, want ClientApp to win", clientAppEnv, got)
	}
}

func TestBuildEnv_EmptyClientAppLeavesEnv(t *testing.T) {
	tr := &SubprocessTransport{options: &Options{Env: map[string]string{clientAppEnv: "from-env/2"}}}
	got := envValues(tr.buildEnv(), clientAppEnv)
	if len(got) == 0 || got[len(got)-1] != "from-env/2" {
		t.Fatalf("last %s = %v, want the Env value", clientAppEnv, got)
	}
}

func TestBuildEnv_DropsDerivedClientApp(t *testing.T) {
	t.Setenv(derivedClientAppEnv, "outer-app/9")
	tr := &SubprocessTransport{options: &Options{Env: map[string]string{derivedClientAppEnv: "x"}}}
	env := tr.buildEnv()
	if got := envValues(env, derivedClientAppEnv); len(got) != 0 {
		t.Fatalf("%s must never reach the CLI, got %v", derivedClientAppEnv, got)
	}
	if got := envValues(env, "CLAUDE_AGENT_SDK_VERSION"); len(got) != 1 || got[0] != sdkVersion {
		t.Fatalf("CLAUDE_AGENT_SDK_VERSION = %v", got)
	}
}
