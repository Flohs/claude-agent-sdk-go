package claude

import (
	"context"
	"encoding/json"
	"testing"
)

// --- Content block constructor tests ---

func TestNewTextContent(t *testing.T) {
	block := NewTextContent("hello world")

	if block["type"] != "text" {
		t.Fatalf("expected type 'text', got %v", block["type"])
	}
	if block["text"] != "hello world" {
		t.Fatalf("expected text 'hello world', got %v", block["text"])
	}
	if len(block) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(block))
	}
}

func TestNewBase64Content(t *testing.T) {
	tests := []struct {
		name          string
		mediaType     string
		data          string
		wantBlockType string
	}{
		{"image/png", "image/png", "aW1hZ2VkYXRh", "image"},
		{"image/jpeg", "image/jpeg", "aW1hZ2VkYXRh", "image"},
		{"image/gif", "image/gif", "aW1hZ2VkYXRh", "image"},
		{"image/webp", "image/webp", "aW1hZ2VkYXRh", "image"},
		{"application/pdf", "application/pdf", "cGRmZGF0YQ==", "document"},
		{"text/plain", "text/plain", "dGV4dA==", "document"},
		{"text/html", "text/html", "dGV4dA==", "document"},
		{"text/csv", "text/csv", "dGV4dA==", "document"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := NewBase64Content(tt.mediaType, tt.data)

			if block["type"] != tt.wantBlockType {
				t.Fatalf("expected type %q, got %v", tt.wantBlockType, block["type"])
			}

			source, ok := block["source"].(map[string]any)
			if !ok {
				t.Fatalf("expected source to be map[string]any, got %T", block["source"])
			}
			if source["type"] != "base64" {
				t.Fatalf("expected source type 'base64', got %v", source["type"])
			}
			if source["media_type"] != tt.mediaType {
				t.Fatalf("expected media_type %q, got %v", tt.mediaType, source["media_type"])
			}
			if source["data"] != tt.data {
				t.Fatalf("expected data %q, got %v", tt.data, source["data"])
			}
		})
	}
}

// --- Input validation tests ---

func TestNewBase64Content_EmptyMediaType_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty mediaType")
		}
	}()
	NewBase64Content("", "aW1hZ2VkYXRh")
}

func TestNewBase64Content_EmptyData_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty base64Data")
		}
	}()
	NewBase64Content("image/png", "")
}

func TestSendQueryWithContent_InvalidContentType(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})
	c := &Client{
		options:   &Options{},
		transport: mt,
		q:         q,
	}

	err := c.SendQueryWithContent(context.TODO(), 42)
	if err == nil {
		t.Fatal("expected error for invalid content type")
	}
	if _, ok := err.(*SDKError); !ok {
		t.Fatalf("expected SDKError, got %T", err)
	}
}

func TestSendQueryWithContent_NilContent(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})
	c := &Client{
		options:   &Options{},
		transport: mt,
		q:         q,
	}

	err := c.SendQueryWithContent(context.TODO(), nil)
	if err == nil {
		t.Fatal("expected error for nil content")
	}
}

// --- Content block JSON serialization tests ---

func TestNewTextContent_JSON(t *testing.T) {
	block := NewTextContent("hello")
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if parsed["type"] != "text" || parsed["text"] != "hello" {
		t.Fatalf("unexpected JSON round-trip: %v", parsed)
	}
}

func TestNewBase64Content_JSON(t *testing.T) {
	tests := []struct {
		name      string
		mediaType string
		wantType  string
	}{
		{"image", "image/jpeg", "image"},
		{"document", "application/pdf", "document"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := NewBase64Content(tt.mediaType, "abc123")
			data, err := json.Marshal(block)
			if err != nil {
				t.Fatalf("unexpected marshal error: %v", err)
			}

			var parsed map[string]any
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unexpected unmarshal error: %v", err)
			}
			if parsed["type"] != tt.wantType {
				t.Fatalf("expected type %q, got %v", tt.wantType, parsed["type"])
			}
			source := parsed["source"].(map[string]any)
			if source["media_type"] != tt.mediaType {
				t.Fatalf("expected media_type %q, got %v", tt.mediaType, source["media_type"])
			}
		})
	}
}

