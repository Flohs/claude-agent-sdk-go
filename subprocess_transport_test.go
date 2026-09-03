package claude

import (
	"context"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestBuildCommand_PermissionPrompts(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "/usr/local/bin/claude",
		options: &Options{
			PermissionPrompts: "none",
		},
	}

	cmd := transport.buildCommand()

	assertContains(t, cmd, "--permission-prompts", "none")
}

func TestBuildCommand_PermissionPromptsOmittedWhenEmpty(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "/usr/local/bin/claude",
		options: &Options{},
	}

	cmd := transport.buildCommand()

	for _, arg := range cmd {
		if arg == "--permission-prompts" {
			t.Fatalf("expected --permission-prompts to be omitted when PermissionPrompts is empty, got %v", cmd)
		}
	}
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

	t.Run("custom prompt with snapshot", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				SystemPrompt: CustomPrompt{Prompt: "You are helpful", Snapshot: true},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--system-prompt", "You are helpful")
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
	t.Run("adaptive uses --thinking flag", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Thinking: ThinkingConfigAdaptive{},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--thinking", "adaptive")
		assertNotContainsFlag(t, cmd, "--max-thinking-tokens")
	})

	t.Run("disabled uses --thinking flag", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Thinking: ThinkingConfigDisabled{},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--thinking", "disabled")
		assertNotContainsFlag(t, cmd, "--max-thinking-tokens")
	})

	t.Run("enabled uses --max-thinking-tokens", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Thinking: ThinkingConfigEnabled{BudgetTokens: 16000},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--max-thinking-tokens", "16000")
		assertNotContainsFlag(t, cmd, "--thinking")
	})

	t.Run("deprecated MaxThinkingTokens fallback", func(t *testing.T) {
		v := 8000
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				MaxThinkingTokens: &v,
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--max-thinking-tokens", "8000")
		assertNotContainsFlag(t, cmd, "--thinking")
	})

	t.Run("Thinking takes precedence over MaxThinkingTokens", func(t *testing.T) {
		v := 8000
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Thinking:          ThinkingConfigAdaptive{},
				MaxThinkingTokens: &v,
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--thinking", "adaptive")
		assertNotContainsFlag(t, cmd, "--max-thinking-tokens")
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

func TestBuildCommand_ExtraArgsFlagLikeValue(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			ExtraArgs: map[string]string{
				"resume": "--version",
			},
		},
	}
	cmd := transport.buildCommand()
	assertContainsFlag(t, cmd, "--resume=--version")
	assertNotContainsFlag(t, cmd, "--version")
}

func TestBuildCommand_ResumeFlagLikeValue(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			Resume: "--version",
		},
	}
	cmd := transport.buildCommand()
	assertContainsFlag(t, cmd, "--resume=--version")
	assertNotContainsFlag(t, cmd, "--version")
}

func TestBuildCommand_SessionIDFlagLikeValue(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			SessionID: "--version",
		},
	}
	cmd := transport.buildCommand()
	assertContainsFlag(t, cmd, "--session-id=--version")
	assertNotContainsFlag(t, cmd, "--version")
}

func TestBuildCommand_ResumeSessionAt(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			Resume:          "session-abc",
			ResumeSessionAt: "msg-uuid-123",
		},
	}
	cmd := transport.buildCommand()
	assertContains(t, cmd, "--resume", "session-abc")
	assertContains(t, cmd, "--resume-session-at", "msg-uuid-123")
}

func TestBuildCommand_ResumeSessionAtFlagLikeValue(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			ResumeSessionAt: "--version",
		},
	}
	cmd := transport.buildCommand()
	assertContainsFlag(t, cmd, "--resume-session-at=--version")
	assertNotContainsFlag(t, cmd, "--version")
}

func TestBuildCommand_ResumeDropsTurn(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			Resume:          "session-abc",
			ResumeSessionAt: "msg-uuid-123",
			ResumeDropsTurn: "prompt-uuid-456",
		},
	}
	cmd := transport.buildCommand()
	assertContains(t, cmd, "--resume-drops-turn", "prompt-uuid-456")
}

