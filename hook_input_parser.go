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
			PermissionSuggestions: mapSliceField(input, "permission_suggestions"),
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

	case HookEventTeammateIdle:
		return &TeammateIdleHookInput{
			BaseHookInput:   base,
			SubagentContext: parseSubagentContext(input),
		}, nil

	case HookEventTaskCompleted:
		return &TaskCompletedHookInput{
			BaseHookInput:   base,
			SubagentContext: parseSubagentContext(input),
			TaskID:          stringField(input, "task_id"),
			ToolUseID:       stringField(input, "tool_use_id"),
		}, nil

	case HookEventConfigChange:
		return &ConfigChangeHookInput{
			BaseHookInput: base,
			Changes:       mapField(input, "changes"),
		}, nil

	case HookEventElicitation:
		var schema map[string]any
		if s, ok := input["requestedSchema"].(map[string]any); ok {
			schema = s
		}
		return &ElicitationHookInput{
			BaseHookInput:   base,
			RequestID:       stringField(input, "request_id"),
			ServerName:      stringField(input, "server_name"),
			Message:         stringField(input, "message"),
			RequestedSchema: schema,
		}, nil

	case HookEventMessageDisplay:
		return &MessageDisplayHookInput{
			BaseHookInput: base,
			TurnID:        stringField(input, "turn_id"),
			MessageID:     stringField(input, "message_id"),
			Index:         intField(input, "index"),
			Final:         boolField(input, "final"),
			Delta:         stringField(input, "delta"),
		}, nil

	case HookEventSessionStart:
		return &SessionStartHookInput{
			BaseHookInput: base,
		}, nil

	case HookEventSessionEnd:
		return &SessionEndHookInput{BaseHookInput: base}, nil

	case HookEventStopFailure:
		return &StopFailureHookInput{BaseHookInput: base}, nil

	case HookEventPostCompact:
		return &PostCompactHookInput{
			BaseHookInput:  base,
			Trigger:        stringField(input, "trigger"),
			CompactSummary: stringField(input, "compact_summary"),
		}, nil

	case HookEventPostToolBatch:
		var calls []PostToolBatchToolCall
		if raw, ok := input["tool_calls"].([]any); ok {
			calls = make([]PostToolBatchToolCall, 0, len(raw))
			for _, item := range raw {
				if m, ok := item.(map[string]any); ok {
					calls = append(calls, PostToolBatchToolCall{
						ToolName:     stringField(m, "tool_name"),
						ToolInput:    mapField(m, "tool_input"),
						ToolUseID:    stringField(m, "tool_use_id"),
						ToolResponse: m["tool_response"],
					})
				}
			}
		}
		return &PostToolBatchHookInput{
			BaseHookInput: base,
			ToolCalls:     calls,
		}, nil

	case HookEventPermissionDenied:
		return &PermissionDeniedHookInput{
			BaseHookInput: base,
			ToolName:      stringField(input, "tool_name"),
			ToolInput:     mapField(input, "tool_input"),
			ToolUseID:     stringField(input, "tool_use_id"),
			Reason:        stringField(input, "reason"),
		}, nil

	case HookEventElicitationResult:
		return &ElicitationResultHookInput{
			BaseHookInput: base,
			McpServerName: stringField(input, "mcp_server_name"),
			ElicitationID: stringField(input, "elicitation_id"),
			Mode:          stringField(input, "mode"),
			Action:        stringField(input, "action"),
			Content:       mapField(input, "content"),
		}, nil

	case HookEventInstructionsLoaded:
		return &InstructionsLoadedHookInput{
			BaseHookInput:   base,
			FilePath:        stringField(input, "file_path"),
			MemoryType:      stringField(input, "memory_type"),
			LoadReason:      stringField(input, "load_reason"),
			Globs:           stringSliceField(input, "globs"),
			TriggerFilePath: stringField(input, "trigger_file_path"),
			ParentFilePath:  stringField(input, "parent_file_path"),
		}, nil

	case HookEventCwdChanged:
		return &CwdChangedHookInput{
			BaseHookInput: base,
			OldCwd:        stringField(input, "old_cwd"),
			NewCwd:        stringField(input, "new_cwd"),
		}, nil

	case HookEventFileChanged:
		return &FileChangedHookInput{
			BaseHookInput: base,
			FilePath:      stringField(input, "file_path"),
			ChangeType:    stringField(input, "change_type"),
		}, nil

	case HookEventWorktreeCreate:
		return &WorktreeCreateHookInput{
			BaseHookInput:  base,
			WorktreeName:   stringField(input, "worktree_name"),
			IsolationLevel: stringField(input, "isolation_level"),
		}, nil

	case HookEventWorktreeRemove:
		return &WorktreeRemoveHookInput{
			BaseHookInput: base,
			WorktreePath:  stringField(input, "worktree_path"),
		}, nil

	case HookEventUserPromptExpansion:
		return &UserPromptExpansionHookInput{
			BaseHookInput: base,
			ExpansionType: stringField(input, "expansion_type"),
			CommandName:   stringField(input, "command_name"),
			CommandArgs:   stringField(input, "command_args"),
			CommandSource: stringField(input, "command_source"),
			Prompt:        stringField(input, "prompt"),
		}, nil

	case HookEventSetup:
		return &SetupHookInput{
			BaseHookInput: base,
			Trigger:       stringField(input, "trigger"),
		}, nil

	case HookEventTaskCreated:
		return &TaskCreatedHookInput{
			BaseHookInput:   base,
			SubagentContext: parseSubagentContext(input),
			TaskName:        stringField(input, "task_name"),
			TaskDescription: stringField(input, "task_description"),
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

func stringSliceField(m map[string]any, key string) []string {
	raw, _ := m[key].([]any)
	if raw == nil {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func mapSliceField(m map[string]any, key string) []map[string]any {
	raw, _ := m[key].([]any)
	if raw == nil {
		return nil
	}
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if entry, ok := item.(map[string]any); ok {
			result = append(result, entry)
		}
	}
	return result
}
