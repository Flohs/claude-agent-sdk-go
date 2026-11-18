package claude

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWarnAgentSdkMcpServers(t *testing.T) {
	calc := NewSdkMcpServer("calc", "1.0.0", nil)
	opts := &Options{
		Agents: map[string]AgentDefinition{
			"b-agent": {
				Description: "b",
				Prompt:      "b",
				MCPServers: []any{
					"github",
					map[string]McpServerConfig{
						"calc":  calc,
						"stdio": McpStdioServerConfig{Command: "node"},
					},
				},
			},
			"a-agent": {
				Description: "a",
				Prompt:      "a",
				MCPServers:  []any{map[string]any{"zeta": calc, "alpha": *calc}},
			},
			"plain": {Description: "p", Prompt: "p", MCPServers: []any{"github"}},
		},
	}
	out := captureStderr(t, func() { warnAgentSdkMcpServers(opts) })
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 warnings, got %d: %q", len(lines), out)
	}
	wantPrefixes := []string{
		`Warning: Agent "a-agent": MCP server "alpha"`,
		`Warning: Agent "a-agent": MCP server "zeta"`,
		`Warning: Agent "b-agent": MCP server "calc"`,
	}
	for i, want := range wantPrefixes {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d = %q, want prefix %q", i, lines[i], want)
		}
	}
	if !strings.Contains(lines[2], "add mcp__calc to it") {
		t.Errorf("warning should name the tool prefix: %q", lines[2])
	}
}

func TestWarnAgentSdkMcpServers_NoneWithoutSdkServers(t *testing.T) {
	opts := &Options{
		McpServers: map[string]McpServerConfig{"calc": NewSdkMcpServer("calc", "", nil)},
		Agents: map[string]AgentDefinition{
			"agent": {Description: "d", Prompt: "p", MCPServers: []any{"calc", map[string]any{"x": map[string]any{"command": "node"}}}},
		},
	}
	if out := captureStderr(t, func() { warnAgentSdkMcpServers(opts) }); out != "" {
		t.Errorf("expected no warning, got %q", out)
	}
	if out := captureStderr(t, func() { warnAgentSdkMcpServers(&Options{}) }); out != "" {
		t.Errorf("expected no warning without agents, got %q", out)
	}
}

// An SDK server in an agent's MCPServers is sent as {type: "sdk", name},
// never with its in-process state, matching TypeScript SDK v0.3.327.
func TestAgentDefinition_SdkMcpServerWireForm(t *testing.T) {
	def := AgentDefinition{
		Description: "d",
		Prompt:      "p",
		MCPServers:  []any{map[string]McpServerConfig{"calc": NewSdkMcpServer("calc", "", nil)}},
	}
	data, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"mcpServers":[{"calc":{"type":"sdk","name":"calc"}}]`) {
		t.Errorf("unexpected wire form: %s", data)
	}
}
