package claude

import "fmt"

// ParseMessage parses a raw JSON message from CLI output into a typed Message.
// Returns nil for unrecognized message types (forward-compatible).
func ParseMessage(data map[string]any) (Message, error) {
	if data == nil {
		return nil, &MessageParseError{SDKError: SDKError{Message: "Invalid message data: nil"}}
	}

	msgType, _ := data["type"].(string)
	if msgType == "" {
		return nil, &MessageParseError{
			SDKError: SDKError{Message: "Message missing 'type' field"},
			Data:     data,
		}
	}

	switch msgType {
	case "user":
		return parseUserMessage(data)
	case "assistant":
		return parseAssistantMessage(data)
	case "system":
		return parseSystemMessage(data)
	case "result":
		return parseResultMessage(data)
	case "stream_event":
		return parseStreamEvent(data)
	case "rate_limit_event":
		return parseRateLimitEvent(data)

	default:
		// Forward-compatible: skip unrecognized message types
		return nil, nil
	}
}

func parseUserMessage(data map[string]any) (*UserMessage, error) {
	msg := &UserMessage{
		ParentToolUseID: stringField(data, "parent_tool_use_id"),
		UUID:            stringField(data, "uuid"),
		Timestamp:       stringField(data, "timestamp"),
		IsMeta:          boolField(data, "isMeta"),
	}

	if tr, ok := data["tool_use_result"].(map[string]any); ok {
		msg.ToolUseResult = tr
	}

	rawMessage, exists := data["message"]
	if !exists || rawMessage == nil {
		return nil, &MessageParseError{
			SDKError: SDKError{Message: "Missing 'message' field in user message"},
			Data:     data,
		}
	}
	message, ok := rawMessage.(map[string]any)
	if !ok {
		return nil, &MessageParseError{
			SDKError: SDKError{Message: "Wrong type for 'message' field in user message"},
			Data:     data,
		}
	}

	content := message["content"]
	switch c := content.(type) {
	case string:
		msg.Content = c
	case []any:
		blocks, err := parseContentBlocks(c)
		if err != nil {
			return nil, err
		}
		msg.Content = blocks
	default:
		msg.Content = fmt.Sprintf("%v", content)
	}

	return msg, nil
}

func parseAssistantMessage(data map[string]any) (*AssistantMessage, error) {
	rawMessage, exists := data["message"]
	if !exists || rawMessage == nil {
		return nil, &MessageParseError{
			SDKError: SDKError{Message: "Missing 'message' field in assistant message"},
			Data:     data,
		}
	}
	message, ok := rawMessage.(map[string]any)
	if !ok {
		return nil, &MessageParseError{
			SDKError: SDKError{Message: "Wrong type for 'message' field in assistant message"},
			Data:     data,
		}
	}

	rawContent, hasContent := message["content"]
	if !hasContent || rawContent == nil {
		return nil, &MessageParseError{
			SDKError: SDKError{Message: "Missing 'content' field in assistant message"},
			Data:     data,
		}
	}
	contentRaw, ok := rawContent.([]any)
	if !ok {
		return nil, &MessageParseError{
			SDKError: SDKError{Message: fmt.Sprintf("Invalid assistant content: expected list, got %T", rawContent)},
			Data:     data,
		}
	}

	blocks, err := parseContentBlocks(contentRaw)
	if err != nil {
		return nil, err
	}

	model, _ := message["model"].(string)

	msg := &AssistantMessage{
		Content:         blocks,
		Model:           model,
		ParentToolUseID: stringField(data, "parent_tool_use_id"),
		MessageID:       stringField(message, "id"),
		SessionID:       stringField(data, "session_id"),
		UUID:            stringField(data, "uuid"),
		StopReason:      stringField(message, "stop_reason"),
		RequestID:       stringField(data, "request_id"),
		Timestamp:       stringField(data, "timestamp"),
	}

	if errStr := stringField(data, "error"); errStr != "" {
		msg.Error = AssistantMessageError(errStr)
	}

	if usage, ok := message["usage"].(map[string]any); ok {
		msg.Usage = usage
	}

	if sd, ok := message["stop_details"].(map[string]any); ok {
		msg.StopDetails = sd
	}

	if rawMeta, ok := data["tool_use_meta"].(map[string]any); ok {
		meta := make(ToolUseMeta, len(rawMeta))
		for id, v := range rawMeta {
			if entry, ok := v.(map[string]any); ok {
				meta[id] = ToolUseMetaEntry{
					Name:    stringField(entry, "name"),
					IconURL: stringField(entry, "icon_url"),
				}
			}
		}
		msg.ToolUseMeta = meta
	}

	msg.RawData = data

	return msg, nil
}

