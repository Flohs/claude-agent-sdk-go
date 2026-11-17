package claude

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func mcpConfigFlag(t *testing.T, cmd []string) map[string]any {
	t.Helper()
	for i, arg := range cmd {
		if arg == "--mcp-config" && i+1 < len(cmd) {
			var cfg map[string]any
			if err := json.Unmarshal([]byte(cmd[i+1]), &cfg); err != nil {
				t.Fatalf("--mcp-config value is not JSON: %v", err)
			}
			servers, _ := cfg["mcpServers"].(map[string]any)
			return servers
		}
	}
	return nil
}

func deliveryTestServers() map[string]McpServerConfig {
	return map[string]McpServerConfig{
		"stdio": McpStdioServerConfig{Command: "srv", Env: map[string]string{"TOKEN": "secret"}},
		"http":  McpHTTPServerConfig{URL: "https://example.com/mcp", Headers: map[string]string{"Authorization": "Bearer secret"}},
		"sdk":   &McpSdkServerConfig{Name: "sdk"},
	}
}

func TestBuildCommand_McpServersDeliveryInitialize(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "/usr/local/bin/claude",
		options: &Options{McpServers: deliveryTestServers(), McpServersDelivery: McpServersDeliveryInitialize},
	}
	cmd := transport.buildCommand()

	assertContainsFlag(t, cmd, "--await-initialize")
	servers := mcpConfigFlag(t, cmd)
	if len(servers) != 1 || servers["sdk"] == nil {
		t.Fatalf("--mcp-config servers = %v, want only the SDK server", servers)
	}
	if joined := strings.Join(cmd, " "); strings.Contains(joined, "secret") {
		t.Errorf("command line carries server credentials: %v", cmd)
	}
}

func TestBuildCommand_McpServersDeliveryInitializeNoSdkServers(t *testing.T) {
	servers := deliveryTestServers()
	delete(servers, "sdk")
	transport := &SubprocessTransport{
		cliPath: "/usr/local/bin/claude",
		options: &Options{McpServers: servers, McpServersDelivery: McpServersDeliveryInitialize},
	}
	cmd := transport.buildCommand()
	assertContainsFlag(t, cmd, "--await-initialize")
	assertNotContainsFlag(t, cmd, "--mcp-config")
}

func TestBuildCommand_McpServersDeliveryArgv(t *testing.T) {
	for _, delivery := range []McpServersDelivery{"", McpServersDeliveryArgv} {
		transport := &SubprocessTransport{
			cliPath: "/usr/local/bin/claude",
			options: &Options{McpServers: deliveryTestServers(), McpServersDelivery: delivery},
		}
		cmd := transport.buildCommand()
		assertNotContainsFlag(t, cmd, "--await-initialize")
		if servers := mcpConfigFlag(t, cmd); len(servers) != 3 {
			t.Errorf("delivery %q: --mcp-config servers = %v, want all 3", delivery, servers)
		}
	}
}

func TestBuildCommand_McpServersDeliveryInitializeOnlySdkServers(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "/usr/local/bin/claude",
		options: &Options{
			McpServers:         map[string]McpServerConfig{"sdk": &McpSdkServerConfig{Name: "sdk"}},
			McpServersDelivery: McpServersDeliveryInitialize,
		},
	}
	cmd := transport.buildCommand()
	assertNotContainsFlag(t, cmd, "--await-initialize")
	if servers := mcpConfigFlag(t, cmd); len(servers) != 1 {
		t.Errorf("--mcp-config servers = %v, want the SDK server", servers)
	}
}

func TestBuildCommand_McpServersDeliveryInitializeWithSdkURL(t *testing.T) {
	for _, key := range []string{"sdk-url", "sdk-url=wss://example.com"} {
		transport := &SubprocessTransport{
			cliPath: "/usr/local/bin/claude",
			options: &Options{
				McpServers:         deliveryTestServers(),
				McpServersDelivery: McpServersDeliveryInitialize,
				ExtraArgs:          map[string]string{key: "wss://example.com"},
			},
		}
		cmd := transport.buildCommand()
		assertNotContainsFlag(t, cmd, "--await-initialize")
		if servers := mcpConfigFlag(t, cmd); len(servers) != 3 {
			t.Errorf("ExtraArgs %q: --mcp-config servers = %v, want all 3", key, servers)
		}
		if got := mcpServersViaInit(transport.options); got != nil {
			t.Errorf("ExtraArgs %q: mcpServersViaInit = %v, want nil", key, got)
		}
	}
}

