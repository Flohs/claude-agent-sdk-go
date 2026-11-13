package claude

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsValue_FileRelativeToCwdWithBOM(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "s.json"), []byte("\xef\xbb\xbf{\"env\":{\"FOO\":\"bar\"}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	enabled := true
	opts := &Options{Settings: "s.json", Cwd: dir, Sandbox: &SandboxSettings{Enabled: &enabled}}
	tr := &SubprocessTransport{options: opts, cwd: opts.Cwd}
	value, err := tr.settingsValue()
	if err != nil {
		t.Fatalf("settingsValue: %v", err)
	}
	for _, want := range []string{`"FOO":"bar"`, `"enabled":true`} {
		if !strings.Contains(value, want) {
			t.Errorf("expected %s in %s", want, value)
		}
	}
}

func TestSettingsValue_AbsolutePathIgnoresCwd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.json")
	if err := os.WriteFile(path, []byte(`{"model":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := &Options{Settings: path, Cwd: t.TempDir(), WorkflowSizeGuideline: WorkflowSizeGuidelineSmall}
	tr := &SubprocessTransport{options: opts, cwd: opts.Cwd}
	value, err := tr.settingsValue()
	if err != nil {
		t.Fatalf("settingsValue: %v", err)
	}
	if !strings.Contains(value, `"model":"x"`) {
		t.Errorf("expected file settings in %s", value)
	}
}

func TestSettingsValue_LoadErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "list.json"), []byte(`[1]`), 0o600); err != nil {
		t.Fatal(err)
	}
	enabled := true
	for name, settings := range map[string]string{
		"missing file": "nope.json",
		"bad inline":   "{not json}",
		"not object":   "list.json",
	} {
		t.Run(name, func(t *testing.T) {
			opts := &Options{Settings: settings, Cwd: dir, Sandbox: &SandboxSettings{Enabled: &enabled}}
			tr := &SubprocessTransport{options: opts, cwd: opts.Cwd}
			_, err := tr.settingsValue()
			var connErr *ConnectionError
			if !errors.As(err, &connErr) || !strings.Contains(err.Error(), "Failed to load settings") {
				t.Fatalf("want *ConnectionError loading settings, got %v", err)
			}
		})
	}
}

func TestSettingsValue_PathAloneIsPassedThrough(t *testing.T) {
	tr := &SubprocessTransport{options: &Options{Settings: "nope.json"}}
	value, err := tr.settingsValue()
	if err != nil || value != "nope.json" {
		t.Fatalf("got %q, %v; want the path passed through for the CLI to read", value, err)
	}
}

func TestConnect_FailsOnUnloadableSettings(t *testing.T) {
	t.Setenv("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK", "1")
	enabled := true
	opts := &Options{Settings: "nope.json", Cwd: t.TempDir(), Sandbox: &SandboxSettings{Enabled: &enabled}}
	tr := &SubprocessTransport{options: opts, cwd: opts.Cwd, cliPath: "/nonexistent/claude"}
	err := tr.Connect(t.Context())
	if err == nil || !strings.Contains(err.Error(), "Failed to load settings") {
		t.Fatalf("Connect error = %v, want a settings load error", err)
	}
	if tr.cmd != nil {
		t.Fatal("no process may be started with unloadable settings")
	}
}
