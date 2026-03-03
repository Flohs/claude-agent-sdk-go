// Quick start examples for the Claude Agent SDK for Go.
//
// Usage:
//
//	go run ./examples/quick_start
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

func basicExample(ctx context.Context) {
	fmt.Println("=== Basic Example ===")

	messages, errs := claude.Query(ctx, "What is 2 + 2?", nil)

	for msg := range messages {
		if m, ok := msg.(*claude.AssistantMessage); ok {
			for _, block := range m.Content {
				if tb, ok := block.(claude.TextBlock); ok {
					fmt.Println("Claude:", tb.Text)
				}
			}
		}
	}
	for err := range errs {
		log.Println("Error:", err)
	}
	fmt.Println()
}

func withOptionsExample(ctx context.Context) {
	fmt.Println("=== With Options Example ===")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "Explain what Go is in one sentence.", &claude.Options{
		SystemPrompt: claude.StringPrompt("You are a helpful assistant that explains things simply."),
		MaxTurns:     &maxTurns,
	})

	for msg := range messages {
		if m, ok := msg.(*claude.AssistantMessage); ok {
			for _, block := range m.Content {
				if tb, ok := block.(claude.TextBlock); ok {
					fmt.Println("Claude:", tb.Text)
				}
			}
		}
	}
	for err := range errs {
		log.Println("Error:", err)
	}
	fmt.Println()
}

func withToolsExample(ctx context.Context) {
	fmt.Println("=== With Tools Example ===")

	messages, errs := claude.Query(ctx, "Create a file called hello.txt with 'Hello, World!' in it", &claude.Options{
		AllowedTools: []string{"Read", "Write"},
		SystemPrompt: claude.StringPrompt("You are a helpful file assistant."),
	})

	for msg := range messages {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if tb, ok := block.(claude.TextBlock); ok {
					fmt.Println("Claude:", tb.Text)
				}
			}
		case *claude.ResultMessage:
			if m.TotalCostUSD != nil && *m.TotalCostUSD > 0 {
				fmt.Printf("\nCost: $%.4f\n", *m.TotalCostUSD)
			}
		}
	}
	for err := range errs {
		log.Println("Error:", err)
	}
	fmt.Println()
}

func main() {
	ctx := context.Background()

	basicExample(ctx)
	withOptionsExample(ctx)
	withToolsExample(ctx)
}