func TestBuildCommand_ResumeDropsTurnFlagLikeValue(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			ResumeDropsTurn: "--version",
		},
	}
	cmd := transport.buildCommand()
	assertContainsFlag(t, cmd, "--resume-drops-turn=--version")
	assertNotContainsFlag(t, cmd, "--version")
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

func TestConnectEnv_EntrypointEmptyStringIsRespected(t *testing.T) {
	env := buildTestEnv(&Options{
		Env: map[string]string{
			"CLAUDE_CODE_ENTRYPOINT": "",
		},
	})

	// An explicit empty string should still count as "set" and prevent the default
	count := 0
	for _, e := range env {
		if strings.HasPrefix(e, "CLAUDE_CODE_ENTRYPOINT=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 CLAUDE_CODE_ENTRYPOINT entry, got %d", count)
	}
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
	if opts.TraceParent != "" {
		env = append(env, "TRACEPARENT="+opts.TraceParent)
	}
	if opts.TraceState != "" {
		env = append(env, "TRACESTATE="+opts.TraceState)
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

func assertEnvContains(t *testing.T, env []string, key, value string) {
	t.Helper()
	target := key + "=" + value
	for _, e := range env {
		if e == target {
			return
		}
	}
	t.Errorf("env missing %s", target)
}

func TestConnectEnv_TraceContext(t *testing.T) {
	t.Run("unset omits both env vars", func(t *testing.T) {
		env := buildTestEnv(&Options{})
		assertEnvNotContainsKey(t, env, "TRACEPARENT")
		assertEnvNotContainsKey(t, env, "TRACESTATE")
	})

	t.Run("TraceParent only", func(t *testing.T) {
		env := buildTestEnv(&Options{
			TraceParent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		})
		assertEnvContains(t, env, "TRACEPARENT", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
		assertEnvNotContainsKey(t, env, "TRACESTATE")
	})

	t.Run("TraceParent and TraceState", func(t *testing.T) {
		env := buildTestEnv(&Options{
			TraceParent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			TraceState:  "vendor1=value1,vendor2=value2",
		})
		assertEnvContains(t, env, "TRACEPARENT", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
		assertEnvContains(t, env, "TRACESTATE", "vendor1=value1,vendor2=value2")
	})
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

func assertNotContainsFlag(t *testing.T, cmd []string, flag string) {
	t.Helper()
	for _, arg := range cmd {
		if arg == flag {
			t.Errorf("command %v should not contain %s", cmd, flag)
			return
		}
	}
}

func TestBuildCommand_Skills(t *testing.T) {
	t.Run("nil skills preserves user config", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				AllowedTools: []string{"Read"},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--allowedTools", "Read")
		assertNotContainsFlag(t, cmd, "--setting-sources")
	})

	t.Run("skills all injects Skill tool and defaults setting sources", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{Skills: "all"},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--allowedTools", "Skill")
		found := false
		for i, a := range cmd {
			if a == "--setting-sources" && i+1 < len(cmd) {
				if !strings.Contains(cmd[i+1], "user") || !strings.Contains(cmd[i+1], "project") {
					t.Errorf("expected user,project default, got %s", cmd[i+1])
				}
				found = true
			}
		}
		if !found {
			t.Error("expected --setting-sources to be defaulted when skills is set")
		}
	})

	t.Run("skills list injects Skill(name) patterns", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{Skills: []string{"pdf-tools", "image-tools"}},
		}
		cmd := transport.buildCommand()
		for i, a := range cmd {
			if a == "--allowedTools" && i+1 < len(cmd) {
				v := cmd[i+1]
				if !strings.Contains(v, "Skill(pdf-tools)") || !strings.Contains(v, "Skill(image-tools)") {
					t.Errorf("expected both Skill(name) patterns, got %s", v)
				}
				return
			}
		}
		t.Error("--allowedTools not found")
	})

	t.Run("skills respects explicit setting sources", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Skills:         "all",
				SettingSources: []SettingSource{SettingSourceLocal},
			},
		}
		cmd := transport.buildCommand()
		for i, a := range cmd {
			if a == "--setting-sources" && i+1 < len(cmd) {
				if cmd[i+1] != "local" {
					t.Errorf("expected explicit setting-sources to be preserved, got %s", cmd[i+1])
				}
				return
			}
		}
		t.Error("--setting-sources not found")
	})

	t.Run("skills with explicit tool list injects Skill into --tools", func(t *testing.T) {
		// Bug #297: when Skills is set alongside an explicit []string Tools,
		// "Skill" must appear in --tools so the model can actually invoke skills.
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Skills: "all",
				Tools:  []string{"Read"},
			},
		}
		cmd := transport.buildCommand()
		for i, a := range cmd {
			if a == "--tools" && i+1 < len(cmd) {
				v := cmd[i+1]
				if !strings.Contains(v, "Read") {
					t.Errorf("expected Read in --tools, got %s", v)
				}
				if !strings.Contains(v, "Skill") {
					t.Errorf("expected Skill injected into --tools, got %s", v)
				}
				return
			}
		}
		t.Error("--tools flag not found")
	})

	t.Run("skills with explicit tool list is idempotent when Skill already present", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Skills: "all",
				Tools:  []string{"Read", "Skill"},
			},
		}
		cmd := transport.buildCommand()
		for i, a := range cmd {
			if a == "--tools" && i+1 < len(cmd) {
				v := cmd[i+1]
				count := strings.Count(v, "Skill")
				if count != 1 {
					t.Errorf("expected exactly one Skill in --tools, got %d in %s", count, v)
				}
				return
			}
		}
		t.Error("--tools flag not found")
	})

	t.Run("skills with nil Tools does not emit --tools", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Skills: "all",
			},
		}
		cmd := transport.buildCommand()
		assertNotContainsFlag(t, cmd, "--tools")
	})

	t.Run("skills with empty tool list does not inject Skill", func(t *testing.T) {
		// An empty []string means the caller explicitly wants no tools.
		// Skills injection only applies when the explicit list is non-empty.
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Skills: "all",
				Tools:  []string{},
			},
		}
		cmd := transport.buildCommand()
		for i, a := range cmd {
			if a == "--tools" && i+1 < len(cmd) {
				if cmd[i+1] != "" {
					t.Errorf("expected empty --tools value, got %s", cmd[i+1])
				}
				return
			}
		}
		t.Error("--tools flag not found")
	})

	t.Run("skills preset does not get Skill injected into --tools", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				Skills: "all",
				Tools:  &ToolsPreset{Preset: "claude_code"},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--tools", "default")
	})
}

