// Partial message streaming example using IncludePartialMessages.
//
// This example demonstrates receiving StreamEvent messages as Claude generates
// content, enabling real-time UI updates with incremental text deltas.
//
// Usage:
//
//	go run ./examples/include_partial_messages
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Partial Message Streaming Example ===")
	fmt.Println("Demonstrates receiving StreamEvent messages for real-time UI updates.")
	fmt.Println()

	maxTurns := 1
	client := claude.NewClient(&claude.Options{
		IncludePartialMessages: true,
		MaxTurns:               &maxTurns,
	})

	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	fmt.Println("User: Write a haiku about Go programming.")
	if err := client.SendQuery(ctx, "Write a haiku about Go programming. Just the haiku, nothing else."); err != nil {
		log.Fatal(err)
	}

	streamEventCount := 0
	for msg := range client.ReceiveResponse(ctx) {
		switch m := msg.(type) {
		case *claude.StreamEvent:
			streamEventCount++
			eventType, _ := m.Event["type"].(string)
			fmt.Printf("  [StreamEvent #%d] type=%s\n", streamEventCount, eventType)

			// Extract text delta from content_block_delta events
			if eventType == "content_block_delta" {
				if delta, ok := m.Event["delta"].(map[string]any); ok {
					if text, ok := delta["text"].(string); ok {
						fmt.Printf("  [Delta] %q\n", text)
					}
				}
			}

		case *claude.AssistantMessage:
			fmt.Println()
			for _, block := range m.Content {
				if tb, ok := block.(claude.TextBlock); ok {
					fmt.Println("Claude:", tb.Text)
				}
			}

		case *claude.ResultMessage:
			fmt.Printf("\nReceived %d stream events total\n", streamEventCount)
			if m.TotalCostUSD != nil {
				fmt.Printf("Cost: $%.4f\n", *m.TotalCostUSD)
			}
		}
	}
}
