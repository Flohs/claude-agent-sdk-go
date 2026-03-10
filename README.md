# Claude Agent SDK for Go

[![CI](https://github.com/Flohs/claude-agent-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Flohs/claude-agent-sdk-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/Flohs/claude-agent-sdk-go.svg)](https://pkg.go.dev/github.com/Flohs/claude-agent-sdk-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Go SDK for Claude Agent. Communicates with the [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) via subprocess stdio using a JSON-based bidirectional control protocol.

## Installation

```bash
go get github.com/Flohs/claude-agent-sdk-go
```

**Prerequisites:**

- Go 1.23+
- Claude Code CLI (>= 2.0.0) installed: `npm install -g @anthropic-ai/claude-code`
  - Or specify a custom path: `Options{CLIPath: "/path/to/claude"}`
  - The SDK uses the bidirectional JSON streaming protocol (`--output-format stream-json`), which requires CLI version 2.0.0 or later. A version check runs automatically on connect and warns if the CLI is too old.

**Version Compatibility:**

| Feature | Minimum CLI Version |
|---------|-------------------|
| Base functionality (streaming JSON protocol) | >= 2.0.0 |
| Fine-grained tool streaming (`IncludePartialMessages`) | >= 2.1.40 |

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    claude "github.com/Flohs/claude-agent-sdk-go"
)

func main() {
    ctx := context.Background()
    messages, errs := claude.Query(ctx, "What is 2 + 2?", nil)

    for msg := range messages {
        switch m := msg.(type) {
        case *claude.AssistantMessage:
            for _, block := range m.Content {
                if tb, ok := block.(claude.TextBlock); ok {
                    fmt.Println("Claude:", tb.Text)
                }
            }
        case *claude.ResultMessage:
            if m.TotalCostUSD != nil {
                fmt.Printf("Cost: $%.4f\n", *m.TotalCostUSD)
            }
        }
    }

    // Check for errors
    for err := range errs {
        fmt.Println("Error:", err)
    }
}
```

## Basic Usage: Query()

`Query()` sends a one-shot prompt and returns messages via channel. It's ideal for simple, stateless queries where you don't need bidirectional communication.

```go
ctx := context.Background()

// Simple query
messages, errs := claude.Query(ctx, "Hello Claude", nil)
for msg := range messages {
    if m, ok := msg.(*claude.AssistantMessage); ok {
        for _, block := range m.Content {
            if tb, ok := block.(claude.TextBlock); ok {
                fmt.Println(tb.Text)
            }
        }
    }
}

// With options
maxTurns := 1
messages, errs = claude.Query(ctx, "Tell me a joke", &claude.Options{
    SystemPrompt: claude.StringPrompt("You are a helpful assistant"),
    MaxTurns:     &maxTurns,
})
```

### Using Tools

```go
messages, _ := claude.Query(ctx, "Create a hello.py file", &claude.Options{
    AllowedTools:   []string{"Read", "Write", "Bash"},
    PermissionMode: claude.PermissionModeAcceptEdits,
})
```

### Working Directory

```go
messages, _ := claude.Query(ctx, "Describe this project", &claude.Options{
    Cwd: "/path/to/project",
})
```

## Client

`Client` supports bidirectional, interactive conversations with Claude Code.

Unlike `Query()`, `Client` additionally enables **custom tools** (via SDK MCP servers), **hooks**, and runtime control (interrupts, model changes, permission mode changes).

```go
client := claude.NewClient(&claude.Options{
    AllowedTools: []string{"Read", "Write"},
})

if err := client.Connect(ctx); err != nil {
    log.Fatal(err)
}
defer client.Close()

// First turn
client.SendQuery(ctx, "What's the capital of France?")
for msg := range client.ReceiveResponse(ctx) {
    // handle messages...
}

// Follow-up
client.SendQuery(ctx, "What's the population of that city?")
for msg := range client.ReceiveResponse(ctx) {
    // handle messages...
}
```

### Custom Tools (SDK MCP Servers)

Custom tools are in-process MCP servers that run directly within your Go application. No separate process needed.

```go
// Define tools
calculator := claude.NewSdkMcpServer("calculator", "1.0.0", []claude.SdkMcpTool{
    {
        Name:        "add",
        Description: "Add two numbers",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "a": map[string]any{"type": "number"},
                "b": map[string]any{"type": "number"},
            },
            "required": []string{"a", "b"},
        },
        Handler: func(ctx context.Context, args map[string]any) (map[string]any, error) {
            a, _ := args["a"].(float64)
            b, _ := args["b"].(float64)
            return map[string]any{
                "content": []map[string]any{
                    {"type": "text", "text": fmt.Sprintf("%.0f + %.0f = %.0f", a, b, a+b)},
                },
            }, nil
        },
    },
})

// Use with Client
client := claude.NewClient(&claude.Options{
    McpServers: map[string]claude.McpServerConfig{
        "calc": calculator,
    },
    AllowedTools: []string{"mcp__calc__add"},
})
```

### Hooks

Hooks are Go functions that the Claude Code application invokes at specific points in the agent loop. They provide deterministic processing and automated feedback.

```go
checkBashCommand := func(ctx context.Context, input claude.HookInput, toolUseID string, hookCtx claude.HookContext) (claude.HookJSONOutput, error) {
    toolName, _ := input["tool_name"].(string)
    if toolName != "Bash" {
        return claude.HookJSONOutput{}, nil
    }
    toolInput, _ := input["tool_input"].(map[string]any)
    command, _ := toolInput["command"].(string)

    if strings.Contains(command, "rm -rf") {
        return claude.HookJSONOutput{
            "hookSpecificOutput": map[string]any{
                "hookEventName":            "PreToolUse",
                "permissionDecision":       "deny",
                "permissionDecisionReason": "Dangerous command blocked",
            },
        }, nil
    }
    return claude.HookJSONOutput{}, nil
}

