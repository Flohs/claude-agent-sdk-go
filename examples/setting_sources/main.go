// Setting sources example demonstrating how to control which configuration
// sources are loaded (user, project, local).
//
// SettingSources controls whether Claude loads settings from:
//   - "user":    ~/.claude/settings.json (global user settings)
//   - "project": .claude/settings.json in the project directory
//   - "local":   .claude/settings.local.json in the project directory
//
// When SettingSources is nil (the SDK default), no settings are loaded,
// providing an isolated environment. Set it explicitly to load project
// commands, agents, or other settings from disk.
//
// Usage:
//
//	go run ./examples/setting_sources
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
	case *claude.SystemMessage:
		if m.Subtype == "init" {
			fmt.Printf("  [Init] Received system init message (subtype=%s)\n", m.Subtype)
		}
	case *claude.ResultMessage:
		fmt.Println("Result ended")
	}
}

// isolatedMode shows the default SDK behavior: no settings loaded.
func isolatedMode(ctx context.Context) {
	fmt.Println("=== Isolated Mode (default) ===")
	fmt.Println("No settings loaded. Slash commands from .claude/commands/ are NOT available.")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "Say hello in one sentence.", &claude.Options{
		// SettingSources is nil by default, which means no settings are loaded.
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

// userSettingsOnly shows loading only global user settings.
func userSettingsOnly(ctx context.Context) {
	fmt.Println("=== User Settings Only ===")
	fmt.Println("Only global user settings from ~/.claude/settings.json are loaded.")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "Say hello in one sentence.", &claude.Options{
		SettingSources: []claude.SettingSource{claude.SettingSourceUser},
		MaxTurns:       &maxTurns,
	})

	for msg := range messages {
		displayMessage(msg)
	}
	for err := range errs {
		log.Println("Error:", err)
	}
	fmt.Println()
}

// projectSettings shows loading both user and project settings.
// This enables project-specific slash commands and agent definitions from disk.
func projectSettings(ctx context.Context) {
	fmt.Println("=== User + Project Settings ===")
	fmt.Println("Both user and project settings loaded. Project commands and agents are available.")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "Say hello in one sentence.", &claude.Options{
		SettingSources: []claude.SettingSource{
			claude.SettingSourceUser,
			claude.SettingSourceProject,
		},
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

	isolatedMode(ctx)
	userSettingsOnly(ctx)
	projectSettings(ctx)
}
