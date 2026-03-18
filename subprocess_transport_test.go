package claude

import (
	"strings"
	"testing"
)

func TestBuildCommand_BasicFlags(t *testing.T) {
	maxTurns := 5
	budget := 1.50
	transport := &SubprocessTransport{
		cliPath: "/usr/local/bin/claude",
		options: &Options{
			PermissionMode: PermissionModeBypassPermissions,
			MaxTurns:       &maxTurns,
			MaxBudgetUSD:   &budget,
			Model:          "claude-sonnet-4-5-20250514",
		},
	}

	cmd := transport.buildCommand()

	assertContains(t, cmd, "--output-format", "stream-json")
	assertContains(t, cmd, "--permission-mode", "bypassPermissions")
	assertContains(t, cmd, "--max-turns", "5")
	assertContains(t, cmd, "--model", "claude-sonnet-4-5-20250514")
	assertContains(t, cmd, "--input-format", "stream-json")
	assertContainsFlag(t, cmd, "--verbose")
}

func TestBuildCommand_SystemPrompt(t *testing.T) {
	t.Run("nil system prompt sends empty", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--system-prompt", "")
	})

	t.Run("string system prompt", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				SystemPrompt: StringPrompt("You are helpful"),
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--system-prompt", "You are helpful")
	})

	t.Run("preset with append", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				SystemPrompt: PresetPrompt{Preset: "claude_code", Append: "extra instructions"},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--append-system-prompt", "extra instructions")
	})
}

func TestBuildCommand_Tools(t *testing.T) {
	t.Run("explicit tool list", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Tools: []string{"Bash", "Read"},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--tools", "Bash,Read")
	})

	t.Run("empty tool list", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Tools: []string{},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--tools", "")
	})

	t.Run("tools preset", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Tools: &ToolsPreset{Preset: "claude_code"},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--tools", "default")
	})
}

func TestBuildCommand_ThinkingConfig(t *testing.T) {
	t.Run("adaptive", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Thinking: ThinkingConfigAdaptive{},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--max-thinking-tokens", "32000")
	})

	t.Run("enabled", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Thinking: ThinkingConfigEnabled{BudgetTokens: 16000},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--max-thinking-tokens", "16000")
	})

	t.Run("disabled", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Thinking: ThinkingConfigDisabled{},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--max-thinking-tokens", "0")
	})
}

func TestBuildCommand_ExtraArgs(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			ExtraArgs: map[string]string{
				"debug-to-stderr":      "",
				"replay-user-messages": "",
			},
		},
	}
	cmd := transport.buildCommand()
	assertContainsFlag(t, cmd, "--debug-to-stderr")
	assertContainsFlag(t, cmd, "--replay-user-messages")
}

func TestConnectEnv_IncludePartialMessages(t *testing.T) {
	t.Run("does not set CLAUDE_CODE_ENABLE_FINE_GRAINED_TOOL_STREAMING even when true", func(t *testing.T) {
		env := buildTestEnv(&Options{IncludePartialMessages: true})
		assertEnvNotContainsKey(t, env, "CLAUDE_CODE_ENABLE_FINE_GRAINED_TOOL_STREAMING")
	})

	t.Run("does not set CLAUDE_CODE_ENABLE_FINE_GRAINED_TOOL_STREAMING when false", func(t *testing.T) {
		env := buildTestEnv(&Options{IncludePartialMessages: false})
		assertEnvNotContainsKey(t, env, "CLAUDE_CODE_ENABLE_FINE_GRAINED_TOOL_STREAMING")
	})
}

func TestConnectEnv_EntrypointDefaultIfAbsent(t *testing.T) {
	t.Run("sets entrypoint when not in env", func(t *testing.T) {
		env := buildTestEnv(&Options{})
		found := false
		for _, e := range env {
			if e == "CLAUDE_CODE_ENTRYPOINT=sdk-go" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected CLAUDE_CODE_ENTRYPOINT=sdk-go in env")
		}
	})

	t.Run("does not override existing entrypoint", func(t *testing.T) {
		env := buildTestEnv(&Options{
			Env: map[string]string{
				"CLAUDE_CODE_ENTRYPOINT": "custom-value",
			},
		})
		count := 0
		for _, e := range env {
			if strings.HasPrefix(e, "CLAUDE_CODE_ENTRYPOINT=") {
				count++
				if e != "CLAUDE_CODE_ENTRYPOINT=custom-value" {
					t.Errorf("expected custom-value, got %s", e)
				}
			}
		}
		if count != 1 {
			t.Errorf("expected exactly 1 CLAUDE_CODE_ENTRYPOINT entry, got %d", count)
		}
	})
}

// buildTestEnv simulates the env-building logic from Connect without starting a process.
func buildTestEnv(opts *Options) []string {
	env := []string{} // start clean to avoid os.Environ() noise
	for k, v := range opts.Env {
		env = append(env, k+"="+v)
	}
	entrypointSet := false
	for _, e := range env {
		if strings.HasPrefix(e, "CLAUDE_CODE_ENTRYPOINT=") {
			entrypointSet = true
			break
		}
	}
	if !entrypointSet {
		env = append(env, "CLAUDE_CODE_ENTRYPOINT=sdk-go")
	}
	env = append(env, "CLAUDE_AGENT_SDK_VERSION="+sdkVersion)
	if opts.EnableFileCheckpointing {
		env = append(env, "CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING=true")
	}
	return env
}

func assertEnvNotContainsKey(t *testing.T, env []string, key string) {
	t.Helper()
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			t.Errorf("env unexpectedly contains key %s: %s", key, e)
			return
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"2.0.0", "2.0.0", 0},
		{"2.1.0", "2.0.0", 1},
		{"1.9.0", "2.0.0", -1},
		{"2.0.1", "2.0.0", 1},
	}

	for _, tt := range tests {
		got := compareVersions(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// Test helpers

func assertContains(t *testing.T, cmd []string, flag, value string) {
	t.Helper()
	for i, arg := range cmd {
		if arg == flag && i+1 < len(cmd) && cmd[i+1] == value {
			return
		}
	}
	t.Errorf("command %v does not contain %s %s", cmd, flag, value)
}

func assertContainsFlag(t *testing.T, cmd []string, flag string) {
	t.Helper()
	for _, arg := range cmd {
		if arg == flag {
			return
		}
	}
	t.Errorf("command %v does not contain %s", cmd, flag)
}

func TestBuildCommand_SettingSources(t *testing.T) {
	t.Run("nil setting sources sends empty", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--setting-sources", "")
	})

	t.Run("explicit setting sources", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				SettingSources: []SettingSource{SettingSourceUser, SettingSourceProject},
			},
		}
		cmd := transport.buildCommand()
		// Find the --setting-sources flag and check its value
		for i, arg := range cmd {
			if arg == "--setting-sources" && i+1 < len(cmd) {
				val := cmd[i+1]
				if !strings.Contains(val, "user") || !strings.Contains(val, "project") {
					t.Errorf("expected setting sources to contain user,project, got %s", val)
				}
				return
			}
		}
		t.Error("--setting-sources flag not found")
	})
}
