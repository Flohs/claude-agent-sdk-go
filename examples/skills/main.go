// Skills example demonstrating the top-level Options.Skills shortcut.
//
// Without Skills, enabling skills requires two coordinated options:
//   - AllowedTools must contain `Skill` (or `Skill(name)` patterns for
//     specific skills), otherwise the CLI rejects skill invocations.
//   - SettingSources must include `user` and/or `project` so the CLI can
//     discover the skill definitions from disk.
//
// Options.Skills wires both automatically. Pass the string "all" to enable
// every discovered skill, or a []string for named skills. Explicit
// SettingSources or AllowedTools values supplied by the caller are
// preserved and take precedence.
//
// Usage:
//
//	go run ./examples/skills
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

// allSkills enables every skill discovered under the user's and the
// project's setting sources.
func allSkills(ctx context.Context) {
	fmt.Println("=== Skills: \"all\" ===")
	fmt.Println("Enables every discovered skill. SettingSources defaults to [user, project].")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "What skills are available in this session?", &claude.Options{
		Skills:   "all",
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

// namedSkills enables only the skills the caller lists explicitly.
func namedSkills(ctx context.Context) {
	fmt.Println("=== Skills: [\"pdf-tools\", \"image-tools\"] ===")
	fmt.Println("Only the listed skills are auto-injected as Skill(name) patterns.")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "List which of the pdf-tools or image-tools skills you can invoke.", &claude.Options{
		Skills:   []string{"pdf-tools", "image-tools"},
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

// skillsWithExtraTools mixes Options.Skills with additional allowed tools
// and an explicit SettingSources override.
func skillsWithExtraTools(ctx context.Context) {
	fmt.Println("=== Skills + explicit AllowedTools + SettingSources ===")
	fmt.Println("User-supplied AllowedTools and SettingSources are preserved; Skill entries are merged in.")

	maxTurns := 1
	messages, errs := claude.Query(ctx, "Read README.md, then list any available skills.", &claude.Options{
		Skills:         "all",
		AllowedTools:   []string{"Read"}, // Skill gets merged alongside Read
		SettingSources: []claude.SettingSource{claude.SettingSourceProject},
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

func main() {
	ctx := context.Background()

	allSkills(ctx)
	namedSkills(ctx)
	skillsWithExtraTools(ctx)
}
