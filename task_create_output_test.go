package claude

import (
	"encoding/json"
	"testing"
)

func TestTaskCreateOutput_Description(t *testing.T) {
	var out TaskCreateOutput
	if err := json.Unmarshal([]byte(`{"task":{"id":"1","subject":"Fix bug","description":"Fix the parser bug"}}`), &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Task.ID != "1" || out.Task.Subject != "Fix bug" || out.Task.Description != "Fix the parser bug" {
		t.Errorf("Task = %+v", out.Task)
	}

	var old TaskCreateOutput
	if err := json.Unmarshal([]byte(`{"task":{"id":"2","subject":"s"}}`), &old); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if old.Task.Description != "" {
		t.Errorf("Description = %q, want empty from older CLIs", old.Task.Description)
	}
	b, err := json.Marshal(old)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got, want := string(b), `{"task":{"id":"2","subject":"s"}}`; got != want {
		t.Errorf("Marshal = %s, want %s", got, want)
	}
}
