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
	"time"

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

func serverInfoExample(ctx context.Context) {
	fmt.Println("=== Server Info Example ===")

	client := claude.NewClient(nil)
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	// GetServerInfo returns the initialization data from the CLI
	info := client.GetServerInfo()
	if info != nil {
		fmt.Println("Server info received:")
		if version, ok := info["version"].(string); ok {
			fmt.Println("  CLI version:", version)
		}
		if tools, ok := info["tools"].([]any); ok {
			fmt.Printf("  Available tools: %d\n", len(tools))
		}
		if sessionID, ok := info["session_id"].(string); ok {
			fmt.Println("  Session ID:", sessionID)
		}
	} else {
		fmt.Println("No server info available")
	}
	fmt.Println()
}

func interruptExample(ctx context.Context) {
	fmt.Println("=== Interrupt Example ===")

	client := claude.NewClient(&claude.Options{
		AllowedTools: []string{"Bash"},
	})
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	// Start a long-running task
	fmt.Println("User: Write a very long essay about the history of computing.")
	if err := client.SendQuery(ctx, "Write a very long and detailed essay about the history of computing from the 1800s to today."); err != nil {
		log.Fatal(err)
	}

	// Consume a few messages, then interrupt
	msgCount := 0
	for msg := range client.ReceiveResponse(ctx) {
		msgCount++
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if tb, ok := block.(claude.TextBlock); ok {
					preview := tb.Text
					if len(preview) > 80 {
						preview = preview[:80] + "..."
					}
					fmt.Println("Claude:", preview)
				}
			}
		case *claude.ResultMessage:
			fmt.Println("Result ended")
			if m.TotalCostUSD != nil {
				fmt.Printf("Cost: $%.4f\n", *m.TotalCostUSD)
			}
		}

		// Interrupt after receiving a few messages
		if msgCount >= 2 {
			fmt.Println("\n  [Sending interrupt...]")
			if err := client.Interrupt(ctx); err != nil {
				fmt.Println("  Interrupt error:", err)
			}
		}
	}
	fmt.Println()
}

func timeoutExample(ctx context.Context) {
	fmt.Println("=== Timeout Example ===")
	fmt.Println("Demonstrates using context timeout for error handling.")

	// Create a context with a short timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	maxTurns := 1
	messages, errs := claude.Query(timeoutCtx, "What is 2+2? One word answer.", &claude.Options{
		MaxTurns: &maxTurns,
	})

	for msg := range messages {
		displayMessage(msg)
	}
	for err := range errs {
		fmt.Println("Error:", err)
	}

	// Check if the context was cancelled
	if timeoutCtx.Err() == context.DeadlineExceeded {
		fmt.Println("  Request timed out!")
	} else {
		fmt.Println("  Completed within timeout.")
	}
	fmt.Println()
}

func main() {
	ctx := context.Background()

	basicStreaming(ctx)
	multiTurnConversation(ctx)
	bashCommandExample(ctx)
	serverInfoExample(ctx)
	interruptExample(ctx)
	timeoutExample(ctx)
}
