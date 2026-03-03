// Agent definitions example demonstrating custom agent configurations.
//
// Usage:
//
//	go run ./examples/agents
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

func main() {
	ctx := context.Background()

	fmt.Println("=== Agent Definitions Example ===")
	fmt.Println("Demonstrates defining custom agents with specific tools and prompts.")

	maxTurns := 3
	messages, errs := claude.Query(ctx, "Review the file go.mod and describe what this project is about.", &claude.Options{
		Agents: map[string]claude.AgentDefinition{
			"code-reviewer": {
				Description: "Reviews code for best practices, bugs, and improvements",
				Tools:       []string{"Read", "Glob", "Grep"},
				Prompt: "You are a code reviewer. When asked to review code, " +
					"read the files and provide constructive feedback on: " +
					"code quality, potential bugs, performance, and readability. " +
					"Be specific and reference line numbers.",
			},
			"doc-writer": {
				Description: "Writes documentation for code",
				Tools:       []string{"Read", "Write", "Glob"},
				Prompt: "You are a documentation writer. When asked to document code, " +
					"read the source files and create clear, concise documentation " +
					"including function signatures, parameter descriptions, and examples.",
			},
		},
		AllowedTools: []string{"Read", "Glob"},
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
