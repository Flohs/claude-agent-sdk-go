package claude

// ParseHookInput converts a raw [HookInput] map into a typed struct.
// Returns nil (not an error) for unrecognized hook event names, keeping
// forward compatibility with future CLI versions that may add new events.
func ParseHookInput(input HookInput) (TypedHookInput, error) {
	if input == nil {
		return nil, nil
	}

	eventName := stringField(input, "hook_event_name")
	base := parseBaseHookInput(input)

	switch HookEvent(eventName) {
	case HookEventPreToolUse:
		return &PreToolUseHookInput{
			BaseHookInput:   base,
			SubagentContext: parseSubagentContext(input),
			ToolName:        stringField(input, "tool_name"),
			ToolInput:       mapField(input, "tool_input"),
			ToolUseID:       stringField(input, "tool_use_id"),
		}, nil

	case HookEventPostToolUse:
		return &PostToolUseHookInput{
			BaseHookInput:   base,
			SubagentContext: parseSubagentContext(input),
			ToolName:        stringField(input, "tool_name"),
			ToolInput:       mapField(input, "tool_input"),
			ToolResponse:    input["tool_response"],
			ToolUseID:       stringField(input, "tool_use_id"),
		}, nil

	case HookEventPostToolUseFailure:
		return &PostToolUseFailureHookInput{
			BaseHookInput:   base,
			SubagentContext: parseSubagentContext(input),
			ToolName:        stringField(input, "tool_name"),
			ToolInput:       mapField(input, "tool_input"),
			ToolUseID:       stringField(input, "tool_use_id"),
			Error:           stringField(input, "error"),
			IsInterrupt:     boolField(input, "is_interrupt"),
		}, nil

	case HookEventPermissionRequest:
		return &PermissionRequestHookInput{
			BaseHookInput:   base,
			SubagentContext: parseSubagentContext(input),
			ToolName:        stringField(input, "tool_name"),
			ToolInput:       mapField(input, "tool_input"),
			PermissionSuggestions: sliceField(input, "permission_suggestions"),
		}, nil

	case HookEventUserPromptSubmit:
		return &UserPromptSubmitHookInput{
			BaseHookInput: base,
			Prompt:        stringField(input, "prompt"),
		}, nil

	case HookEventStop:
		return &StopHookInput{
			BaseHookInput:  base,
			StopHookActive: boolField(input, "stop_hook_active"),
		}, nil

	case HookEventSubagentStop:
		return &SubagentStopHookInput{
			BaseHookInput:       base,
			StopHookActive:      boolField(input, "stop_hook_active"),
			AgentID:             stringField(input, "agent_id"),
			AgentTranscriptPath: stringField(input, "agent_transcript_path"),
			AgentType:           stringField(input, "agent_type"),
		}, nil

	case HookEventSubagentStart:
		return &SubagentStartHookInput{
			BaseHookInput: base,
			AgentID:       stringField(input, "agent_id"),
			AgentType:     stringField(input, "agent_type"),
		}, nil

	case HookEventPreCompact:
		return &PreCompactHookInput{
			BaseHookInput:      base,
			Trigger:            stringField(input, "trigger"),
			CustomInstructions: stringField(input, "custom_instructions"),
		}, nil

	case HookEventNotification:
		return &NotificationHookInput{
			BaseHookInput:    base,
			Message:          stringField(input, "message"),
			Title:            stringField(input, "title"),
			NotificationType: stringField(input, "notification_type"),
		}, nil

	default:
		// Forward-compatible: return nil for unrecognized events.
		return nil, nil
	}
}

func parseBaseHookInput(m map[string]any) BaseHookInput {
	return BaseHookInput{
		SessionID:      stringField(m, "session_id"),
		TranscriptPath: stringField(m, "transcript_path"),
		Cwd:            stringField(m, "cwd"),
		PermissionMode: stringField(m, "permission_mode"),
		HookEventName:  stringField(m, "hook_event_name"),
	}
}

func parseSubagentContext(m map[string]any) SubagentContext {
	return SubagentContext{
		AgentID:   stringField(m, "agent_id"),
		AgentType: stringField(m, "agent_type"),
	}
}

func mapField(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func sliceField(m map[string]any, key string) []any {
	v, _ := m[key].([]any)
	return v
}
