package claude

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestGetServerCapabilities_PopulatesCapabilities verifies that the open-set
// "capabilities" field from the CLI's initialization result is surfaced on
// ServerCapabilities.Capabilities. Port of TypeScript SDK v0.3.205.
func TestGetServerCapabilities_PopulatesCapabilities(t *testing.T) {
	c := &Client{
		q: &query{
			initializationResult: map[string]any{
				"supportsEffort": true,
				"capabilities":   []any{"interrupt_receipt_v1", "some_future_capability"},
			},
		},
	}

	caps := c.GetServerCapabilities()
	if caps == nil {
		t.Fatal("expected non-nil ServerCapabilities")
	}
	want := []string{"interrupt_receipt_v1", "some_future_capability"}
	if len(caps.Capabilities) != len(want) {
		t.Fatalf("Capabilities = %v, want %v", caps.Capabilities, want)
	}
	for i, s := range want {
		if caps.Capabilities[i] != s {
			t.Fatalf("Capabilities[%d] = %q, want %q", i, caps.Capabilities[i], s)
		}
	}
}

// TestGetServerCapabilities_OlderCLIOmitsCapabilities verifies that older
// CLIs without a "capabilities" key yield a nil Capabilities slice rather
// than an error.
func TestGetServerCapabilities_OlderCLIOmitsCapabilities(t *testing.T) {
	c := &Client{
		q: &query{
			initializationResult: map[string]any{
				"supportsEffort": true,
			},
		},
	}

	caps := c.GetServerCapabilities()
	if caps == nil {
		t.Fatal("expected non-nil ServerCapabilities")
	}
	if caps.Capabilities != nil {
		t.Fatalf("Capabilities = %v, want nil", caps.Capabilities)
	}
}

// TestGetServerCapabilities_PopulatesFastModeState verifies that
// fast_mode_state and fast_mode_disabled_reason from the CLI's
// initialization result are surfaced on ServerCapabilities. Port of
// TypeScript SDK v0.3.219.
func TestGetServerCapabilities_PopulatesFastModeState(t *testing.T) {
	c := &Client{
		q: &query{
			initializationResult: map[string]any{
				"supportsFastMode":          true,
				"fast_mode_state":           "cooldown",
				"fast_mode_disabled_reason": "extra_usage_disabled",
			},
		},
	}

	caps := c.GetServerCapabilities()
	if caps == nil {
		t.Fatal("expected non-nil ServerCapabilities")
	}
	if caps.FastModeState != FastModeStateCooldown {
		t.Errorf("FastModeState = %q, want %q", caps.FastModeState, FastModeStateCooldown)
	}
	if caps.FastModeDisabledReason != FastModeDisabledReasonExtraUsageDisabled {
		t.Errorf("FastModeDisabledReason = %q, want %q", caps.FastModeDisabledReason, FastModeDisabledReasonExtraUsageDisabled)
	}
}

// TestSlashCommands_ParsesTypedListFromInitializationResult verifies that
// Client.SlashCommands decodes the initialize handshake's "commands" field
// into typed SlashCommand values — including a builtin entry, and one that
// omits every optional field (argumentHint, aliases, builtin) to confirm
// those don't break parsing. It never issues a control request: the CLI's
// commands list is sourced from the cached initialize response, mirroring
// the TypeScript SDK's supportedCommands(). Port of TypeScript SDK v0.3.277
// (SlashCommand.Builtin). ([#725])
func TestSlashCommands_ParsesTypedListFromInitializationResult(t *testing.T) {
	c := &Client{
		q: &query{
			initializationResult: map[string]any{
				"commands": []any{
					map[string]any{
						"name":         "usage",
						"description":  "Show plan usage",
						"argumentHint": "<period>",
						"aliases":      []any{"cost", "stats"},
						"builtin":      true,
					},
					map[string]any{
						"name":        "my-skill",
						"description": "A user-defined skill",
					},
				},
			},
		},
	}

	commands, err := c.SlashCommands(context.Background())
	if err != nil {
		t.Fatalf("SlashCommands failed: %v", err)
	}
	if len(commands) != 2 {
		t.Fatalf("len(commands) = %d, want 2: %+v", len(commands), commands)
	}

	usage := commands[0]
	if usage.Name != "usage" || usage.Description != "Show plan usage" {
		t.Errorf("usage = %+v, want name=usage description=%q", usage, "Show plan usage")
	}
	if usage.ArgumentHint != "<period>" {
		t.Errorf("usage.ArgumentHint = %q, want %q", usage.ArgumentHint, "<period>")
	}
	if len(usage.Aliases) != 2 || usage.Aliases[0] != "cost" || usage.Aliases[1] != "stats" {
		t.Errorf("usage.Aliases = %v, want [cost stats]", usage.Aliases)
	}
	if !usage.Builtin {
		t.Error("usage.Builtin = false, want true")
	}

	skill := commands[1]
	if skill.Name != "my-skill" || skill.Description != "A user-defined skill" {
		t.Errorf("skill = %+v, want name=my-skill description=%q", skill, "A user-defined skill")
	}
	if skill.ArgumentHint != "" {
		t.Errorf("skill.ArgumentHint = %q, want empty", skill.ArgumentHint)
	}
	if skill.Aliases != nil {
		t.Errorf("skill.Aliases = %v, want nil", skill.Aliases)
	}
	if skill.Builtin {
		t.Error("skill.Builtin = true, want false")
	}
}

