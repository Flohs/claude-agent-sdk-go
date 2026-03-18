// Tools option example demonstrating the difference between Tools and AllowedTools.
//
// Tools controls which tools are available to Claude (tool existence).
// AllowedTools controls which available tools are pre-approved (no permission prompt).
//
// Usage:
//
//	go run ./examples/tools_option
package main

import (
	"context"
	"fmt"
	"log"

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
		if m.TotalCostUSD != nil {
			fmt.Printf("Cost: $%.4f\n", *m.TotalCostUSD)
		}
	}
}

// explicitToolList shows how to restrict Claude to only specific tools.
func explicitToolList(ctx context.Context) {
	fmt.Println("=== Explicit Tool List ===")
	fmt.Println("Only Read, Glob, and Grep are available (no Bash, Write, etc.).")

	maxTurns := 2
	messages, errs := claude.Query(ctx, "List the Go files in this directory and read the first one.", &claude.Options{
		Tools:        []string{"Read", "Glob", "Grep"},
		AllowedTools: []string{"Read", "Glob", "Grep"},
		MaxTurns:     &maxTurns,
	})

	for msg := range messages {
		displayMessage(msg)
	}
	for err := range errs {
		log.Println("Error:", err)
	}
	fmt.Println()
}

// noToolsMode shows how to disable all tools, making Claude a pure text model.
func noToolsMode(ctx context.Context) {
	fmt.Println("=== No Tools Mode ===")
	fmt.Println("All tools disabled. Claude cannot read files or run commands.")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "What is 2+2? Answer in one sentence.", &claude.Options{
		Tools:    []string{},
		MaxTurns: &maxTurns,
	})

	for msg := range messages {
		displayMessage(msg)
	}
	for err := range errs {
		log.Println("Error:", err)
	}
	fmt.Println()
}

// defaultToolPreset shows how to use the default Claude Code tool preset.
func defaultToolPreset(ctx context.Context) {
	fmt.Println("=== Default Tool Preset ===")
	fmt.Println("Using the default Claude Code tool preset (all standard tools).")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "What tools do you have access to? Just list their names briefly.", &claude.Options{
		Tools:    &claude.ToolsPreset{Preset: "claude_code"},
		MaxTurns: &maxTurns,
	})

	for msg := range messages {
		displayMessage(msg)
	}
	for err := range errs {
		log.Println("Error:", err)
	}
	fmt.Println()
}

func main() {
	ctx := context.Background()

	explicitToolList(ctx)
	noToolsMode(ctx)
	defaultToolPreset(ctx)
}
