// Stderr callback example demonstrating how to capture CLI debug output.
//
// The Stderr callback receives each line of stderr output from the Claude CLI
// subprocess. Combined with the "debug-to-stderr" extra arg, this enables
// capturing detailed debug logs for diagnostics and monitoring.
//
// Usage:
//
//	go run ./examples/stderr_callback
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Stderr Callback Example ===")
	fmt.Println("Captures CLI debug output via the Stderr callback.")
	fmt.Println()

	var mu sync.Mutex
	var stderrLines []string

	maxTurns := 1
	messages, errs := claude.Query(ctx, "What is 2+2? Answer in one word.", &claude.Options{
		MaxTurns: &maxTurns,
		// Enable debug output on stderr
		ExtraArgs: map[string]string{
			"debug-to-stderr": "",
		},
		// Capture stderr lines
		Stderr: func(line string) {
			mu.Lock()
			defer mu.Unlock()
			stderrLines = append(stderrLines, line)
		},
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
			if m.TotalCostUSD != nil {
				fmt.Printf("Cost: $%.4f\n", *m.TotalCostUSD)
			}
		}
	}
	for err := range errs {
		log.Println("Error:", err)
	}

	mu.Lock()
	defer mu.Unlock()

	fmt.Printf("\n--- Captured %d stderr lines ---\n", len(stderrLines))
	for i, line := range stderrLines {
		// Show first 10 lines as a preview
		if i >= 10 {
			fmt.Printf("  ... and %d more lines\n", len(stderrLines)-10)
			break
		}
		// Truncate long lines
		if len(line) > 120 {
			line = line[:120] + "..."
		}
		// Redact any potential sensitive data
		line = strings.ReplaceAll(line, "\n", " ")
		fmt.Printf("  [stderr] %s\n", line)
	}
}
