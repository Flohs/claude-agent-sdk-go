package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// Client provides bidirectional, interactive conversations with Claude Code.
//
// Use [NewClient] to create a client, then [Client.Connect] to start.
// For simple one-shot queries, use [Query] instead.
type Client struct {
	options   *Options
	transport Transport
	q         *query
}

// NewClient creates a new Claude SDK client.
func NewClient(opts *Options) *Client {
	if opts == nil {
		opts = &Options{}
	}
	return &Client{
		options: opts,
	}
}

// Connect establishes the connection to Claude Code.
// An optional initial prompt can be provided.
func (c *Client) Connect(ctx context.Context, prompt ...string) error {
	if os.Getenv("CLAUDE_CODE_ENTRYPOINT") == "" {
		_ = os.Setenv("CLAUDE_CODE_ENTRYPOINT", "sdk-go-client")
	}

	// Configure permission settings
	configuredOpts := *c.options
	if c.options.CanUseTool != nil {
		if c.options.PermissionPromptToolName != "" {
			return &SDKError{Message: "CanUseTool callback cannot be used with PermissionPromptToolName"}
		}
		configuredOpts.PermissionPromptToolName = "stdio"
	}

	// Create transport
	transport, err := NewSubprocessTransport(&configuredOpts)
	if err != nil {
		return err
	}

	if err := transport.Connect(ctx); err != nil {
		return err
	}
	c.transport = transport

	// Extract SDK MCP servers
	sdkServers := extractSdkMcpServers(configuredOpts.McpServers)

	// Extract excludeDynamicSections from PresetPrompt if set
	var excludeDynamic bool
	if pp, ok := configuredOpts.SystemPrompt.(PresetPrompt); ok {
		excludeDynamic = pp.ExcludeDynamicSections
	}

	// Create query handler
	c.q = newQuery(queryConfig{
		transport:              transport,
		canUseTool:             configuredOpts.CanUseTool,
		hooks:                  configuredOpts.Hooks,
		mcpServers:             sdkServers,
		agents:                 configuredOpts.Agents,
		excludeDynamicSections: excludeDynamic,
	})

	c.q.start()

	// Initialize
	if _, err := c.q.initialize(); err != nil {
		_ = c.Close()
		return err
	}

	// Send initial prompt if provided
	if len(prompt) > 0 && prompt[0] != "" {
		return c.SendQuery(ctx, prompt[0])
	}

	return nil
}

// SendQuery sends a new text message to Claude.
func (c *Client) SendQuery(ctx context.Context, prompt string) error {
	return c.SendQueryWithContent(ctx, prompt)
}

// SendQueryWithContent sends a message to Claude with arbitrary content.
// The content parameter can be a plain string for text-only messages,
// or a slice of content blocks for multimodal messages (text, images, documents).
//
// Use the helper constructors to build content blocks:
//
//	content := []any{
//	    claude.NewTextContent("Describe this image"),
//	    claude.NewBase64Content("image/png", base64ImageData),
//	}
//	client.SendQueryWithContent(ctx, content)
//
// For documents (PDFs, plain text, HTML, CSV):
//
//	content := []any{
//	    claude.NewTextContent("Summarize this document"),
//	    claude.NewBase64Content("application/pdf", base64PDFData),
//	}
func (c *Client) SendQueryWithContent(ctx context.Context, content any) error {
	if c.q == nil || c.transport == nil {
		return &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}

	if !c.transport.IsReady() {
		return &ConnectionError{SDKError: SDKError{Message: "Transport is not ready. The subprocess may have exited."}}
	}

	switch content.(type) {
	case string:
		// ok
	case []any:
		// ok
	default:
		return &SDKError{Message: fmt.Sprintf("content must be a string or []any, got %T", content)}
	}

	message := map[string]any{
		"type":               "user",
		"message":            map[string]any{"role": "user", "content": content},
		"parent_tool_use_id": nil,
		"session_id":         "default",
	}

	data, _ := json.Marshal(message)
	return c.transport.Write(string(data) + "\n")
}

