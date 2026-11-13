package claude

import (
	"strings"
	"testing"
)

func TestClaudeVersionWarnings(t *testing.T) {
	tests := []struct {
		name     string
		reported any
		verbatim bool
		want     []string // substrings, one per expected warning, in order
	}{
		{name: "supported", reported: "2.1.323 (Claude Code)"},
		{name: "supported verbatim", reported: "2.1.323", verbatim: true},
		{name: "missing", reported: nil, want: []string{"too old to report its version"}},
		{name: "not a string", reported: 2.1, want: []string{"too old to report its version"}},
		{name: "unparseable", reported: "dev", want: []string{"too old to report its version"}},
		{name: "below minimum", reported: "2.0.5", want: []string{"version 2.0.5 at /bin/claude is unsupported"}},
		{
			name: "missing verbatim", reported: nil, verbatim: true,
			want: []string{"too old to report its version", "Claude Code version unknown at /bin/claude ignores it"},
		},
		{
			name: "verbatim too old", reported: "2.1.200", verbatim: true,
			want: []string{"Claude Code version 2.1.200 at /bin/claude ignores it"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := claudeVersionWarnings("/bin/claude", &Options{VerbatimPrompts: tt.verbatim}, tt.reported)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d warnings %q, want %d", len(got), got, len(tt.want))
			}
			for i, w := range tt.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("warning %d = %q, want it to contain %q", i, got[i], w)
				}
			}
		})
	}
}

func TestCheckClaudeVersionOnlyFirstInit(t *testing.T) {
	tr := &SubprocessTransport{options: &Options{}, cliPath: "/bin/claude"}

	tr.checkClaudeVersion(map[string]any{"type": "assistant"})
	if tr.versionChecked {
		t.Fatal("a non-init message must not count as the version check")
	}
	tr.checkClaudeVersion(map[string]any{"type": "system", "subtype": "status"})
	if tr.versionChecked {
		t.Fatal("a non-init system message must not count as the version check")
	}
	tr.checkClaudeVersion(map[string]any{"type": "system", "subtype": "init", "claude_code_version": "2.1.323"})
	if !tr.versionChecked {
		t.Fatal("the first init message must mark the version as checked")
	}
}

func TestConnectSkipVersionCheckEnv(t *testing.T) {
	t.Setenv("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK", "1")
	tr := &SubprocessTransport{options: &Options{}, cliPath: "/nonexistent/claude"}
	_ = tr.Connect(t.Context())
	if !tr.versionChecked {
		t.Fatal("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK must turn the init-message check off")
	}
}
