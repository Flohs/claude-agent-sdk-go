// Filesystem agents example demonstrating loading agents from project files.
//
// Instead of defining agents inline via Options.Agents, this example shows
// how to load agent definitions from .claude/agents/*.md files on disk by
// setting SettingSources to include "project".
//
// This approach is useful when agent definitions are managed as part of the
// project configuration and shared across team members via version control.
//
// Usage:
//
//	go run ./examples/filesystem_agents
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

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
	case *claude.SystemMessage:
		if m.Subtype == "init" {
			fmt.Println("  [Init] System initialized with project settings")
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

	fmt.Println("=== Filesystem Agents Example ===")
	fmt.Println("Loads agent definitions from .claude/agents/*.md files.")
	fmt.Println()

	// Create a temporary project directory with agent definitions
	projectDir, err := createProjectWithAgents()
	if err != nil {
		log.Fatal("Failed to create project:", err)
	}
	defer func() { _ = os.RemoveAll(projectDir) }()

	fmt.Printf("Project directory: %s\n", projectDir)
	fmt.Println()

	maxTurns := 1
	client := claude.NewClient(&claude.Options{
		MaxTurns: &maxTurns,
		Cwd:      projectDir,
		SettingSources: []claude.SettingSource{
			claude.SettingSourceUser,
			claude.SettingSourceProject,
		},
	})

	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	fmt.Println("User: Summarize what this project is about.")
	if err := client.SendQuery(ctx, "Summarize what this project is about. Keep it to one sentence."); err != nil {
		log.Fatal(err)
	}

	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}
}

// createProjectWithAgents creates a temporary project directory with agent
// definition files in .claude/agents/.
func createProjectWithAgents() (string, error) {
	dir, err := os.MkdirTemp("", "fs-agents-example-*")
	if err != nil {
		return "", err
	}

	// Create agent definitions directory
	agentsDir := filepath.Join(dir, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return "", err
	}

	// Create a summarizer agent definition
	summarizer := `---
name: summarizer
description: Summarizes code and documentation concisely
tools:
  - Read
  - Glob
model: sonnet
---

You are a summarizer agent. When asked to summarize code or documentation,
read the relevant files and provide a clear, concise summary. Focus on the
key purpose, main features, and notable design decisions.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "summarizer.md"), []byte(summarizer), 0644); err != nil {
		return "", err
	}

	// Create a sample file for the agent to read
	readme := `# Example Project

This is a sample project used to demonstrate loading agent definitions from
the filesystem. The agents are defined in .claude/agents/ markdown files.
`
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0644); err != nil {
		return "", err
	}

	return dir, nil
}