// ReceiveMessages returns a channel of all messages from Claude.
func (c *Client) ReceiveMessages(ctx context.Context) <-chan Message {
	out := make(chan Message, 100)

	if c.q == nil {
		close(out)
		return out
	}

	msgCh := c.q.receiveMessages()

	go func() {
		defer close(out)
		for {
			select {
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				parsed, err := ParseMessage(msg)
				if err != nil || parsed == nil {
					continue
				}
				select {
				case out <- parsed:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// ReceiveResponse returns a channel that yields messages until (and including) a ResultMessage.
func (c *Client) ReceiveResponse(ctx context.Context) <-chan Message {
	out := make(chan Message, 100)

	if c.q == nil {
		close(out)
		return out
	}

	msgCh := c.q.receiveMessages()

	go func() {
		defer close(out)
		for {
			select {
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				parsed, err := ParseMessage(msg)
				if err != nil || parsed == nil {
					continue
				}
				select {
				case out <- parsed:
				case <-ctx.Done():
					return
				}
				if _, ok := parsed.(*ResultMessage); ok {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// Interrupt sends an interrupt signal to the current operation.
// The ctx parameter is respected: if the context is cancelled or its deadline
// expires, the interrupt request is abandoned.
func (c *Client) Interrupt(ctx context.Context) error {
	if c.q == nil {
		return &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.interrupt(ctx)
}

// SetPermissionMode changes the permission mode during a conversation.
func (c *Client) SetPermissionMode(ctx context.Context, mode string) error {
	if c.q == nil {
		return &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.setPermissionMode(mode)
}

// SetModel changes the AI model during a conversation.
func (c *Client) SetModel(ctx context.Context, model string) error {
	if c.q == nil {
		return &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.setModel(model)
}

// GetMcpStatus returns the current MCP server connection status.
func (c *Client) GetMcpStatus(ctx context.Context) (*McpStatusResponse, error) {
	if c.q == nil {
		return nil, &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.getMcpStatus()
}

// GetContextUsage returns the current context window usage breakdown.
func (c *Client) GetContextUsage(ctx context.Context) (*ContextUsage, error) {
	if c.q == nil {
		return nil, &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.getContextUsage()
}

// ReloadPlugins reloads plugins and returns refreshed commands, agents, and
// MCP server status.
func (c *Client) ReloadPlugins(ctx context.Context) (map[string]any, error) {
	if c.q == nil {
		return nil, &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.reloadPlugins()
}

// EnableMcpChannel activates a capability channel on an MCP server. Available
// channels for a given server are advertised in McpServerStatus.Capabilities.
func (c *Client) EnableMcpChannel(ctx context.Context, serverName, channel string) error {
	if c.q == nil {
		return &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.enableMcpChannel(serverName, channel)
}

// SupportedAgents returns the list of subagent names available in the session.
func (c *Client) SupportedAgents(ctx context.Context) ([]string, error) {
	if c.q == nil {
		return nil, &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.supportedAgents()
}

// SupportedCommands returns the list of slash command names available in the
// session (including plugin-provided commands).
func (c *Client) SupportedCommands(ctx context.Context) ([]string, error) {
	if c.q == nil {
		return nil, &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.supportedCommands()
}

// PromptSuggestion requests prompt suggestions based on the current
// conversation context and returns them as a list of strings.
func (c *Client) PromptSuggestion(ctx context.Context) ([]string, error) {
	if c.q == nil {
		return nil, &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.promptSuggestion()
}

// ReconnectMcpServer reconnects a disconnected or failed MCP server.
func (c *Client) ReconnectMcpServer(ctx context.Context, name string) error {
	if c.q == nil {
		return &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.reconnectMcpServer(name)
}

// ToggleMcpServer enables or disables an MCP server.
func (c *Client) ToggleMcpServer(ctx context.Context, name string, enabled bool) error {
	if c.q == nil {
		return &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.toggleMcpServer(name, enabled)
}

// StopTask stops a running task.
func (c *Client) StopTask(ctx context.Context, taskID string) error {
	if c.q == nil {
		return &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.stopTask(taskID)
}

// RewindFiles rewinds tracked files to their state at a specific user message.
// Requires EnableFileCheckpointing to be set in Options.
func (c *Client) RewindFiles(ctx context.Context, userMessageID string) error {
	if c.q == nil {
		return &ConnectionError{SDKError: SDKError{Message: "Not connected. Call Connect() first."}}
	}
	return c.q.rewindFiles(userMessageID)
}

// GetServerInfo returns server initialization info.
func (c *Client) GetServerInfo() map[string]any {
	if c.q == nil {
		return nil
	}
	return c.q.initializationResult
}

// Close disconnects from Claude Code and cleans up resources.
func (c *Client) Close() error {
	if c.q != nil {
		err := c.q.close()
		c.q = nil
		c.transport = nil
		return err
	}
	if c.transport != nil {
		err := c.transport.Close()
		c.transport = nil
		return err
	}
	return nil
}