// TestSupportedCommands_ReturnsNamesForBackwardCompat verifies that
// Client.SupportedCommands keeps its pre-existing []string-of-names
// signature and behavior, deriving the names from the same typed data
// Client.SlashCommands returns rather than dropping any command whose CLI
// payload isn't a bare string (the bug this SDK's typed SlashCommand fixes).
// ([#725])
func TestSupportedCommands_ReturnsNamesForBackwardCompat(t *testing.T) {
	c := &Client{
		q: &query{
			initializationResult: map[string]any{
				"commands": []any{
					map[string]any{"name": "usage", "description": "Show plan usage", "builtin": true},
					map[string]any{"name": "my-skill", "description": "A user-defined skill"},
				},
			},
		},
	}

	names, err := c.SupportedCommands(context.Background())
	if err != nil {
		t.Fatalf("SupportedCommands failed: %v", err)
	}
	want := []string{"usage", "my-skill"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("names[%d] = %q, want %q", i, names[i], n)
		}
	}
}

// TestSlashCommands_NotConnectedReturnsConnectionError verifies that
// Client.SlashCommands fails fast with a ConnectionError when called before
// Connect(), mirroring the nil-query guard used by the other control
// methods. ([#725])
func TestSlashCommands_NotConnectedReturnsConnectionError(t *testing.T) {
	c := &Client{}

	_, err := c.SlashCommands(context.Background())
	if err == nil {
		t.Fatal("expected an error when not connected, got nil")
	}
	if _, ok := err.(*ConnectionError); !ok {
		t.Fatalf("expected a *ConnectionError, got %T: %v", err, err)
	}
}

// TestSupportedCommands_NotConnectedReturnsConnectionError verifies that
// Client.SupportedCommands fails fast with a ConnectionError when called
// before Connect(), mirroring the nil-query guard used by the other control
// methods (e.g. ReloadPlugins).
func TestSupportedCommands_NotConnectedReturnsConnectionError(t *testing.T) {
	c := &Client{}

	_, err := c.SupportedCommands(context.Background())
	if err == nil {
		t.Fatal("expected an error when not connected, got nil")
	}
	if _, ok := err.(*ConnectionError); !ok {
		t.Fatalf("expected a *ConnectionError, got %T: %v", err, err)
	}
}

// TestReloadOutputStyles_NotConnectedReturnsConnectionError verifies that
// Client.ReloadOutputStyles fails fast with a ConnectionError when called
// before Connect(), mirroring the nil-query guard used by the other control
// methods (e.g. ReloadPlugins). Port of TypeScript SDK v0.3.261. ([#673])
func TestReloadOutputStyles_NotConnectedReturnsConnectionError(t *testing.T) {
	c := &Client{}

	_, err := c.ReloadOutputStyles(context.Background())
	if err == nil {
		t.Fatal("expected an error when not connected, got nil")
	}
	if _, ok := err.(*ConnectionError); !ok {
		t.Fatalf("expected a *ConnectionError, got %T: %v", err, err)
	}
}

// TestReloadPluginsWithOptions_NotConnectedReturnsConnectionError verifies
// that Client.ReloadPluginsWithOptions fails fast with a ConnectionError
// when called before Connect(), mirroring the nil-query guard used by the
// other control methods. Port of TypeScript SDK v0.3.268.
func TestReloadPluginsWithOptions_NotConnectedReturnsConnectionError(t *testing.T) {
	c := &Client{}

	_, err := c.ReloadPluginsWithOptions(context.Background(), true)
	if err == nil {
		t.Fatal("expected an error when not connected, got nil")
	}
	if _, ok := err.(*ConnectionError); !ok {
		t.Fatalf("expected a *ConnectionError, got %T: %v", err, err)
	}
}

