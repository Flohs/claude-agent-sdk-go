package claude

import "testing"

func TestEscapeSlashCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/ add tests", " / add tests"},
		{"/  add tests", " /  add tests"},    // multiple spaces
		{"/\tadd tests", " /\tadd tests"},    // tab after slash
		{"/command", "/command"},              // slash command — NOT escaped
		{"hello / world", "hello / world"},   // slash not at start
		{"", ""},
		{"normal text", "normal text"},
	}
	for _, tt := range tests {
		got := escapeSlashCommand(tt.input)
		if got != tt.want {
			t.Errorf("escapeSlashCommand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
