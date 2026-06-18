// Package claude provides a Go SDK for interacting with Claude Code via the CLI subprocess.
//
// This SDK communicates with the Claude Code CLI via a JSON-based bidirectional
// control protocol over subprocess stdio. It supports both one-shot queries and
// interactive bidirectional conversations.
//
// Quick start with a one-shot query:
//
//	messages, errs := claude.Query(ctx, "What is 2+2?", &claude.Options{
//	    PermissionMode: claude.PermissionModeBypassPermissions,
//	})
//	for msg := range messages {
//	    switch m := msg.(type) {
//	    case *claude.AssistantMessage:
//	        for _, block := range m.Content {
//	            if tb, ok := block.(claude.TextBlock); ok {
//	                fmt.Println(tb.Text)
//	            }
//	        }
//	    case *claude.ResultMessage:
//	        fmt.Printf("Cost: $%.4f\n", *m.TotalCostUSD)
//	    }
//	}
//
// For interactive conversations, use [Client]:
//
//	client := claude.NewClient(&claude.Options{})
//	if err := client.Connect(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	if err := client.SendQuery(ctx, "Hello!"); err != nil {
//	    log.Fatal(err)
//	}
//	for msg := range client.ReceiveResponse(ctx) {
//	    // handle messages...
//	}
package claude

import (
	"context"
	"encoding/json"
	"os"
)

// normalizePromptContent converts a string prompt starting with "/ " (slash
// followed by whitespace) to array content so the CLI does not interpret it
// as a malformed slash command and silently drop it. The original text is
// preserved verbatim in the text content block.
// Port of TypeScript SDK v0.3.172.
func normalizePromptContent(content any) any {
	if str, ok := content.(string); ok && len(str) >= 2 && str[0] == '/' && (str[1] == ' ' || str[1] == '\t') {
		return []any{map[string]any{"type": "text", "text": str}}
	}
	return content
}

// Query sends a one-shot prompt to Claude Code and returns messages via channel.
//
// This is the simplest way to interact with Claude Code. For interactive
// conversations with follow-ups, use [Client] instead.
//
// The returned messages channel will be closed when the conversation ends.
// The errors channel receives at most one error and is then closed.
func Query(ctx context.Context, prompt string, opts *Options) (<-chan Message, <-chan error) {
	messages := make(chan Message, 100)
	errs := make(chan error, 1)

	go func() {
		defer close(messages)
		defer close(errs)

		if opts == nil {
			opts = &Options{}
		}

		if os.Getenv("CLAUDE_CODE_ENTRYPOINT") == "" {
			_ = os.Setenv("CLAUDE_CODE_ENTRYPOINT", "sdk-go")
		}

		// Configure permission settings
		configuredOpts := *opts
		if opts.CanUseTool != nil {
			if opts.PermissionPromptToolName != "" {
				errs <- &SDKError{Message: "CanUseTool callback cannot be used with PermissionPromptToolName"}
				return
			}
			configuredOpts.PermissionPromptToolName = "stdio"
		}

		// Create transport
		transport, err := NewSubprocessTransport(&configuredOpts)
		if err != nil {
			errs <- err
			return
		}

		if err := transport.Connect(ctx); err != nil {
			errs <- err
			return
		}

		// Extract SDK MCP servers
		sdkServers := extractSdkMcpServers(configuredOpts.McpServers)

		// Extract excludeDynamicSections from PresetPrompt if set
		var excludeDynamic bool
		if pp, ok := configuredOpts.SystemPrompt.(PresetPrompt); ok {
			excludeDynamic = pp.ExcludeDynamicSections
		}

		// Create query handler
		q := newQuery(queryConfig{
			transport:              transport,
			canUseTool:             configuredOpts.CanUseTool,
			hooks:                  configuredOpts.Hooks,
			mcpServers:             sdkServers,
			agents:                 configuredOpts.Agents,
			excludeDynamicSections: excludeDynamic,
		})

		q.start()

		// Initialize
		if _, err := q.initialize(); err != nil {
			errs <- err
			_ = q.close()
			return
		}

		// Send the user message
		userMessage := map[string]any{
			"type":               "user",
			"session_id":         "",
			"message":            map[string]any{"role": "user", "content": normalizePromptContent(prompt)},
			"parent_tool_use_id": nil,
		}
		data, _ := json.Marshal(userMessage)
		if err := transport.Write(string(data) + "\n"); err != nil {
			errs <- err
			_ = q.close()
			return
		}
		go q.waitForResultAndEndInput()

		// Receive and parse messages
		for msg := range q.receiveMessages() {
			parsed, err := ParseMessage(msg)
			if err != nil {
				continue // skip unparseable messages
			}
			if parsed != nil {
				select {
				case messages <- parsed:
				case <-ctx.Done():
					_ = q.close()
					return
				}
			}
		}

		// Surface a subprocess error only when no is_error result was already
		// delivered, so callers don't see both the error result and a ProcessError.
		if q.processError != nil {
			errs <- q.processError
		}
		_ = q.close()
	}()

	return messages, errs
}