func TestBuildSettingsValue_SandboxFailIfUnavailable(t *testing.T) {
	trueVal := true
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			Sandbox: &SandboxSettings{
				Enabled:           &trueVal,
				FailIfUnavailable: &trueVal,
			},
		},
	}
	value := transport.buildSettingsValue()
	if !strings.Contains(value, `"failIfUnavailable":true`) {
		t.Errorf("expected failIfUnavailable in settings JSON, got %s", value)
	}
}

func TestBuildSettingsValue_SandboxNetworkStrictAllowlist(t *testing.T) {
	trueVal := true
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			Sandbox: &SandboxSettings{
				Network: &SandboxNetworkConfig{
					AllowedDomains:  []string{"example.com"},
					StrictAllowlist: &trueVal,
				},
			},
		},
	}
	value := transport.buildSettingsValue()
	if !strings.Contains(value, `"strictAllowlist":true`) {
		t.Errorf("expected strictAllowlist in settings JSON, got %s", value)
	}
}

func TestBuildSettingsValue_WorkflowSizeGuideline(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			WorkflowSizeGuideline: WorkflowSizeGuidelineLarge,
		},
	}
	value := transport.buildSettingsValue()
	if !strings.Contains(value, `"workflowSizeGuideline":"large"`) {
		t.Errorf("expected workflowSizeGuideline in settings JSON, got %s", value)
	}
}