// TestSetPermissionMode_RejectsInvalidModeWithoutSendingRequest verifies
// that Client.SetPermissionMode validates the mode before dispatching a
// set_permission_mode control request, so a typo never reaches the CLI.
func TestSetPermissionMode_RejectsInvalidModeWithoutSendingRequest(t *testing.T) {
	mt := newMockTransport()
	c := &Client{q: newQuery(queryConfig{transport: mt})}

	err := c.SetPermissionMode(context.Background(), "acceptEdit")
	if err == nil {
		t.Fatal("expected error for invalid PermissionMode, got nil")
	}
	if !strings.Contains(err.Error(), "acceptEdit") {
		t.Errorf("error message = %q, want it to contain %q", err.Error(), "acceptEdit")
	}

	mt.mu.Lock()
	written := len(mt.written)
	mt.mu.Unlock()
	if written != 0 {
		t.Errorf("expected no control request to be written, got %d write(s): %v", written, mt.written)
	}
}

// lastWrittenMessage decodes the most recent line mt has recorded as a JSON
// object, for asserting on an outgoing user message's shape.
func lastWrittenMessage(t *testing.T, mt *mockTransport) map[string]any {
	t.Helper()
	mt.mu.Lock()
	defer mt.mu.Unlock()
	if len(mt.written) == 0 {
		t.Fatal("expected at least one write, got none")
	}
	var msg map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(mt.written[len(mt.written)-1])), &msg); err != nil {
		t.Fatalf("failed to unmarshal written message: %v", err)
	}
	return msg
}

// TestSendQueryWithContent_VerbatimPrompts verifies that Options.VerbatimPrompts
// stamps "client_composed": true on the outgoing user message and delivers a
// slash-prefixed string prompt without escapeSlashCommand's leading-space
// workaround. Port of Python SDK v0.2.157 (anthropics/claude-agent-sdk-python#1269).
func TestSendQueryWithContent_VerbatimPrompts(t *testing.T) {
	mt := newMockTransport()
	c := &Client{
		options:   &Options{VerbatimPrompts: true},
		transport: mt,
		q:         newQuery(queryConfig{transport: mt}),
	}

	if err := c.SendQueryWithContent(context.Background(), "/ add tests"); err != nil {
		t.Fatalf("SendQueryWithContent failed: %v", err)
	}

	msg := lastWrittenMessage(t, mt)
	if msg["client_composed"] != true {
		t.Errorf("expected client_composed: true, got %v", msg["client_composed"])
	}
	inner, _ := msg["message"].(map[string]any)
	if inner["content"] != "/ add tests" {
		t.Errorf("expected verbatim content %q (no escapeSlashCommand), got %v", "/ add tests", inner["content"])
	}
}

// TestSendQueryWithContent_VerbatimPromptsDefaultOff verifies that the
// default (VerbatimPrompts unset) leaves the message unstamped and still
// applies the escapeSlashCommand workaround, matching pre-existing behavior.
func TestSendQueryWithContent_VerbatimPromptsDefaultOff(t *testing.T) {
	mt := newMockTransport()
	c := &Client{
		options:   &Options{},
		transport: mt,
		q:         newQuery(queryConfig{transport: mt}),
	}

	if err := c.SendQueryWithContent(context.Background(), "/ add tests"); err != nil {
		t.Fatalf("SendQueryWithContent failed: %v", err)
	}

	msg := lastWrittenMessage(t, mt)
	if _, ok := msg["client_composed"]; ok {
		t.Errorf("expected no client_composed key, got %v", msg)
	}
	inner, _ := msg["message"].(map[string]any)
	if inner["content"] != " / add tests" {
		t.Errorf("expected escapeSlashCommand-escaped content %q, got %v", " / add tests", inner["content"])
	}
}

// TestAppendMessage_VerbatimPrompts verifies that Options.VerbatimPrompts
// also stamps "client_composed": true on a context-injection message sent
// via AppendMessage.
func TestAppendMessage_VerbatimPrompts(t *testing.T) {
	mt := newMockTransport()
	c := &Client{
		options:   &Options{VerbatimPrompts: true},
		transport: mt,
		q:         newQuery(queryConfig{transport: mt}),
	}

	if err := c.AppendMessage(context.Background(), "some tool result text"); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}

	msg := lastWrittenMessage(t, mt)
	if msg["client_composed"] != true {
		t.Errorf("expected client_composed: true, got %v", msg["client_composed"])
	}
	if msg["shouldQuery"] != false {
		t.Errorf("expected shouldQuery: false to be preserved, got %v", msg["shouldQuery"])
	}
}
