package claude

import (
	"reflect"
	"testing"
)

func TestParseSessionWritesLostMessage(t *testing.T) {
	data := map[string]any{
		"type":                   "system",
		"subtype":                "session_writes_lost",
		"error_code":             "ENOSPC",
		"lost":                   []any{"transcript", "global_config"},
		"transcript_session_ids": []any{"sess-old", "sess-1"},
		"uuid":                   "u-1",
		"session_id":             "sess-1",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	m, ok := msg.(*SessionWritesLostMessage)
	if !ok {
		t.Fatalf("got %T, want *SessionWritesLostMessage", msg)
	}
	if m.Subtype != "session_writes_lost" {
		t.Errorf("Subtype = %q", m.Subtype)
	}
	if m.ErrorCode != SessionWritesLostErrorENOSPC {
		t.Errorf("ErrorCode = %q, want ENOSPC", m.ErrorCode)
	}
	if want := []SessionWritesLostKind{SessionWritesLostTranscript, SessionWritesLostGlobalConfig}; !reflect.DeepEqual(m.Lost, want) {
		t.Errorf("Lost = %v, want %v", m.Lost, want)
	}
	if want := []string{"sess-old", "sess-1"}; !reflect.DeepEqual(m.TranscriptSessionIDs, want) {
		t.Errorf("TranscriptSessionIDs = %v, want %v", m.TranscriptSessionIDs, want)
	}
	if m.UUID != "u-1" || m.SessionID != "sess-1" {
		t.Errorf("UUID/SessionID = %q/%q", m.UUID, m.SessionID)
	}
}

func TestParseSessionWritesLostMessage_GlobalConfigOnly(t *testing.T) {
	msg, err := ParseMessage(map[string]any{
		"type":       "system",
		"subtype":    "session_writes_lost",
		"error_code": "EDQUOT",
		"lost":       []any{"global_config"},
		"session_id": "s",
	})
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	m, ok := msg.(*SessionWritesLostMessage)
	if !ok {
		t.Fatalf("got %T, want *SessionWritesLostMessage", msg)
	}
	if m.ErrorCode != SessionWritesLostErrorEDQUOT {
		t.Errorf("ErrorCode = %q", m.ErrorCode)
	}
	if m.TranscriptSessionIDs != nil {
		t.Errorf("TranscriptSessionIDs = %v, want nil", m.TranscriptSessionIDs)
	}
	if len(m.Lost) != 1 || m.Lost[0] != SessionWritesLostGlobalConfig {
		t.Errorf("Lost = %v", m.Lost)
	}
}
