package claude

import (
	"encoding/json"
	"testing"
)

func TestMcpStdioServerConfig_RequestTimeoutMs(t *testing.T) {
	cfg := McpStdioServerConfig{
		Command:          "my-server",
		RequestTimeoutMs: 5000,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result["requestTimeoutMs"] != float64(5000) {
		t.Errorf("expected requestTimeoutMs 5000, got %v", result["requestTimeoutMs"])
	}
}

func TestMcpServerStatus_Source(t *testing.T) {
	data := []byte(`{"mcpServers":[{"name":"calculator","status":"connected","source":"sdk"},{"name":"github","status":"connected"}]}`)

	var status McpStatusResponse
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(status.McpServers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(status.McpServers))
	}
	if status.McpServers[0].Source != "sdk" {
		t.Errorf("McpServers[0].Source = %q, want %q", status.McpServers[0].Source, "sdk")
	}
	if status.McpServers[1].Source != "" {
		t.Errorf("McpServers[1].Source = %q, want empty when absent", status.McpServers[1].Source)
	}
}

func TestMcpToolInfo_Meta(t *testing.T) {
	data := []byte(`{"mcpServers":[{"name":"widgets","status":"connected","tools":[
		{"name":"render_widget","annotations":{"readOnly":true},"_meta":{"ui":{"resourceUri":"ui://widgets/render","visibility":["assistant"]}}},
		{"name":"legacy_widget","_meta":{"ui/resourceUri":"ui://widgets/legacy"}},
		{"name":"plain_tool"}
	]}]}`)

	var status McpStatusResponse
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(status.McpServers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(status.McpServers))
	}
	tools := status.McpServers[0].Tools
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}

	if tools[0].Meta == nil {
		t.Fatalf("Meta = nil, want populated map")
	}
	ui, ok := tools[0].Meta["ui"].(map[string]any)
	if !ok {
		t.Fatalf("Meta[\"ui\"] = %v, want map", tools[0].Meta["ui"])
	}
	if ui["resourceUri"] != "ui://widgets/render" {
		t.Errorf("Meta[\"ui\"][\"resourceUri\"] = %v, want %q", ui["resourceUri"], "ui://widgets/render")
	}
	visibility, ok := ui["visibility"].([]any)
	if !ok || len(visibility) != 1 || visibility[0] != "assistant" {
		t.Errorf("Meta[\"ui\"][\"visibility\"] = %v, want [\"assistant\"]", ui["visibility"])
	}

	if tools[1].Meta == nil {
		t.Fatalf("Meta = nil for legacy flat variant, want populated map")
	}
	if tools[1].Meta["ui/resourceUri"] != "ui://widgets/legacy" {
		t.Errorf("Meta[\"ui/resourceUri\"] = %v, want %q", tools[1].Meta["ui/resourceUri"], "ui://widgets/legacy")
	}

	if tools[2].Meta != nil {
		t.Errorf("Meta = %v, want nil when absent", tools[2].Meta)
	}
}

func TestMcpStdioServerConfig_RequestTimeoutMs_OmitEmpty(t *testing.T) {
	cfg := McpStdioServerConfig{Command: "my-server"}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := result["requestTimeoutMs"]; ok {
		t.Errorf("expected requestTimeoutMs to be omitted when zero, got %v", result["requestTimeoutMs"])
	}
}

func TestMcpSSEServerConfig_RequestTimeoutMs(t *testing.T) {
	cfg := McpSSEServerConfig{
		URL:              "https://example.com/sse",
		RequestTimeoutMs: 3000,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result["requestTimeoutMs"] != float64(3000) {
		t.Errorf("expected requestTimeoutMs 3000, got %v", result["requestTimeoutMs"])
	}
}

func TestMcpHTTPServerConfig_RequestTimeoutMs(t *testing.T) {
	cfg := McpHTTPServerConfig{
		URL:              "https://example.com/mcp",
		RequestTimeoutMs: 7500,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result["requestTimeoutMs"] != float64(7500) {
		t.Errorf("expected requestTimeoutMs 7500, got %v", result["requestTimeoutMs"])
	}
}

func TestMcpSdkServerConfig_TimeoutMs(t *testing.T) {
	cfg := NewSdkMcpServer("my-sdk-server", "1.0.0", nil)
	cfg.TimeoutMs = 5000

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result["timeout"] != float64(5000) {
		t.Errorf("expected timeout 5000, got %v", result["timeout"])
	}
}

func TestMcpSdkServerConfig_TimeoutMs_OmitEmpty(t *testing.T) {
	cfg := NewSdkMcpServer("my-sdk-server", "1.0.0", nil)

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := result["timeout"]; ok {
		t.Errorf("expected timeout to be omitted when zero, got %v", result["timeout"])
	}
}