func parseSystemMessage(data map[string]any) (Message, error) {
	subtype := stringField(data, "subtype")
	if subtype == "" {
		return nil, &MessageParseError{
			SDKError: SDKError{Message: "Missing 'subtype' field in system message"},
			Data:     data,
		}
	}

	base := SystemMessage{
		Subtype:   subtype,
		Data:      data,
		Timestamp: stringField(data, "timestamp"),
	}

	switch subtype {
	case "task_started":
		return &TaskStartedMessage{
			SystemMessage:   base,
			TaskID:          stringField(data, "task_id"),
			Description:     stringField(data, "description"),
			UUID:            stringField(data, "uuid"),
			SessionID:       stringField(data, "session_id"),
			ToolUseID:       stringField(data, "tool_use_id"),
			TaskType:        stringField(data, "task_type"),
			SubagentType:    stringField(data, "subagent_type"),
			TaskDescription: stringField(data, "task_description"),
		}, nil

	case "task_progress":
		usage := parseTaskUsage(data["usage"])
		return &TaskProgressMessage{
			SystemMessage:   base,
			TaskID:          stringField(data, "task_id"),
			Description:     stringField(data, "description"),
			Usage:           usage,
			UUID:            stringField(data, "uuid"),
			SessionID:       stringField(data, "session_id"),
			ToolUseID:       stringField(data, "tool_use_id"),
			LastToolName:    stringField(data, "last_tool_name"),
			Summary:         stringField(data, "summary"),
			SubagentType:    stringField(data, "subagent_type"),
			TaskDescription: stringField(data, "task_description"),
			Blocked:         boolField(data, "blocked"),
		}, nil

	case "mirror_error":
		msg := &MirrorErrorMessage{
			SystemMessage: base,
			Error:         stringField(data, "error"),
			UUID:          stringField(data, "uuid"),
			SessionID:     stringField(data, "session_id"),
		}
		if keyMap, ok := data["key"].(map[string]any); ok {
			msg.Key = &SessionKey{
				ProjectKey: stringField(keyMap, "project_key"),
				SessionID:  stringField(keyMap, "session_id"),
				Subpath:    stringField(keyMap, "subpath"),
			}
		}
		return msg, nil

	case "task_updated":
		msg := &TaskUpdatedMessage{
			SystemMessage: base,
			TaskID:        stringField(data, "task_id"),
			SessionID:     stringField(data, "session_id"),
			UUID:          stringField(data, "uuid"),
		}
		if patch, ok := data["patch"].(map[string]any); ok {
			msg.Patch = patch
			if s, ok := patch["status"].(string); ok {
				msg.Status = TaskUpdatedStatus(s)
			}
		}
		return msg, nil

	case "task_notification":
		var usage *TaskUsage
		if u := data["usage"]; u != nil {
			tu := parseTaskUsage(u)
			usage = &tu
		}
		return &TaskNotificationMessage{
			SystemMessage:   base,
			TaskID:          stringField(data, "task_id"),
			Status:          TaskNotificationStatus(stringField(data, "status")),
			OutputFile:      stringField(data, "output_file"),
			Summary:         stringField(data, "summary"),
			UUID:            stringField(data, "uuid"),
			SessionID:       stringField(data, "session_id"),
			ToolUseID:       stringField(data, "tool_use_id"),
			Usage:           usage,
			SubagentType:    stringField(data, "subagent_type"),
			TaskDescription: stringField(data, "task_description"),
		}, nil

	case "api_retry":
		msg := &ApiRetryMessage{
			SystemMessage: base,
			AttemptNumber: intField(data, "attempt_number"),
			MaxAttempts:   intField(data, "max_attempts"),
			DelayMs:       intField(data, "delay_ms"),
			ErrorMessage:  stringField(data, "error_message"),
		}
		if v, ok := data["error_status"]; ok {
			if status := intFromAny(v); status != 0 {
				msg.ErrorStatus = &status
			}
		}
		if errStr, ok := data["error"].(string); ok {
			msg.Error = ApiRetryError(errStr)
		}
		return msg, nil

	case "memory_recall":
		var paths []string
		if raw, ok := data["paths"].([]any); ok {
			paths = make([]string, 0, len(raw))
			for _, p := range raw {
				if s, ok := p.(string); ok {
					paths = append(paths, s)
				}
			}
		}
		return &MemoryRecallMessage{
			SystemMessage: base,
			Paths:         paths,
		}, nil

	case "elicitation_complete":
		var result map[string]any
		if r, ok := data["result"].(map[string]any); ok {
			result = r
		}
		return &ElicitationCompleteMessage{
			SystemMessage: base,
			RequestID:     stringField(data, "request_id"),
			ServerName:    stringField(data, "server_name"),
			Result:        result,
		}, nil

	case "hook_started", "hook_response":
		msg := &HookEventMessage{
			SystemMessage: base,
			HookEvent:     stringField(data, "hook_event"),
			HookID:        stringField(data, "hook_id"),
			HookName:      stringField(data, "hook_name"),
			Outcome:       stringField(data, "outcome"),
		}
		if output, ok := data["output"].(map[string]any); ok {
			msg.Output = output
		}
		if v, ok := data["exit_code"]; ok && v != nil {
			if fv, ok := v.(float64); ok {
				code := int(fv)
				msg.ExitCode = &code
			}
		}
		return msg, nil

	case "model_fallback":
		return &ModelFallbackMessage{
			SystemMessage: base,
			Trigger:       ModelFallbackTrigger(stringField(data, "trigger")),
			Model:         stringField(data, "model"),
			OriginalModel: stringField(data, "original_model"),
		}, nil

	case "worker_shutting_down":
		return &WorkerShuttingDownMessage{
			SystemMessage: base,
			Reason:        stringField(data, "reason"),
		}, nil

	case "permission_denied_advisory":
		return &PermissionDeniedAdvisoryMessage{
			SystemMessage: base,
			ToolName:      stringField(data, "tool_name"),
			DenialReason:  PermissionDeniedAdvisoryReason(stringField(data, "denial_reason")),
		}, nil

	default:
		return &base, nil
	}
}

