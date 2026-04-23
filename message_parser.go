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
	}

	if tr, ok := data["tool_use_result"].(map[string]any); ok {
		msg.ToolUseResult = tr
	}

	message, ok := data["message"].(map[string]any)
	if !ok {
		return nil, &MessageParseError{
			SDKError: SDKError{Message: "Missing 'message' field in user message"},
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
	message, ok := data["message"].(map[string]any)
	if !ok {
		return nil, &MessageParseError{
			SDKError: SDKError{Message: "Missing 'message' field in assistant message"},
			Data:     data,
		}
	}

	contentRaw, ok := message["content"].([]any)
	if !ok {
		return nil, &MessageParseError{
			SDKError: SDKError{Message: "Missing 'content' field in assistant message"},
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
	}

	if errStr := stringField(data, "error"); errStr != "" {
		msg.Error = AssistantMessageError(errStr)
	}

	if usage, ok := message["usage"].(map[string]any); ok {
		msg.Usage = usage
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
		Subtype: subtype,
		Data:    data,
	}

	switch subtype {
	case "task_started":
		return &TaskStartedMessage{
			SystemMessage: base,
			TaskID:        stringField(data, "task_id"),
			Description:   stringField(data, "description"),
			UUID:          stringField(data, "uuid"),
			SessionID:     stringField(data, "session_id"),
			ToolUseID:     stringField(data, "tool_use_id"),
			TaskType:      stringField(data, "task_type"),
		}, nil

	case "task_progress":
		usage := parseTaskUsage(data["usage"])
		return &TaskProgressMessage{
			SystemMessage: base,
			TaskID:        stringField(data, "task_id"),
			Description:   stringField(data, "description"),
			Usage:         usage,
			UUID:          stringField(data, "uuid"),
			SessionID:     stringField(data, "session_id"),
			ToolUseID:     stringField(data, "tool_use_id"),
			LastToolName:  stringField(data, "last_tool_name"),
			Summary:       stringField(data, "summary"),
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

	case "task_notification":
		var usage *TaskUsage
		if u := data["usage"]; u != nil {
			tu := parseTaskUsage(u)
			usage = &tu
		}
		return &TaskNotificationMessage{
			SystemMessage: base,
			TaskID:        stringField(data, "task_id"),
			Status:        TaskNotificationStatus(stringField(data, "status")),
			OutputFile:    stringField(data, "output_file"),
			Summary:       stringField(data, "summary"),
			UUID:          stringField(data, "uuid"),
			SessionID:     stringField(data, "session_id"),
			ToolUseID:     stringField(data, "tool_use_id"),
			Usage:         usage,
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
		Result:         stringField(data, "result"),
	}

	if errors, ok := data["errors"].([]any); ok {
		msg.Errors = errors
	}

	if cost, ok := data["total_cost_usd"].(float64); ok {
		msg.TotalCostUSD = &cost
	}
	if usage, ok := data["usage"].(map[string]any); ok {
		msg.Usage = usage
	}
	msg.StructuredOutput = data["structured_output"]

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
	if infoMap, ok := data["rate_limit_info"].(map[string]any); ok {
		event.RateLimitInfo = parseRateLimitInfo(infoMap)
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
	if v, ok := m["utilization"].(float64); ok {
		info.Utilization = &v
	}
	return info
}

func parseContentBlocks(raw []any) ([]ContentBlock, error) {
	blocks := make([]ContentBlock, 0, len(raw))
	for _, item := range raw {
		blockMap, ok := item.(map[string]any)
		if !ok {
			continue
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