// --- Multimodal content slice tests ---

func TestMultimodalContentSlice_JSON(t *testing.T) {
	content := []any{
		NewTextContent("Describe this image"),
		NewBase64Content("image/png", "aW1hZ2VkYXRh"),
	}

	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var parsed []map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(parsed))
	}
	if parsed[0]["type"] != "text" {
		t.Fatalf("expected first block type 'text', got %v", parsed[0]["type"])
	}
	if parsed[1]["type"] != "image" {
		t.Fatalf("expected second block type 'image', got %v", parsed[1]["type"])
	}
}

func TestMixedContentSlice_WithDocument(t *testing.T) {
	content := []any{
		NewTextContent("Summarize this PDF"),
		NewBase64Content("application/pdf", "cGRmZGF0YQ=="),
	}

	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var parsed []map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(parsed))
	}
	if parsed[0]["type"] != "text" {
		t.Fatalf("expected first block type 'text', got %v", parsed[0]["type"])
	}
	if parsed[1]["type"] != "document" {
		t.Fatalf("expected second block type 'document', got %v", parsed[1]["type"])
	}
}

// --- Typed struct tests ---

func TestBase64Block_ImplementsContentBlock(t *testing.T) {
	var _ ContentBlock = Base64Block{}
}

func TestBase64Block_JSON(t *testing.T) {
	tests := []struct {
		name      string
		blockType string
		mediaType string
	}{
		{"image", "image", "image/png"},
		{"document", "document", "application/pdf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := Base64Block{
				Type: tt.blockType,
				Source: Base64Source{
					Type:      "base64",
					MediaType: tt.mediaType,
					Data:      "dGVzdA==",
				},
			}

			data, err := json.Marshal(block)
			if err != nil {
				t.Fatalf("unexpected marshal error: %v", err)
			}

			var parsed map[string]any
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unexpected unmarshal error: %v", err)
			}
			if parsed["type"] != tt.blockType {
				t.Fatalf("expected type %q, got %v", tt.blockType, parsed["type"])
			}
			source := parsed["source"].(map[string]any)
			if source["media_type"] != tt.mediaType {
				t.Fatalf("expected media_type %q, got %v", tt.mediaType, source["media_type"])
			}
		})
	}
}

// --- SendQueryWithContent message format test ---

func TestSendQueryWithContent_StringContent(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})
	c := &Client{
		options:   &Options{},
		transport: mt,
		q:         q,
	}

	err := c.SendQueryWithContent(context.TODO(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()
	if len(mt.written) != 1 {
		t.Fatalf("expected 1 written message, got %d", len(mt.written))
	}

	var msg map[string]any
	if err := json.Unmarshal([]byte(mt.written[0]), &msg); err != nil {
		t.Fatalf("failed to parse written message: %v", err)
	}

	if msg["type"] != "user" {
		t.Fatalf("expected type 'user', got %v", msg["type"])
	}
	message := msg["message"].(map[string]any)
	if message["role"] != "user" {
		t.Fatalf("expected role 'user', got %v", message["role"])
	}
	if message["content"] != "hello" {
		t.Fatalf("expected content 'hello', got %v", message["content"])
	}
}

func TestSendQueryWithContent_MultimodalContent(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})
	c := &Client{
		options:   &Options{},
		transport: mt,
		q:         q,
	}

	content := []any{
		NewTextContent("Describe this image"),
		NewBase64Content("image/png", "aWJhc2U2NA=="),
	}

	err := c.SendQueryWithContent(context.TODO(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()
	if len(mt.written) != 1 {
		t.Fatalf("expected 1 written message, got %d", len(mt.written))
	}

	var msg map[string]any
	if err := json.Unmarshal([]byte(mt.written[0]), &msg); err != nil {
		t.Fatalf("failed to parse written message: %v", err)
	}

	message := msg["message"].(map[string]any)
	blocks, ok := message["content"].([]any)
	if !ok {
		t.Fatalf("expected content to be []any, got %T", message["content"])
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(blocks))
	}

	textBlock := blocks[0].(map[string]any)
	if textBlock["type"] != "text" {
		t.Fatalf("expected first block type 'text', got %v", textBlock["type"])
	}

	imageBlock := blocks[1].(map[string]any)
	if imageBlock["type"] != "image" {
		t.Fatalf("expected second block type 'image', got %v", imageBlock["type"])
	}
}

