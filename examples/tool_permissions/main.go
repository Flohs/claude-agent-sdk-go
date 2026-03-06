// Tool permission callback examples demonstrating CanUseTool.
//
// Usage:
//
//	go run ./examples/tool_permissions
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

func displayMessage(msg claude.Message) {
	switch m := msg.(type) {
	case *claude.AssistantMessage:
		for _, block := range m.Content {
			if tb, ok := block.(claude.TextBlock); ok {
				fmt.Println("Claude:", tb.Text)
			}
		}
	case *claude.ResultMessage:
		fmt.Println("Result ended")
	}
}

func basicPermissions(ctx context.Context) {
	fmt.Println("=== Basic Permission Callback ===")
	fmt.Println("Allows Read/Glob, blocks dangerous Bash commands, allows everything else.")

	client := claude.NewClient(&claude.Options{
		AllowedTools:   []string{"Read", "Write", "Bash", "Glob"},
		PermissionMode: claude.PermissionModeDefault,
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx claude.ToolPermissionContext) (claude.PermissionResult, error) {
			fmt.Printf("  [Permission] Tool: %s\n", toolName)

			// Always allow read-only tools
			if toolName == "Read" || toolName == "Glob" {
				fmt.Println("  [Permission] -> Allow (read-only tool)")
				return claude.PermissionResultAllow{}, nil
			}

			// Block dangerous bash commands
			if toolName == "Bash" {
				cmd, _ := input["command"].(string)
				for _, pattern := range []string{"rm -rf", "sudo", "> /dev/"} {
					if strings.Contains(cmd, pattern) {
						fmt.Printf("  [Permission] -> Deny (dangerous: %s)\n", pattern)
						return claude.PermissionResultDeny{
							Message: fmt.Sprintf("Command contains dangerous pattern: %s", pattern),
						}, nil
					}
				}
				fmt.Println("  [Permission] -> Allow (safe command)")
				return claude.PermissionResultAllow{}, nil
			}

			// Allow everything else
			fmt.Println("  [Permission] -> Allow (default)")
			return claude.PermissionResultAllow{}, nil
		},
	})

	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	// Test: safe command
	fmt.Println("User: Run echo 'hello world'")
	if err := client.SendQuery(ctx, "Run the bash command: echo 'hello world'"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	fmt.Println()

	// Test: dangerous command
	fmt.Println("User: Run rm -rf /tmp/test")
	if err := client.SendQuery(ctx, "Run the bash command: rm -rf /tmp/test"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}
	fmt.Println()
}

func main() {
	ctx := context.Background()
	basicPermissions(ctx)
}