func parseResultMessage(data map[string]any) (*ResultMessage, error) {
	msg := &ResultMessage{
		Subtype:        stringField(data, "subtype"),
		DurationMs:     intField(data, "duration_ms"),
		DurationAPIMs:  intField(data, "duration_api_ms"),
		IsError:        boolField(data, "is_error"),
		NumTurns:       intField(data, "num_turns"),
		SessionID:      stringField(data, "session_id"),
		StopReason:     stringField(data, "stop_reason"),
		TerminalReason: stringField(data, "terminal_reason"),
		Origin:         stringField(data, "origin"),
		RequestID:      stringField(data, "request_id"),
		Result:         stringField(data, "result"),
		Timestamp:      stringField(data, "timestamp"),
	}

	if errors, ok := data["errors"].([]any); ok {
		msg.Errors = errors
	}

	if sd, ok := data["stop_details"].(map[string]any); ok {
		msg.StopDetails = sd
	}

	if v, ok := data["api_error_status"]; ok {
		if status := intFromAny(v); status != 0 {
			msg.APIErrorStatus = &status
		}
	}

	if cost, ok := data["total_cost_usd"].(float64); ok {
		msg.TotalCostUSD = &cost
	}
	if usage, ok := data["usage"].(map[string]any); ok {
		msg.Usage = usage
	}
	msg.StructuredOutput = data["structured_output"]

	if dtu, ok := data["deferred_tool_use"].(map[string]any); ok {
		msg.DeferredToolUse = &DeferredToolUse{
			ToolUseID: stringField(dtu, "tool_use_id"),
			ToolName:  stringField(dtu, "tool_name"),
			ToolInput: mapField(dtu, "tool_input"),
		}
	}

	msg.RawData = data

	return msg, nil
}

func parseStreamEvent(data map[string]any) (*StreamEvent, error) {
	event, _ := data["event"].(map[string]any)
	return &StreamEvent{
		UUID:            stringField(data, "uuid"),
		SessionID:       stringField(data, "session_id"),
		Event:           event,
		ParentToolUseID: stringField(data, "parent_tool_use_id"),
	}, nil
}

func parseRateLimitEvent(data map[string]any) (*RateLimitEvent, error) {
	event := &RateLimitEvent{
		Type:      stringField(data, "type"),
		UUID:      stringField(data, "uuid"),
		SessionID: stringField(data, "session_id"),
	}

	// Extract rate_limit_info from nested map if present
	if rl, exists := data["rate_limit_info"]; exists {
		if infoMap, ok := rl.(map[string]any); ok {
			event.RateLimitInfo = parseRateLimitInfo(infoMap)
		} else if rl != nil {
			return nil, &MessageParseError{
				SDKError: SDKError{Message: "rate_limit_info has wrong type"},
				Data:     data,
			}
		}
	}

	return event, nil
}

