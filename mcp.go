package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// McpServerConfig is the interface for MCP server configurations.
type McpServerConfig interface {
	mcpServerConfigMarker()
}

// McpStdioServerConfig configures an MCP server using stdio transport.
type McpStdioServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func (McpStdioServerConfig) mcpServerConfigMarker() {}

// MarshalJSON implements custom JSON marshaling for McpStdioServerConfig.
func (c McpStdioServerConfig) MarshalJSON() ([]byte, error) {
	type alias McpStdioServerConfig
	return json.Marshal(struct {
		Type string `json:"type,omitempty"`
		alias
	}{
		Type:  "stdio",
		alias: alias(c),
	})
}

// McpSSEServerConfig configures an MCP server using SSE transport.
type McpSSEServerConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (McpSSEServerConfig) mcpServerConfigMarker() {}

// MarshalJSON implements custom JSON marshaling for McpSSEServerConfig.
func (c McpSSEServerConfig) MarshalJSON() ([]byte, error) {
	type alias McpSSEServerConfig
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{
		Type:  "sse",
		alias: alias(c),
	})
}

// McpHTTPServerConfig configures an MCP server using HTTP transport.
type McpHTTPServerConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (McpHTTPServerConfig) mcpServerConfigMarker() {}

// MarshalJSON implements custom JSON marshaling for McpHTTPServerConfig.
func (c McpHTTPServerConfig) MarshalJSON() ([]byte, error) {
	type alias McpHTTPServerConfig
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{
		Type:  "http",
		alias: alias(c),
	})
}

// McpSdkServerConfig configures an in-process SDK MCP server.
type McpSdkServerConfig struct {
	Name    string
	tools   []SdkMcpTool
	version string
}

func (McpSdkServerConfig) mcpServerConfigMarker() {}

// MarshalJSON implements custom JSON marshaling for McpSdkServerConfig.
// Only serializes type and name (not the in-process instance).
func (c McpSdkServerConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}{
		Type: "sdk",
		Name: c.Name,
	})
}

// McpServerConnectionStatus represents the connection status of an MCP server.
type McpServerConnectionStatus string

const (
	McpServerConnectionStatusConnected McpServerConnectionStatus = "connected"
	McpServerConnectionStatusFailed    McpServerConnectionStatus = "failed"
	McpServerConnectionStatusNeedsAuth McpServerConnectionStatus = "needs-auth"
	McpServerConnectionStatusPending   McpServerConnectionStatus = "pending"
	McpServerConnectionStatusDisabled  McpServerConnectionStatus = "disabled"
)

// McpToolAnnotations contains tool annotations from MCP server status.
type McpToolAnnotations struct {
	ReadOnly    *bool `json:"readOnly,omitempty"`
	Destructive *bool `json:"destructive,omitempty"`
	OpenWorld   *bool `json:"openWorld,omitempty"`
}

// McpToolInfo describes a tool provided by an MCP server.
type McpToolInfo struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Annotations *McpToolAnnotations `json:"annotations,omitempty"`
}

// McpServerInfo contains server info from the MCP initialize handshake.
type McpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// McpServerStatus contains status information for an MCP server connection.
type McpServerStatus struct {
	Name       string                    `json:"name"`
	Status     McpServerConnectionStatus `json:"status"`
	ServerInfo *McpServerInfo            `json:"serverInfo,omitempty"`
	Error      string                    `json:"error,omitempty"`
	Config     map[string]any            `json:"config,omitempty"`
	Scope      string                    `json:"scope,omitempty"`
	Tools      []McpToolInfo             `json:"tools,omitempty"`
}

// McpStatusResponse is the response from GetMcpStatus.
type McpStatusResponse struct {
	McpServers []McpServerStatus `json:"mcpServers"`
}

// SdkMcpToolHandler is the handler function for an SDK MCP tool.
type SdkMcpToolHandler func(ctx context.Context, arguments map[string]any) (map[string]any, error)

// SdkMcpTool defines an SDK MCP tool.
type SdkMcpTool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     SdkMcpToolHandler
}

// NewSdkMcpServer creates an in-process MCP server configuration.
func NewSdkMcpServer(name string, version string, tools []SdkMcpTool) *McpSdkServerConfig {
	if version == "" {
		version = "1.0.0"
	}
	return &McpSdkServerConfig{
		Name:    name,
		tools:   tools,
		version: version,
	}
}

// sdkMcpRouter handles JSONRPC requests for SDK MCP servers.
type sdkMcpRouter struct {
	servers map[string]*McpSdkServerConfig
	mu      sync.RWMutex
}

func newSdkMcpRouter() *sdkMcpRouter {
	return &sdkMcpRouter{
		servers: make(map[string]*McpSdkServerConfig),
	}
}

func (r *sdkMcpRouter) addServer(name string, server *McpSdkServerConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.servers[name] = server
}

func (r *sdkMcpRouter) handleRequest(ctx context.Context, serverName string, message map[string]any) map[string]any {
	r.mu.RLock()
	server, ok := r.servers[serverName]
	r.mu.RUnlock()

	if !ok {
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      message["id"],
			"error": map[string]any{
				"code":    -32601,
				"message": fmt.Sprintf("Server '%s' not found", serverName),
			},
		}
	}

	method, _ := message["method"].(string)
	params, _ := message["params"].(map[string]any)

	switch method {
	case "initialize":
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      message["id"],
			"result": map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    server.Name,
					"version": server.version,
				},
			},
		}

	case "tools/list":
		toolsList := make([]map[string]any, len(server.tools))
		for i, tool := range server.tools {
			toolData := map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": tool.InputSchema,
			}
			toolsList[i] = toolData
		}
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      message["id"],
			"result": map[string]any{
				"tools": toolsList,
			},
		}

	case "tools/call":
		toolName, _ := params["name"].(string)
		arguments, _ := params["arguments"].(map[string]any)

		var matchedTool *SdkMcpTool
		for i := range server.tools {
			if server.tools[i].Name == toolName {
				matchedTool = &server.tools[i]
				break
			}
		}

		if matchedTool == nil {
			return map[string]any{
				"jsonrpc": "2.0",
				"id":      message["id"],
				"error": map[string]any{
					"code":    -32601,
					"message": fmt.Sprintf("Tool '%s' not found", toolName),
				},
			}
		}

		result, err := matchedTool.Handler(ctx, arguments)
		if err != nil {
			return map[string]any{
				"jsonrpc": "2.0",
				"id":      message["id"],
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": err.Error()},
					},
					"isError": true,
				},
			}
		}
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      message["id"],
			"result":  result,
		}

	case "notifications/initialized":
		return map[string]any{
			"jsonrpc": "2.0",
			"result":  map[string]any{},
		}

	default:
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      message["id"],
			"error": map[string]any{
				"code":    -32601,
				"message": fmt.Sprintf("Method '%s' not found", method),
			},
		}
	}
}
