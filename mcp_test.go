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
