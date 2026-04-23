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