func TestBuildSettingsValue_WorkflowSizeGuidelineMergesWithSandboxAndSettings(t *testing.T) {
	trueVal := true
	transport := &SubprocessTransport{
		cliPath: "claude",
		options: &Options{
			Settings:              `{"env":{"FOO":"bar"}}`,
			Sandbox:               &SandboxSettings{Enabled: &trueVal},
			WorkflowSizeGuideline: WorkflowSizeGuidelineSmall,
		},
	}
	value := transport.buildSettingsValue()
	for _, want := range []string{`"workflowSizeGuideline":"small"`, `"enabled":true`, `"FOO":"bar"`} {
		if !strings.Contains(value, want) {
			t.Errorf("expected %s in merged settings JSON, got %s", want, value)
		}
	}
}

func TestBuildCommand_ThinkingDisplay(t *testing.T) {
	t.Run("no display omits flag", func(t *testing.T) {
		transport := &SubprocessTransport{cliPath: "claude", options: &Options{
			Thinking: ThinkingConfigAdaptive{},
		}}
		cmd := transport.buildCommand()
		assertNotContainsFlag(t, cmd, "--thinking-display")
	})
	t.Run("adaptive with summarized display", func(t *testing.T) {
		transport := &SubprocessTransport{cliPath: "claude", options: &Options{
			Thinking: ThinkingConfigAdaptive{Display: ThinkingDisplaySummarized},
		}}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--thinking", "adaptive")
		assertContains(t, cmd, "--thinking-display", "summarized")
	})
	t.Run("enabled with omitted display", func(t *testing.T) {
		transport := &SubprocessTransport{cliPath: "claude", options: &Options{
			Thinking: ThinkingConfigEnabled{BudgetTokens: 2048, Display: ThinkingDisplayOmitted},
		}}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--max-thinking-tokens", "2048")
		assertContains(t, cmd, "--thinking-display", "omitted")
	})
}

func TestBuildCommand_AgentProgressSummaries(t *testing.T) {
	t.Run("false omits flag", func(t *testing.T) {
		transport := &SubprocessTransport{cliPath: "claude", options: &Options{}}
		cmd := transport.buildCommand()
		assertNotContainsFlag(t, cmd, "--agent-progress-summaries")
	})
	t.Run("true sets flag", func(t *testing.T) {
		transport := &SubprocessTransport{cliPath: "claude", options: &Options{AgentProgressSummaries: true}}
		cmd := transport.buildCommand()
		assertContainsFlag(t, cmd, "--agent-progress-summaries")
	})
}

func TestBuildCommand_IncludeHookEvents(t *testing.T) {
	t.Run("false omits flag", func(t *testing.T) {
		transport := &SubprocessTransport{cliPath: "claude", options: &Options{}}
		cmd := transport.buildCommand()
		assertNotContainsFlag(t, cmd, "--include-hook-events")
	})
	t.Run("true sets flag", func(t *testing.T) {
		transport := &SubprocessTransport{cliPath: "claude", options: &Options{IncludeHookEvents: true}}
		cmd := transport.buildCommand()
		assertContainsFlag(t, cmd, "--include-hook-events")
	})
}

func TestBuildCommand_ManagedSettings(t *testing.T) {
	t.Run("empty omits flag", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{},
		}
		cmd := transport.buildCommand()
		assertNotContainsFlag(t, cmd, "--managed-settings")
	})

	t.Run("sets flag", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{ManagedSettings: `{"policy":true}`},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--managed-settings", `{"policy":true}`)
	})
}

func TestBuildCommand_Title(t *testing.T) {
	t.Run("empty title omits flag", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{},
		}
		cmd := transport.buildCommand()
		assertNotContainsFlag(t, cmd, "--title")
	})

	t.Run("title sets flag", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{Title: "My Session"},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--title", "My Session")
	})
}