func TestBuildCommand_McpServersDeliveryInitializeStringConfig(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "/usr/local/bin/claude",
		options: &Options{McpServers: "/path/to/mcp.json", McpServersDelivery: McpServersDeliveryInitialize},
	}
	cmd := transport.buildCommand()
	assertContains(t, cmd, "--mcp-config", "/path/to/mcp.json")
	assertNotContainsFlag(t, cmd, "--await-initialize")
}

func TestBuildCommand_McpServersAndPluginsViaInitialize(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "/usr/local/bin/claude",
		options: &Options{
			McpServers:         deliveryTestServers(),
			McpServersDelivery: McpServersDeliveryInitialize,
			Plugins:            []SdkPluginConfig{{Type: "local", Path: "/plugins/one"}},
			PluginDelivery:     "initialize",
		},
	}
	cmd := transport.buildCommand()
	n := 0
	for _, arg := range cmd {
		if arg == "--await-initialize" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("--await-initialize appears %d times, want 1: %v", n, cmd)
	}
}

func TestBuildCommand_McpServersViaInitializeKeepsArgvPlugins(t *testing.T) {
	transport := &SubprocessTransport{
		cliPath: "/usr/local/bin/claude",
		options: &Options{
			McpServers:         deliveryTestServers(),
			McpServersDelivery: McpServersDeliveryInitialize,
			Plugins:            []SdkPluginConfig{{Type: "local", Path: "/plugins/one"}},
		},
	}
	cmd := transport.buildCommand()
	assertContainsFlag(t, cmd, "--await-initialize")
	assertContains(t, cmd, "--plugin-dir", "/plugins/one")
}

// runMcpDeliveryInitialize runs one initialize sending viaInit, against a
// transport whose response payload carries extra.
func runMcpDeliveryInitialize(t *testing.T, viaInit, extra map[string]any) (map[string]any, error) {
	t.Helper()
	p := &pendingRespondTransport{
		autoRespondTransport: autoRespondTransport{mockTransport: mockTransport{messages: make(chan map[string]any, 100)}},
		inner:                true,
		extra:                extra,
	}
	q := newQuery(queryConfig{transport: p, mcpServersViaInit: viaInit})
	q.start()
	_, err := q.initialize()

	p.mu.Lock()
	written := append([]string(nil), p.written...)
	p.mu.Unlock()
	_ = q.close()

	for _, w := range written {
		var msg map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(w)), &msg) != nil {
			continue
		}
		if req, _ := msg["request"].(map[string]any); req != nil && req["subtype"] == "initialize" {
			return req, err
		}
	}
	t.Fatal("no initialize request written")
	return nil, nil
}

func TestInitialize_McpServersDelivery(t *testing.T) {
	opts := &Options{McpServers: deliveryTestServers(), McpServersDelivery: McpServersDeliveryInitialize}
	viaInit := mcpServersViaInit(opts)
	if len(viaInit) != 2 || viaInit["sdk"] != nil {
		t.Fatalf("mcpServersViaInit = %v, want stdio and http only", viaInit)
	}

	t.Run("sends servers and accepts applied", func(t *testing.T) {
		req, err := runMcpDeliveryInitialize(t, viaInit, map[string]any{"mcp_servers_applied": true})
		if err != nil {
			t.Fatalf("initialize: %v", err)
		}
		servers, _ := req["mcpServers"].(map[string]any)
		stdio, _ := servers["stdio"].(map[string]any)
		env, _ := stdio["env"].(map[string]any)
		if len(servers) != 2 || env["TOKEN"] != "secret" {
			t.Errorf("mcpServers = %v, want stdio (with env) and http", req["mcpServers"])
		}
	})

	t.Run("not applied only warns", func(t *testing.T) {
		if _, err := runMcpDeliveryInitialize(t, viaInit, map[string]any{"mcp_servers_applied": false}); err != nil {
			t.Fatalf("initialize: %v", err)
		}
	})

	t.Run("CLI too old fails", func(t *testing.T) {
		_, err := runMcpDeliveryInitialize(t, viaInit, map[string]any{"claude_code_version": "2.1.200"})
		var connErr *ConnectionError
		if !errors.As(err, &connErr) {
			t.Fatalf("err = %v, want *ConnectionError", err)
		}
		if !strings.Contains(err.Error(), "too old") || !strings.Contains(err.Error(), "2.1.200") || !strings.Contains(err.Error(), "McpServersDelivery") {
			t.Errorf("err = %q", err)
		}
	})

	t.Run("nothing sent, nothing checked", func(t *testing.T) {
		req, err := runMcpDeliveryInitialize(t, nil, nil)
		if err != nil {
			t.Fatalf("initialize: %v", err)
		}
		if _, ok := req["mcpServers"]; ok {
			t.Errorf("mcpServers = %v, want absent", req["mcpServers"])
		}
	})
}
