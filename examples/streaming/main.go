// Streaming mode examples using Client for bidirectional conversations.
//
// Usage:
//
//	go run ./examples/streaming
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

func displayMessage(msg claude.Message) {
	switch m := msg.(type) {
	case *claude.UserMessage:
		if s, ok := m.Content.(string); ok {
			fmt.Println("User:", s)
		}
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

func basicStreaming(ctx context.Context) {
	fmt.Println("=== Basic Streaming Example ===")

	client := claude.NewClient(nil)
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	fmt.Println("User: What is 2+2?")
	if err := client.SendQuery(ctx, "What is 2+2?"); err != nil {
		log.Fatal(err)
	}

	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}
	fmt.Println()
}

func multiTurnConversation(ctx context.Context) {
	fmt.Println("=== Multi-Turn Conversation Example ===")

	client := claude.NewClient(nil)
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	// First turn
	fmt.Println("User: What's the capital of France?")
	if err := client.SendQuery(ctx, "What's the capital of France?"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	// Second turn - follow-up
	fmt.Println("\nUser: What's the population of that city?")
	if err := client.SendQuery(ctx, "What's the population of that city?"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}
	fmt.Println()
}

func bashCommandExample(ctx context.Context) {
	fmt.Println("=== Bash Command Example ===")

	client := claude.NewClient(&claude.Options{
		AllowedTools: []string{"Bash"},
	})
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	fmt.Println("User: Run a bash echo command")
	if err := client.SendQuery(ctx, "Run a bash echo command that says 'Hello from Go SDK!'"); err != nil {
		log.Fatal(err)
	}

	for msg := range client.ReceiveResponse(ctx) {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				switch b := block.(type) {
				case claude.TextBlock:
					fmt.Println("Claude:", b.Text)
				case claude.ToolUseBlock:
					fmt.Printf("Tool Use: %s (id: %s)\n", b.Name, b.ID)
					if b.Name == "Bash" {
						if cmd, ok := b.Input["command"].(string); ok {
							fmt.Println("  Command:", cmd)
						}
					}
				}
			}
		case *claude.UserMessage:
			if blocks, ok := m.Content.([]claude.ContentBlock); ok {
				for _, block := range blocks {
					if tr, ok := block.(claude.ToolResultBlock); ok {
						content := fmt.Sprintf("%v", tr.Content)
						if len(content) > 100 {
							content = content[:100] + "..."
						}
						fmt.Printf("Tool Result (id: %s): %s\n", tr.ToolUseID, content)
					}
				}
			}
		case *claude.ResultMessage:
			displayMessage(msg)
		}
	}
	fmt.Println()
}

func main() {
	ctx := context.Background()

	basicStreaming(ctx)
	multiTurnConversation(ctx)
	bashCommandExample(ctx)
}