func TestBuildCommand_SettingSources(t *testing.T) {
	t.Run("nil setting sources omits flag", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{},
		}
		cmd := transport.buildCommand()
		for _, arg := range cmd {
			if arg == "--setting-sources" {
				t.Error("--setting-sources flag should not be present when SettingSources is nil")
				return
			}
		}
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

func TestBuildCommand_SessionMirrorFlag(t *testing.T) {
	t.Run("absent when SessionStore is nil", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{},
		}
		cmd := transport.buildCommand()
		assertNotContainsFlag(t, cmd, "--session-mirror")
	})

	t.Run("present when SessionStore is set", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{SessionStore: NewInMemorySessionStore()},
		}
		cmd := transport.buildCommand()
		assertContainsFlag(t, cmd, "--session-mirror")
	})
}

func TestNormaliseDisallowedTools(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "server-level spec gets wildcard",
			input: []string{"mcp__myserver"},
			want:  []string{"mcp__myserver__*"},
		},
		{
			name:  "tool-level spec unchanged",
			input: []string{"mcp__myserver__mytool"},
			want:  []string{"mcp__myserver__mytool"},
		},
		{
			name:  "already wildcard unchanged",
			input: []string{"mcp__myserver__*"},
			want:  []string{"mcp__myserver__*"},
		},
		{
			name:  "non-MCP tool unchanged",
			input: []string{"Bash", "Write"},
			want:  []string{"Bash", "Write"},
		},
		{
			name:  "mixed list",
			input: []string{"mcp__server1", "Bash", "mcp__server2__tool"},
			want:  []string{"mcp__server1__*", "Bash", "mcp__server2__tool"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normaliseDisallowedTools(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildCommand_DisallowedTools(t *testing.T) {
	t.Run("server-level spec expanded to wildcard", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				DisallowedTools: []string{"mcp__myserver"},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--disallowedTools", "mcp__myserver__*")
	})

	t.Run("tool-level spec passed through unchanged", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				DisallowedTools: []string{"mcp__myserver__mytool"},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--disallowedTools", "mcp__myserver__mytool")
	})

	t.Run("non-MCP tool passed through unchanged", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				DisallowedTools: []string{"Bash"},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--disallowedTools", "Bash")
	})

	t.Run("mixed list normalised correctly", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{
				DisallowedTools: []string{"mcp__server1", "Bash", "mcp__server2__tool"},
			},
		}
		cmd := transport.buildCommand()
		assertContains(t, cmd, "--disallowedTools", "mcp__server1__*,Bash,mcp__server2__tool")
	})

	t.Run("empty DisallowedTools omits flag", func(t *testing.T) {
		transport := &SubprocessTransport{
			cliPath: "claude",
			options: &Options{},
		}
		cmd := transport.buildCommand()
		assertNotContainsFlag(t, cmd, "--disallowedTools")
	})
}

// TestHandleStderr_PanicInCallbackDoesNotAbortLoop is a regression test for
// the stderr-callback panic-isolation guarantee (port of Python SDK v0.2.82
// PR #932). A panicking Stderr callback must not abort the reading loop —
// all subsequent stderr lines must still be delivered.
func TestHandleStderr_PanicInCallbackDoesNotAbortLoop(t *testing.T) {
	const totalLines = 5
	const panicOnLine = 0 // panic on the very first delivery

	var delivered []string

	stderrFn := func(line string) {
		delivered = append(delivered, line)
		if len(delivered)-1 == panicOnLine {
			panic("deliberate test panic")
		}
	}

	// Call callStderr directly — it is the per-line panic-isolation wrapper
	// that handleStderr invokes for every line. All lines must be delivered
	// even when the callback panics on one of them.
	transport := &SubprocessTransport{
		options: &Options{Stderr: stderrFn},
	}

	for i := 0; i < totalLines; i++ {
		transport.callStderr(strings.Repeat("x", i+1)) // unique, non-empty lines
	}

	if len(delivered) != totalLines {
		t.Errorf("expected %d lines delivered, got %d — panic on line %d aborted the loop",
			totalLines, len(delivered), panicOnLine)
	}
}