client := claude.NewClient(&claude.Options{
    AllowedTools: []string{"Bash"},
    Hooks: map[claude.HookEvent][]claude.HookMatcher{
        claude.HookEventPreToolUse: {
            {Matcher: "Bash", Hooks: []claude.HookCallback{checkBashCommand}},
        },
    },
})
```

### Tool Permission Callbacks

Control which tools Claude can use and modify their inputs:

```go
client := claude.NewClient(&claude.Options{
    PermissionMode: claude.PermissionModeDefault,
    CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx claude.ToolPermissionContext) (claude.PermissionResult, error) {
        // Allow read-only tools
        if toolName == "Read" || toolName == "Glob" {
            return claude.PermissionResultAllow{}, nil
        }
        // Deny dangerous operations
        if toolName == "Bash" {
            cmd, _ := input["command"].(string)
            if strings.Contains(cmd, "rm -rf") {
                return claude.PermissionResultDeny{Message: "Dangerous command"}, nil
            }
        }
        return claude.PermissionResultAllow{}, nil
    },
})
```

### Interrupt and Runtime Control

```go
// Change permission mode mid-conversation
client.SetPermissionMode(ctx, "acceptEdits")

// Change model
client.SetModel(ctx, "claude-sonnet-4-5")

// Interrupt current operation
client.Interrupt(ctx)

// Get MCP server status
status, _ := client.GetMcpStatus(ctx)
for _, server := range status.McpServers {
    fmt.Printf("%s: %s\n", server.Name, server.Status)
}
```

## Types

### Messages

| Type | Description |
|------|-------------|
| `AssistantMessage` | Claude's response with content blocks |
| `UserMessage` | User message (also contains tool results) |
| `SystemMessage` | System messages (init, status, etc.) |
| `ResultMessage` | Final result with cost and usage info |
| `StreamEvent` | Partial message updates (when streaming enabled) |
| `TaskStartedMessage` | Task started notification |
| `TaskProgressMessage` | Task progress update |
| `TaskNotificationMessage` | Task completed/failed/stopped |

### Content Blocks

| Type | Description |
|------|-------------|
| `TextBlock` | Text content |
| `ThinkingBlock` | Extended thinking content |
| `ToolUseBlock` | Tool invocation |
| `ToolResultBlock` | Tool execution result |

### Configuration

| Option | Type | Description |
|--------|------|-------------|
| `SystemPrompt` | `StringPrompt` / `PresetPrompt` | System prompt configuration |
| `PermissionMode` | `PermissionMode` | Tool execution permissions |
| `MaxTurns` | `*int` | Max conversation turns |
| `MaxBudgetUSD` | `*float64` | Max cost limit |
| `Model` | `string` | AI model to use |
| `AllowedTools` | `[]string` | Tools to allow |
| `DisallowedTools` | `[]string` | Tools to disallow |
| `Cwd` | `string` | Working directory |
| `Thinking` | `ThinkingConfig` | Extended thinking config |
| `Effort` | `Effort` | Thinking depth |

See [options.go](options.go) for the full `Options` struct.

## Error Handling

```go
messages, errs := claude.Query(ctx, "Hello", nil)

// Drain messages
for range messages {}

// Check errors
for err := range errs {
    switch e := err.(type) {
    case *claude.NotFoundError:
        fmt.Println("Install Claude Code: npm install -g @anthropic-ai/claude-code")
    case *claude.ProcessError:
        fmt.Printf("Process failed (exit code %d): %s\n", *e.ExitCode, e.Stderr)
    case *claude.ConnectionError:
        fmt.Println("Connection error:", e.Message)
    default:
        fmt.Println("Error:", err)
    }
}
```

## Session Management

List and read historical sessions:

```go
// List sessions for a project
sessions, _ := claude.ListSessions(claude.ListSessionsOptions{
    Directory: "/path/to/project",
})
for _, s := range sessions {
    fmt.Printf("%s: %s (modified %d)\n", s.SessionID, s.Summary, s.LastModified)
}

// Read messages from a session
messages, _ := claude.GetSessionMessages("session-uuid", claude.GetSessionMessagesOptions{
    Directory: "/path/to/project",
})
```

## Examples

See the [examples/](examples/) directory for complete working examples:

| Example | Description |
|---------|-------------|
| [quick_start](examples/quick_start/) | Basic usage, options, and tools |
| [streaming](examples/streaming/) | Multi-turn conversations with `Client` |
| [hooks](examples/hooks/) | PreToolUse, PostToolUse, and other hook patterns |
| [tool_permissions](examples/tool_permissions/) | Tool permission callbacks |
| [mcp_calculator](examples/mcp_calculator/) | In-process MCP server with calculator tools |
| [agents](examples/agents/) | Custom agent definitions |
| [system_prompt](examples/system_prompt/) | System prompt configurations |
| [budget](examples/budget/) | Cost control with MaxBudgetUSD |

## License and Terms

Use of this SDK is governed by Anthropic's [Commercial Terms of Service](https://www.anthropic.com/legal/commercial-terms).
