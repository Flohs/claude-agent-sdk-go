// Budget control example demonstrating MaxBudgetUSD.
//
// Usage:
//
//	go run ./examples/budget
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
			fmt.Printf("Total cost: $%.6f\n", *m.TotalCostUSD)
		}
	}
}

func budgetLimitExample(ctx context.Context) {
	fmt.Println("=== Budget Limit Example ===")
	fmt.Println("Sets a maximum budget of $0.05 for the query.")

	maxBudget := 0.05
	maxTurns := 1
	messages, errs := claude.Query(ctx, "Explain quantum computing in one paragraph.", &claude.Options{
		MaxBudgetUSD: &maxBudget,
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

func budgetWithToolsExample(ctx context.Context) {
	fmt.Println("=== Budget with Tools Example ===")
	fmt.Println("Combines budget control with tool usage.")

	maxBudget := 0.10
	messages, errs := claude.Query(ctx, "List the files in the current directory using bash.", &claude.Options{
		MaxBudgetUSD: &maxBudget,
		AllowedTools: []string{"Bash"},
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

	budgetLimitExample(ctx)
	budgetWithToolsExample(ctx)
}