// TestReadMessages_SurfacesScannerErrTooLong is a regression test: previously
// a line exceeding maxBufSize caused bufio.Scanner to stop with
// bufio.ErrTooLong, and ReadMessages silently ended the channel with no
// indication anything went wrong. It must now surface a "type": "error"
// message so callers see the failure instead of a silent truncated stream.
func TestReadMessages_SurfacesScannerErrTooLong(t *testing.T) {
	oversized := strings.Repeat("a", 200)
	line := `{"type":"assistant","` + oversized + `":"x"}` + "\n"

	transport := &SubprocessTransport{
		stdout:     io.NopCloser(strings.NewReader(line)),
		maxBufSize: 64, // smaller than the line above, forces ErrTooLong
	}

	ch := transport.ReadMessages(context.Background())

	var sawError bool
	for msg := range ch {
		if msg["type"] == "error" {
			sawError = true
			errStr, _ := msg["error"].(string)
			if errStr == "" {
				t.Error("expected non-empty error message for oversized line")
			}
		}
	}

	if !sawError {
		t.Error("expected a \"type\": \"error\" message when a line exceeds maxBufSize, got none")
	}
}

// TestNewSubprocessTransport_RejectsInvalidPermissionMode verifies that a
// typo'd Options.PermissionMode is rejected before the CLI is even located,
// so a bad value never reaches the subprocess launch path.
func TestNewSubprocessTransport_RejectsInvalidPermissionMode(t *testing.T) {
	_, err := NewSubprocessTransport(&Options{
		CLIPath:        "/usr/local/bin/claude",
		PermissionMode: PermissionMode("acceptEdit"),
	})
	if err == nil {
		t.Fatal("expected error for invalid PermissionMode, got nil")
	}
	if !strings.Contains(err.Error(), "acceptEdit") {
		t.Errorf("error message = %q, want it to contain %q", err.Error(), "acceptEdit")
	}
}

// TestNewSubprocessTransport_AcceptsValidPermissionMode verifies that a
// known PermissionMode value does not trip the new validation.
func TestNewSubprocessTransport_AcceptsValidPermissionMode(t *testing.T) {
	transport, err := NewSubprocessTransport(&Options{
		CLIPath:        "/usr/local/bin/claude",
		PermissionMode: PermissionModeAcceptEdits,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
}

// TestReadMessages_CtxCancelDuringScanUnregistersChild is a regression test
// for issue #520: cancelling the ctx passed to ReadMessages while its
// bufio.Scanner-based read loop was still running caused the goroutine to
// return early from inside the loop, bypassing the cleanup that normally
// runs after the loop ends naturally (unregisterChild + t.runCleanup()). That
// left the *exec.Cmd permanently registered in the package-level
// activeChildren registry, leaking one entry per cancelled ReadMessages call
// that wasn't also followed by an explicit Close().
//
// This spawns a real short-lived subprocess that continuously emits NDJSON
// lines, confirms the scan loop is actively running (by receiving a message),
// cancels the context mid-stream, and asserts the cmd is promptly removed
// from activeChildren once the ReadMessages goroutine exits.
func TestReadMessages_CtxCancelDuringScanUnregistersChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell subprocess; not supported on windows")
	}

	cmd := exec.Command("sh", "-c", `while true; do printf '{"type":"assistant"}\n'; sleep 0.02; done`)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	registerChild(cmd)
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	if !activeChildrenContains(cmd) {
		t.Fatal("expected cmd to be registered in activeChildren before the test begins")
	}

	transport := &SubprocessTransport{
		cmd:        cmd,
		stdout:     stdout,
		maxBufSize: defaultMaxBufferSize,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := transport.ReadMessages(ctx)

	// Wait for at least one message so we know the scan loop is genuinely
	// live (blocked in scanner.Scan()/select) before cancelling, rather than
	// racing an already-finished goroutine.
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before any message was received")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first message from ReadMessages")
	}

	cancel()

	// Drain until the channel closes. close(ch) happens after the
	// ReadMessages goroutine's deferred cleanup (unregisterChild + runCleanup)
	// has already run, so once this returns the registry check below is not
	// racing the cleanup itself.
	drained := make(chan struct{})
	go func() {
		for range ch {
		}
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for ReadMessages goroutine to exit after ctx cancellation")
	}

	if activeChildrenContains(cmd) {
		t.Error("expected cmd to be unregistered from activeChildren after ctx cancellation, but it is still registered (leak)")
	}
}