func parseRateLimitInfo(m map[string]any) RateLimitInfo {
	info := RateLimitInfo{
		Status: RateLimitStatus(stringField(m, "status")),
	}
	info.ResetsAt = optionalStringField(m, "resets_at")
	info.RateLimitType = optionalStringField(m, "rate_limit_type")
	info.OverageStatus = optionalStringField(m, "overage_status")
	info.OverageResetsAt = optionalStringField(m, "overage_resets_at")
	info.OverageDisabledReason = optionalStringField(m, "overage_disabled_reason")
	info.ErrorCode = optionalStringField(m, "error_code")
	if v, ok := m["utilization"].(float64); ok {
		info.Utilization = &v
	}
	if v, ok := m["can_user_purchase_credits"].(bool); ok {
		info.CanUserPurchaseCredits = &v
	}
	if v, ok := m["has_chargeable_saved_payment_method"].(bool); ok {
		info.HasChargeableSavedPaymentMethod = &v
	}
	return info
}

func parseContentBlocks(raw []any) ([]ContentBlock, error) {
	blocks := make([]ContentBlock, 0, len(raw))
	for _, item := range raw {
		blockMap, ok := item.(map[string]any)
		if !ok {
			return nil, &MessageParseError{
				SDKError: SDKError{
					Message: fmt.Sprintf("Invalid content block: expected object, got %T", item),
				},
			}
		}
		blockType, _ := blockMap["type"].(string)
		switch blockType {
		case "text":
			text, _ := blockMap["text"].(string)
			blocks = append(blocks, TextBlock{Text: text})
		case "thinking":
			thinking, _ := blockMap["thinking"].(string)
			signature, _ := blockMap["signature"].(string)
			blocks = append(blocks, ThinkingBlock{Thinking: thinking, Signature: signature})
		case "image", "document":
			blocks = append(blocks, Base64Block{Type: blockType, Source: parseBase64Source(blockMap)})
		case "tool_use":
			id, _ := blockMap["id"].(string)
			name, _ := blockMap["name"].(string)
			input, _ := blockMap["input"].(map[string]any)
			blocks = append(blocks, ToolUseBlock{ID: id, Name: name, Input: input})
		case "tool_result":
			toolUseID, _ := blockMap["tool_use_id"].(string)
			block := ToolResultBlock{ToolUseID: toolUseID, Content: blockMap["content"]}
			if isErr, ok := blockMap["is_error"].(bool); ok {
				block.IsError = &isErr
			}
			blocks = append(blocks, block)
		case "server_tool_use":
			id, _ := blockMap["id"].(string)
			name, _ := blockMap["name"].(string)
			input, _ := blockMap["input"].(map[string]any)
			blocks = append(blocks, ServerToolUseBlock{
				ID:    id,
				Name:  ServerToolName(name),
				Input: input,
			})
		case "advisor_tool_result", "server_tool_result":
			toolUseID, _ := blockMap["tool_use_id"].(string)
			content, _ := blockMap["content"].(map[string]any)
			blocks = append(blocks, ServerToolResultBlock{
				ToolUseID: toolUseID,
				Content:   content,
			})
		}
	}
	return blocks, nil
}

func parseBase64Source(blockMap map[string]any) Base64Source {
	source, _ := blockMap["source"].(map[string]any)
	if source == nil {
		return Base64Source{}
	}
	return Base64Source{
		Type:      stringField(source, "type"),
		MediaType: stringField(source, "media_type"),
		Data:      stringField(source, "data"),
	}
}

func parseTaskUsage(v any) TaskUsage {
	m, ok := v.(map[string]any)
	if !ok {
		return TaskUsage{}
	}
	return TaskUsage{
		TotalTokens: intFromAny(m["total_tokens"]),
		ToolUses:    intFromAny(m["tool_uses"]),
		DurationMs:  intFromAny(m["duration_ms"]),
	}
}

// Helper functions for extracting typed fields from maps.

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func intField(m map[string]any, key string) int {
	return intFromAny(m[key])
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

func boolField(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func optionalStringField(m map[string]any, key string) *string {
	v, ok := m[key].(string)
	if !ok {
		return nil
	}
	return &v
}
