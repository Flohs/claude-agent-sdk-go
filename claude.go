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

		// Create query handler
		q := newQuery(queryConfig{
			transport:  transport,
			canUseTool: configuredOpts.CanUseTool,
			hooks:      configuredOpts.Hooks,
			mcpServers: sdkServers,
			agents:     configuredOpts.Agents,
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
			"message":            map[string]any{"role": "user", "content": prompt},
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

		_ = q.close()
	}()

	return messages, errs
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