// --- Windows batch-script / cmd.exe metacharacter refusal (issue #527) ---
//
// isWindowsBatchScript and containsWindowsCmdMetacharacters are deliberately
// pure string logic with no runtime.GOOS dependency, so they are exercised
// directly here and run on every CI host regardless of OS. The OS gate
// itself (rejectWindowsBatchCLIForGOOS / rejectWindowsCmdMetacharactersForGOOS)
// takes the OS name as a parameter rather than reading runtime.GOOS
// internally, specifically so the gating behavior — refuse on "windows",
// no-op on everything else — can also be verified from a non-Windows CI
// host by passing both a real and a fake goos value.

func TestIsWindowsBatchScript(t *testing.T) {
	positive := []string{
		`C:\tools\claude.cmd`,
		`C:\tools\claude.bat`,
		`C:\tools\claude.CMD`,        // case-insensitive
		`C:\tools\claude.cmd.`,       // trailing dot
		`C:\tools\claude.CMD `,       // trailing space
		`C:\tools\claude.cmd:stream`, // NTFS alternate data stream on the base file
		`C:\tools\claude:evil.cmd`,   // NTFS alternate data stream naming a .cmd
		`C:\tools\.cmd`,              // bare extension
		`:claude.cmd`,                // leading colon
		`C:claude.cmd`,               // drive-relative path
		`C:\tools\claude.cmd\.`,      // trailing "." component after the batch file
		`C:\tools\claude.cmd\..`,     // trailing ".." component after the batch file
		`claude.cmd`,
		`claude.bat`,
		`/mnt/c/tools/claude.cmd`, // forward slashes
		`C:\\tools\\\\claude.cmd`, // repeated separators
		`relative\path\claude.cmd`,
	}
	for _, p := range positive {
		if !isWindowsBatchScript(p) {
			t.Errorf("isWindowsBatchScript(%q) = false, want true", p)
		}
	}

	negative := []string{
		`C:\tools\claude.exe`,
		`C:\tools\claude`,
		`/usr/local/bin/claude`,
		`claude`,
		`claude.exe`,
		`C:\tools\claude.com`,
		``,
		`C:\tools\notcmd`,
		`C:\tools\claude.cmdx`, // suffix, not exact extension
	}
	for _, p := range negative {
		if isWindowsBatchScript(p) {
			t.Errorf("isWindowsBatchScript(%q) = true, want false", p)
		}
	}
}

func TestContainsWindowsCmdMetacharacters(t *testing.T) {
	positive := []string{
		"x&calc",
		"x|whoami",
		"x<in",
		"x>out",
		"x^y",
		"x%PATH%y",
		"x!VAR!y",
		`x"y`,
		"x\ny",
		"x\ry",
		"x\r\ny",
	}
	for _, v := range positive {
		if !containsWindowsCmdMetacharacters(v) {
			t.Errorf("containsWindowsCmdMetacharacters(%q) = false, want true", v)
		}
	}

	negative := []string{
		"",
		"my-session-id",
		"My project - daily notes (v2) #3",
		"abc123-def456",
		"a normal resume title",
	}
	for _, v := range negative {
		if containsWindowsCmdMetacharacters(v) {
			t.Errorf("containsWindowsCmdMetacharacters(%q) = true, want false", v)
		}
	}
}

