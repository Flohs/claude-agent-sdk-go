// Plugin example demonstrating how to load local plugins.
//
// Plugins extend Claude Code with custom commands, agents, skills, and hooks.
// A plugin is a directory containing a .claude-plugin/plugin.json manifest
// and optional command files in a commands/ subdirectory.
//
// This example loads a demo plugin that provides a custom /greet command.
//
// Usage:
//
//	go run ./examples/plugins
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
			fmt.Println("  [Init] System initialized")
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

	fmt.Println("=== Plugin Example ===")
	fmt.Println("Loads a local demo plugin with a custom /greet command.")
	fmt.Println()

	// Create a temporary demo plugin
	pluginDir, err := createDemoPlugin()
	if err != nil {
		log.Fatal("Failed to create demo plugin:", err)
	}
	defer func() { _ = os.RemoveAll(pluginDir) }()

	fmt.Printf("Plugin directory: %s\n\n", pluginDir)

	maxTurns := 1
	messages, errs := claude.Query(ctx, "Say hello to the user. Keep it brief.", &claude.Options{
		MaxTurns: &maxTurns,
		Plugins: []claude.SdkPluginConfig{
			{Type: "local", Path: pluginDir},
		},
	})

	for msg := range messages {
		displayMessage(msg)
	}
	for err := range errs {
		log.Println("Error:", err)
	}
}

// createDemoPlugin creates a temporary plugin directory with the required structure.
func createDemoPlugin() (string, error) {
	dir, err := os.MkdirTemp("", "demo-plugin-*")
	if err != nil {
		return "", err
	}

	// Create plugin manifest
	manifestDir := filepath.Join(dir, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		return "", err
	}

	manifest := `{
  "name": "demo-plugin",
  "description": "A demo plugin for the Go SDK example",
  "version": "1.0.0",
  "author": "Go SDK Examples"
}`
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(manifest), 0644); err != nil {
		return "", err
	}

	// Create a custom command
	commandsDir := filepath.Join(dir, "commands")
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return "", err
	}

	command := `Greet the user warmly. Include a fun fact about Go programming.`
	if err := os.WriteFile(filepath.Join(commandsDir, "greet.md"), []byte(command), 0644); err != nil {
		return "", err
	}

	return dir, nil
}