// WarmQuery holds a pre-warmed CLI subprocess that has completed initialization
// and is ready to accept a user prompt. Create one with [Startup] and use it via
// [WarmQuery.Query] to skip subprocess startup and initialization overhead on the
// first query. If [WarmQuery.Query] is never called, call [WarmQuery.Close] to
// avoid leaking the subprocess.
//
// Port of TypeScript SDK v0.2.89 `startup()` / `WarmQuery`.
type WarmQuery struct {
	transport Transport
	q         *query
}

// Close terminates the pre-warmed subprocess. Call this if [WarmQuery.Query]
// will never be called (e.g. during application shutdown before the first query).
func (w *WarmQuery) Close() {
	_ = w.q.close()
}

// Query sends a prompt to the pre-warmed subprocess and returns message and
// error channels, identical in shape to the top-level [Query] function.
// The subprocess startup and initialization steps are already done, so the
// first response begins ~20× faster than a cold [Query] call.
//
// The WarmQuery must not be reused after Query returns.
func (w *WarmQuery) Query(ctx context.Context, prompt string) (<-chan Message, <-chan error) {
	messages := make(chan Message, 100)
	errs := make(chan error, 1)

	go func() {
		defer close(messages)
		defer close(errs)

		userMessage := map[string]any{
			"type":               "user",
			"session_id":         "",
			"message":            map[string]any{"role": "user", "content": normalizePromptContent(prompt)},
			"parent_tool_use_id": nil,
		}
		data, _ := json.Marshal(userMessage)
		if err := w.transport.Write(string(data) + "\n"); err != nil {
			errs <- err
			_ = w.q.close()
			return
		}
		go w.q.waitForResultAndEndInput()

		for msg := range w.q.receiveMessages() {
			parsed, err := ParseMessage(msg)
			if err != nil {
				continue
			}
			if parsed != nil {
				select {
				case messages <- parsed:
				case <-ctx.Done():
					_ = w.q.close()
					return
				}
			}
		}

		if w.q.processError != nil {
			errs <- w.q.processError
		}
		_ = w.q.close()
	}()

	return messages, errs
}

// Startup pre-warms the CLI subprocess so the first query is significantly
// faster. It starts the subprocess, completes initialization, and returns a
// [WarmQuery] ready to accept a prompt. The subprocess startup cost (typically
// 1–3 seconds) is paid during Startup so the subsequent [WarmQuery.Query] call
// can begin immediately.
//
// Use this when startup cost can be paid upfront — for example, during
// application initialization before the first user interaction:
//
//	warm, err := claude.Startup(ctx, opts)
//	if err != nil { ... }
//	defer warm.Close() // no-op if Query is called
//	// ... wait for user input ...
//	msgs, errs := warm.Query(ctx, userPrompt)
//
// Port of TypeScript SDK v0.2.89.
func Startup(ctx context.Context, opts *Options) (*WarmQuery, error) {
	if opts == nil {
		opts = &Options{}
	}

	if os.Getenv("CLAUDE_CODE_ENTRYPOINT") == "" {
		_ = os.Setenv("CLAUDE_CODE_ENTRYPOINT", "sdk-go")
	}

	configuredOpts := *opts
	if opts.CanUseTool != nil {
		if opts.PermissionPromptToolName != "" {
			return nil, &SDKError{Message: "CanUseTool callback cannot be used with PermissionPromptToolName"}
		}
		configuredOpts.PermissionPromptToolName = "stdio"
	}

	transport, err := NewSubprocessTransport(&configuredOpts)
	if err != nil {
		return nil, err
	}

	if err := transport.Connect(ctx); err != nil {
		return nil, err
	}

	sdkServers := extractSdkMcpServers(configuredOpts.McpServers)

	var excludeDynamic bool
	if pp, ok := configuredOpts.SystemPrompt.(PresetPrompt); ok {
		excludeDynamic = pp.ExcludeDynamicSections
	}

	q := newQuery(queryConfig{
		transport:              transport,
		canUseTool:             configuredOpts.CanUseTool,
		hooks:                  configuredOpts.Hooks,
		mcpServers:             sdkServers,
		agents:                 configuredOpts.Agents,
		excludeDynamicSections: excludeDynamic,
	})
	q.start()

	if _, err := q.initialize(); err != nil {
		_ = q.close()
		return nil, err
	}

	return &WarmQuery{transport: transport, q: q}, nil
}

// extractSdkMcpServers extracts SDK MCP server configs from the McpServers option.
func extractSdkMcpServers(mcpServers any) map[string]*McpSdkServerConfig {
	servers, ok := mcpServers.(map[string]McpServerConfig)
	if !ok {
		return nil
	}

	sdkServers := make(map[string]*McpSdkServerConfig)
	for name, config := range servers {
		if sdk, ok := config.(*McpSdkServerConfig); ok {
			sdkServers[name] = sdk
		}
	}

	if len(sdkServers) == 0 {
		return nil
	}
	return sdkServers
}