func TestRejectWindowsBatchCLIForGOOS(t *testing.T) {
	t.Run("refuses batch script on windows", func(t *testing.T) {
		err := rejectWindowsBatchCLIForGOOS("windows", `C:\tools\claude.cmd`)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := err.(*ConnectionError); !ok {
			t.Fatalf("expected *ConnectionError, got %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), "claude.cmd") {
			t.Errorf("error message = %q, want it to mention the offending path", err.Error())
		}
		if !strings.Contains(err.Error(), "CVE-2024-27980") {
			t.Errorf("error message = %q, want it to reference CVE-2024-27980", err.Error())
		}
	})

	t.Run("allows native exe on windows", func(t *testing.T) {
		if err := rejectWindowsBatchCLIForGOOS("windows", `C:\tools\claude.exe`); err != nil {
			t.Fatalf("unexpected error for native exe: %v", err)
		}
	})

	t.Run("no-op off windows even for a .cmd path", func(t *testing.T) {
		for _, goos := range []string{"linux", "darwin", "freebsd", ""} {
			if err := rejectWindowsBatchCLIForGOOS(goos, `/tmp/claude.cmd`); err != nil {
				t.Errorf("goos=%q: unexpected error: %v", goos, err)
			}
		}
	})

	t.Run("matches the host's actual runtime.GOOS", func(t *testing.T) {
		// This repo's CI runs on Linux, so this should always be a no-op here;
		// it exists to document that production code calls the *ForGOOS
		// helpers with runtime.GOOS, not a hardcoded value.
		if err := rejectWindowsBatchCLIForGOOS(runtime.GOOS, `C:\tools\claude.cmd`); runtime.GOOS != "windows" && err != nil {
			t.Errorf("unexpected error on non-windows host: %v", err)
		}
	})
}

func TestRejectWindowsCmdMetacharactersForGOOS(t *testing.T) {
	t.Run("refuses metacharacters on windows", func(t *testing.T) {
		err := rejectWindowsCmdMetacharactersForGOOS("windows", "Resume", "x&calc")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := err.(*ConnectionError); !ok {
			t.Fatalf("expected *ConnectionError, got %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), "Resume") {
			t.Errorf("error message = %q, want it to mention the option name", err.Error())
		}
	})

	t.Run("allows plain values on windows", func(t *testing.T) {
		if err := rejectWindowsCmdMetacharactersForGOOS("windows", "SessionID", "abc-123-def"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("no-op off windows even with metacharacters", func(t *testing.T) {
		for _, goos := range []string{"linux", "darwin", "freebsd", ""} {
			if err := rejectWindowsCmdMetacharactersForGOOS(goos, "Resume", "x&calc|y<z>w^v%u!\"\r\n"); err != nil {
				t.Errorf("goos=%q: unexpected error: %v", goos, err)
			}
		}
	})

	t.Run("empty value is always fine", func(t *testing.T) {
		if err := rejectWindowsCmdMetacharactersForGOOS("windows", "Resume", ""); err != nil {
			t.Errorf("unexpected error for empty value: %v", err)
		}
	})
}

// TestConnect_NonWindowsHost_NoBatchOrMetacharacterRefusal is a light
// integration check that on this CI host's actual runtime.GOOS (non-Windows),
// Connect does not short-circuit with the Windows-only refusals even when
// given inputs that would trip them on Windows. It still fails to connect
// (there is no real CLI at the given path), but the error must not be the
// batch-script/metacharacter ConnectionError — proving those checks are
// inert here, matching "no behavior change on Linux or macOS" from #527.
func TestConnect_NonWindowsHost_NoBatchOrMetacharacterRefusal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this test documents non-Windows behavior")
	}

	transport := &SubprocessTransport{
		cliPath: "/nonexistent/path/claude.cmd",
		options: &Options{
			Resume: "x&calc",
		},
		maxBufSize: defaultMaxBufferSize,
	}

	err := transport.Connect(context.Background())
	if err == nil {
		t.Fatal("expected an error connecting to a nonexistent CLI path, got nil")
	}
	if strings.Contains(err.Error(), "CVE-2024-27980") {
		t.Errorf("Connect refused as a batch script on a non-Windows host: %v", err)
	}
	if strings.Contains(err.Error(), "unsafe to pass on a Windows command line") {
		t.Errorf("Connect refused cmd.exe metacharacters on a non-Windows host: %v", err)
	}
}