func TestSendQueryWithContent_DocumentContent(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})
	c := &Client{
		options:   &Options{},
		transport: mt,
		q:         q,
	}

	content := []any{
		NewTextContent("Summarize this document"),
		NewBase64Content("application/pdf", "cGRmZGF0YQ=="),
	}

	err := c.SendQueryWithContent(context.TODO(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()

	var msg map[string]any
	if err := json.Unmarshal([]byte(mt.written[0]), &msg); err != nil {
		t.Fatalf("failed to parse written message: %v", err)
	}

	message := msg["message"].(map[string]any)
	blocks := message["content"].([]any)
	docBlock := blocks[1].(map[string]any)
	if docBlock["type"] != "document" {
		t.Fatalf("expected block type 'document', got %v", docBlock["type"])
	}
	source := docBlock["source"].(map[string]any)
	if source["media_type"] != "application/pdf" {
		t.Fatalf("expected media_type 'application/pdf', got %v", source["media_type"])
	}
}

func TestSendQueryWithContent_NotConnected(t *testing.T) {
	c := &Client{options: &Options{}}

	err := c.SendQueryWithContent(context.TODO(), "hello")
	if err == nil {
		t.Fatal("expected error for not connected client")
	}
	if _, ok := err.(*ConnectionError); !ok {
		t.Fatalf("expected ConnectionError, got %T", err)
	}
}

func TestSendQuery_DelegatesToSendQueryWithContent(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})
	c := &Client{
		options:   &Options{},
		transport: mt,
		q:         q,
	}

	err := c.SendQuery(context.TODO(), "test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()

	var msg map[string]any
	if err := json.Unmarshal([]byte(mt.written[0]), &msg); err != nil {
		t.Fatalf("failed to parse written message: %v", err)
	}

	message := msg["message"].(map[string]any)
	if message["content"] != "test prompt" {
		t.Fatalf("expected content 'test prompt', got %v", message["content"])
	}
}

// --- Message parser tests for image/document blocks ---

func TestParseMessage_AssistantMessage_Base64Block(t *testing.T) {
	tests := []struct {
		name      string
		blockType string
		mediaType string
		data      string
	}{
		{"image", "image", "image/png", "aWJhc2U2NA=="},
		{"document", "document", "application/pdf", "cGRmZGF0YQ=="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"model": "test",
					"content": []any{
						map[string]any{
							"type": tt.blockType,
							"source": map[string]any{
								"type":       "base64",
								"media_type": tt.mediaType,
								"data":       tt.data,
							},
						},
					},
				},
			}

			msg, err := ParseMessage(data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			asst := msg.(*AssistantMessage)
			if len(asst.Content) != 1 {
				t.Fatalf("expected 1 content block, got %d", len(asst.Content))
			}

			block, ok := asst.Content[0].(Base64Block)
			if !ok {
				t.Fatalf("expected Base64Block, got %T", asst.Content[0])
			}
			if block.Type != tt.blockType {
				t.Fatalf("expected type %q, got %s", tt.blockType, block.Type)
			}
			if block.Source.MediaType != tt.mediaType {
				t.Fatalf("expected media_type %q, got %s", tt.mediaType, block.Source.MediaType)
			}
			if block.Source.Data != tt.data {
				t.Fatalf("expected data %q, got %s", tt.data, block.Source.Data)
			}
		})
	}
}

func TestParseMessage_Base64Block_MissingSource(t *testing.T) {
	data := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"model": "test",
			"content": []any{
				map[string]any{"type": "image"},
			},
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	asst := msg.(*AssistantMessage)
	imgBlock := asst.Content[0].(Base64Block)
	if imgBlock.Source.Type != "" {
		t.Fatalf("expected empty source type for missing source, got %s", imgBlock.Source.Type)
	}
}
