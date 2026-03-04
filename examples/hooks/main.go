// Hook examples demonstrating PreToolUse, PostToolUse, and UserPromptSubmit hooks.
//
// Usage:
//
//	go run ./examples/hooks
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

// checkBashCommand blocks commands containing forbidden patterns.
// Demonstrates ParseHookInput for type-safe access to hook fields.
func checkBashCommand(ctx context.Context, input claude.HookInput, toolUseID string, hookCtx claude.HookContext) (claude.HookJSONOutput, error) {
	typed, err := claude.ParseHookInput(input)
	if err != nil {
		return claude.HookJSONOutput{}, err
	}
	preToolUse, ok := typed.(*claude.PreToolUseHookInput)
	if !ok || preToolUse.ToolName != "Bash" {
		return claude.HookJSONOutput{}, nil
	}

	// Log sub-agent context if present (useful when multiple agents run in parallel).
	if preToolUse.AgentID != "" {
		fmt.Printf("  [Hook] Tool call from sub-agent %s (%s)\n", preToolUse.AgentID, preToolUse.AgentType)
	}

	command, _ := preToolUse.ToolInput["command"].(string)

	blockPatterns := []string{"foo.sh", "rm -rf"}
	for _, pattern := range blockPatterns {
		if strings.Contains(command, pattern) {
			fmt.Printf("  [Hook] Blocked command: %s\n", command)
			return claude.HookJSONOutput{
				"hookSpecificOutput": map[string]any{
					"hookEventName":            "PreToolUse",
					"permissionDecision":       "deny",
					"permissionDecisionReason": fmt.Sprintf("Command contains blocked pattern: %s", pattern),
				},
			}, nil
		}
	}

	return claude.HookJSONOutput{}, nil
}

// addCustomInstructions adds context at prompt submission.
func addCustomInstructions(ctx context.Context, input claude.HookInput, toolUseID string, hookCtx claude.HookContext) (claude.HookJSONOutput, error) {
	return claude.HookJSONOutput{
		"hookSpecificOutput": map[string]any{
			"hookEventName":    "SessionStart",
			"additionalContext": "My favorite color is hot pink",
		},
	}, nil
}

// reviewToolOutput provides feedback after tool execution.
func reviewToolOutput(ctx context.Context, input claude.HookInput, toolUseID string, hookCtx claude.HookContext) (claude.HookJSONOutput, error) {
	toolResponse := fmt.Sprintf("%v", input["tool_response"])

	if strings.Contains(strings.ToLower(toolResponse), "error") {
		return claude.HookJSONOutput{
			"systemMessage": "The command produced an error",
			"reason":        "Tool execution failed - consider checking the command syntax",
			"hookSpecificOutput": map[string]any{
				"hookEventName":    "PostToolUse",
				"additionalContext": "The command encountered an error. You may want to try a different approach.",
			},
		}, nil
	}

	return claude.HookJSONOutput{}, nil
}

func preToolUseExample(ctx context.Context) {
	fmt.Println("=== PreToolUse Example ===")
	fmt.Println("Demonstrates how PreToolUse can block some bash commands but not others.")

	client := claude.NewClient(&claude.Options{
		AllowedTools: []string{"Bash"},
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookEventPreToolUse: {
				{Matcher: "Bash", Hooks: []claude.HookCallback{checkBashCommand}},
			},
		},
	})

	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Test 1: Command with forbidden pattern (blocked)
	fmt.Println("Test 1: Trying a command that should be blocked...")
	fmt.Println("User: Run the bash command: ./foo.sh --help")
	if err := client.SendQuery(ctx, "Run the bash command: ./foo.sh --help"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	fmt.Println("\n" + strings.Repeat("=", 50) + "")

	// Test 2: Safe command (allowed)
	fmt.Println("Test 2: Trying a command that should be allowed...")
	fmt.Println("User: Run the bash command: echo 'Hello from hooks example!'")
	if err := client.SendQuery(ctx, "Run the bash command: echo 'Hello from hooks example!'"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}
	fmt.Println()
}

func userPromptSubmitExample(ctx context.Context) {
	fmt.Println("=== UserPromptSubmit Example ===")
	fmt.Println("Shows how a UserPromptSubmit hook can add context.")

	client := claude.NewClient(&claude.Options{
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			"UserPromptSubmit": {
				{Hooks: []claude.HookCallback{addCustomInstructions}},
			},
		},
	})

	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("User: What's my favorite color?")
	if err := client.SendQuery(ctx, "What's my favorite color?"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}
	fmt.Println()
}

func postToolUseExample(ctx context.Context) {
	fmt.Println("=== PostToolUse Example ===")
	fmt.Println("Shows how PostToolUse can provide feedback.")

	client := claude.NewClient(&claude.Options{
		AllowedTools: []string{"Bash"},
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookEventPostToolUse: {
				{Matcher: "Bash", Hooks: []claude.HookCallback{reviewToolOutput}},
			},
		},
	})

	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("User: Run a command that will produce an error: ls /nonexistent_directory")
	if err := client.SendQuery(ctx, "Run this command: ls /nonexistent_directory"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}
	fmt.Println()
}

func main() {
	ctx := context.Background()

	preToolUseExample(ctx)
	fmt.Println(strings.Repeat("-", 50))
	userPromptSubmitExample(ctx)
	fmt.Println(strings.Repeat("-", 50))
	postToolUseExample(ctx)
}
