package claude

import (
	"reflect"
	"testing"
)

func TestPostToolUseHookOutput_ToHookJSONOutput_Empty(t *testing.T) {
	got := PostToolUseHookOutput{}.ToHookJSONOutput()
	want := HookJSONOutput{
		"hookSpecificOutput": map[string]any{"hookEventName": "PostToolUse"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ToHookJSONOutput() = %#v, want %#v", got, want)
	}
}

func TestPostToolUseHookOutput_ToHookJSONOutput_AllFields(t *testing.T) {
	got := PostToolUseHookOutput{
		UpdatedToolOutput:    "sanitized",
		UpdatedMCPToolOutput: "legacy",
		AdditionalContext:    "extra context",
		ClassifierContext:    "classifier note",
	}.ToHookJSONOutput()

	want := HookJSONOutput{
		"hookSpecificOutput": map[string]any{
			"hookEventName":        "PostToolUse",
			"updatedToolOutput":    "sanitized",
			"updatedMCPToolOutput": "legacy",
			"additionalContext":    "extra context",
			"classifierContext":    "classifier note",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ToHookJSONOutput() = %#v, want %#v", got, want)
	}
}

func TestPostToolUseHookOutput_ToHookJSONOutput_NestedUnderHookSpecificOutput(t *testing.T) {
	// Regression test: updatedToolOutput/updatedMCPToolOutput/additionalContext/
	// classifierContext must live under "hookSpecificOutput", not top-level —
	// per https://code.claude.com/docs/en/hooks, the CLI only honors them nested.
	got := PostToolUseHookOutput{UpdatedToolOutput: "x"}.ToHookJSONOutput()
	if _, ok := got["updatedToolOutput"]; ok {
		t.Errorf("ToHookJSONOutput() must not place updatedToolOutput at the top level, got %#v", got)
	}
	specific, ok := got["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("ToHookJSONOutput() missing hookSpecificOutput, got %#v", got)
	}
	if specific["updatedToolOutput"] != "x" {
		t.Errorf("hookSpecificOutput[updatedToolOutput] = %#v, want %q", specific["updatedToolOutput"], "x")
	}
}
