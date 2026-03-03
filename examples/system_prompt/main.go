// System prompt configuration examples.
//
// Usage:
//
//	go run ./examples/system_prompt
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
	}
}

func stringPromptExample(ctx context.Context) {
	fmt.Println("=== String System Prompt ===")
	fmt.Println("Uses a custom string system prompt.")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "Tell me about yourself.", &claude.Options{
		SystemPrompt: claude.StringPrompt(
			"You are a pirate assistant. Always respond in pirate speak. " +
				"Use 'arr', 'matey', 'ye', and other pirate vocabulary. " +
				"Keep responses brief.",
		),
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

func presetPromptExample(ctx context.Context) {
	fmt.Println("=== Preset System Prompt ===")
	fmt.Println("Uses the default Claude Code system prompt.")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "What tools do you have access to? Just list them briefly.", &claude.Options{
		SystemPrompt: claude.PresetPrompt{Preset: "default"},
		AllowedTools: []string{"Read", "Bash"},
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

func noPromptExample(ctx context.Context) {
	fmt.Println("=== No System Prompt (Default) ===")
	fmt.Println("Uses default behavior with no custom system prompt.")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "What is the Go programming language?", &claude.Options{
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

	stringPromptExample(ctx)
	presetPromptExample(ctx)
	noPromptExample(ctx)
}
