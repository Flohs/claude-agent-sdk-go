package claude

import "testing"

func TestSendFeedbackInput_DecodesFromToolInput(t *testing.T) {
	toolInput := map[string]any{
		"type":    "bug",
		"title":   "hooks config crashes on empty matcher",
		"details": "Set matcher to \"\" in settings.json, ran claude, got a stack trace.",
		"area":    "hooks config",
	}

	var in SendFeedbackInput
	decodeToolUseResult(t, toolInput, &in)

	if in.Type != "bug" {
		t.Errorf("Type = %q, want %q", in.Type, "bug")
	}
	if in.Title != "hooks config crashes on empty matcher" {
		t.Errorf("Title = %q, want %q", in.Title, "hooks config crashes on empty matcher")
	}
	if in.Area != "hooks config" {
		t.Errorf("Area = %q, want %q", in.Area, "hooks config")
	}
}

func TestSendFeedbackOutput_DecodesFromToolResponse(t *testing.T) {
	toolResponse := map[string]any{
		"success": true,
		"message": "Thanks, feedback recorded.",
	}

	var out SendFeedbackOutput
	decodeToolUseResult(t, toolResponse, &out)

	if !out.Success {
		t.Error("Success = false, want true")
	}
	if out.Message != "Thanks, feedback recorded." {
		t.Errorf("Message = %q, want %q", out.Message, "Thanks, feedback recorded.")
	}
}

func TestProposeSkillsInput_DecodesFromToolInput(t *testing.T) {
	toolInput := map[string]any{
		"proposals": []any{
			map[string]any{
				"name":        "deploy-checklist",
				"kind":        "new",
				"description": "Run the deploy checklist before shipping",
				"evidence":    []any{"~/.claude/memory/2026-07-10.md"},
				"skillMd":     "---\nname: deploy-checklist\n---\n\n## Trigger\n...",
			},
			map[string]any{
				"name":        "code-review",
				"kind":        "improvement",
				"target":      "code-review",
				"description": "Add a security-focused pass",
				"skillMd":     "---\nname: code-review\n---\n\n## Trigger\n...",
			},
		},
	}

	var in ProposeSkillsInput
	decodeToolUseResult(t, toolInput, &in)

	if len(in.Proposals) != 2 {
		t.Fatalf("len(Proposals) = %d, want 2", len(in.Proposals))
	}
	if in.Proposals[0].Name != "deploy-checklist" || in.Proposals[0].Kind != "new" {
		t.Errorf("Proposals[0] = %+v, want name=deploy-checklist kind=new", in.Proposals[0])
	}
	if len(in.Proposals[0].Evidence) != 1 || in.Proposals[0].Evidence[0] != "~/.claude/memory/2026-07-10.md" {
		t.Errorf("Proposals[0].Evidence = %v, want [~/.claude/memory/2026-07-10.md]", in.Proposals[0].Evidence)
	}
	if in.Proposals[1].Kind != "improvement" || in.Proposals[1].Target != "code-review" {
		t.Errorf("Proposals[1] = %+v, want kind=improvement target=code-review", in.Proposals[1])
	}
}

func TestProposeSkillsOutput_DecodesFromToolResponse(t *testing.T) {
	toolResponse := map[string]any{
		"proposalCount": float64(2),
	}

	var out ProposeSkillsOutput
	decodeToolUseResult(t, toolResponse, &out)

	if out.ProposalCount != 2 {
		t.Errorf("ProposalCount = %d, want 2", out.ProposalCount)
	}
}

func TestSkillToolOutput_DecodesFromToolResponse_Background(t *testing.T) {
	toolResponse := map[string]any{
		"background": true,
	}

	var out SkillToolOutput
	decodeToolUseResult(t, toolResponse, &out)

	if !out.Background {
		t.Error("Background = false, want true")
	}
}

func TestSkillToolOutput_DecodesFromToolResponse_Inline(t *testing.T) {
	toolResponse := map[string]any{}

	var out SkillToolOutput
	decodeToolUseResult(t, toolResponse, &out)

	if out.Background {
		t.Error("Background = true, want false when the key is absent (inline skill run)")
	}
}
