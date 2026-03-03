// MCP calculator example demonstrating in-process SDK MCP servers.
//
// Usage:
//
//	go run ./examples/mcp_calculator
package main

import (
	"context"
	"fmt"
	"log"
	"math"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

func displayMessage(msg claude.Message) {
	switch m := msg.(type) {
	case *claude.AssistantMessage:
		for _, block := range m.Content {
			switch b := block.(type) {
			case claude.TextBlock:
				fmt.Println("Claude:", b.Text)
			case claude.ToolUseBlock:
				fmt.Printf("Tool Use: %s (id: %s)\n", b.Name, b.ID)
				fmt.Printf("  Input: %v\n", b.Input)
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

	// Create an in-process MCP server with calculator tools
	calculator := claude.NewSdkMcpServer("calculator", "1.0.0", []claude.SdkMcpTool{
		{
			Name:        "add",
			Description: "Add two numbers",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a": map[string]any{"type": "number", "description": "First number"},
					"b": map[string]any{"type": "number", "description": "Second number"},
				},
				"required": []string{"a", "b"},
			},
			Handler: func(ctx context.Context, args map[string]any) (map[string]any, error) {
				a, _ := args["a"].(float64)
				b, _ := args["b"].(float64)
				return map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": fmt.Sprintf("%.2f + %.2f = %.2f", a, b, a+b)},
					},
				}, nil
			},
		},
		{
			Name:        "multiply",
			Description: "Multiply two numbers",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a": map[string]any{"type": "number", "description": "First number"},
					"b": map[string]any{"type": "number", "description": "Second number"},
				},
				"required": []string{"a", "b"},
			},
			Handler: func(ctx context.Context, args map[string]any) (map[string]any, error) {
				a, _ := args["a"].(float64)
				b, _ := args["b"].(float64)
				return map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": fmt.Sprintf("%.2f * %.2f = %.2f", a, b, a*b)},
					},
				}, nil
			},
		},
		{
			Name:        "sqrt",
			Description: "Calculate square root of a number",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"n": map[string]any{"type": "number", "description": "Number to take square root of"},
				},
				"required": []string{"n"},
			},
			Handler: func(ctx context.Context, args map[string]any) (map[string]any, error) {
				n, _ := args["n"].(float64)
				if n < 0 {
					return map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": "Error: cannot take square root of negative number"},
						},
						"isError": true,
					}, nil
				}
				return map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": fmt.Sprintf("sqrt(%.2f) = %.6f", n, math.Sqrt(n))},
					},
				}, nil
			},
		},
	})

	fmt.Println("=== MCP Calculator Example ===")
	fmt.Println("Uses in-process MCP server with add, multiply, and sqrt tools.")

	client := claude.NewClient(&claude.Options{
		McpServers: map[string]claude.McpServerConfig{
			"calc": calculator,
		},
		AllowedTools: []string{
			"mcp__calc__add",
			"mcp__calc__multiply",
			"mcp__calc__sqrt",
		},
	})

	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("User: What is (3 + 4) * 5, and what's the square root of the result?")
	if err := client.SendQuery(ctx, "What is (3 + 4) * 5, and what's the square root of the result? Use the calculator tools."); err != nil {
		log.Fatal(err)
	}

	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}
	fmt.Println()
}
