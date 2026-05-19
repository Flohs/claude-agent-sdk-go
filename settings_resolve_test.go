package claude

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile is a small test helper that writes content to a path, creating
// parent directories. Mirrors the style used in sessions_test.go.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestResolveSettings_NilOpts(t *testing.T) {
	// With nil opts, ConfigDir/Cwd fall back to defaults; no panic.
	r, err := ResolveSettings(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil ResolvedSettings")
	}
	if r.Merged == nil || r.BySource == nil {
		t.Fatal("expected initialized maps even when no sources present")
	}
}

func TestResolveSettings_EmptyDirs(t *testing.T) {
	// All sources absent: returns initialized but empty result, no error.
	r, err := ResolveSettings(&ResolveSettingsOptions{
		ConfigDir: t.TempDir(),
		Cwd:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Merged) != 0 {
		t.Errorf("Merged should be empty, got %v", r.Merged)
	}
	if len(r.BySource) != 0 {
		t.Errorf("BySource should be empty, got %v", r.BySource)
	}
}

func TestResolveSettings_UserOnly(t *testing.T) {
	cfg := t.TempDir()
	writeFile(t, filepath.Join(cfg, "settings.json"), `{"theme": "dark", "model": "opus"}`)

	r, err := ResolveSettings(&ResolveSettingsOptions{
		ConfigDir: cfg,
		Cwd:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Merged["theme"] != "dark" {
		t.Errorf("Merged[theme] = %v, want dark", r.Merged["theme"])
	}
	if r.Merged["model"] != "opus" {
		t.Errorf("Merged[model] = %v, want opus", r.Merged["model"])
	}
	if _, ok := r.BySource["user"]; !ok {
		t.Error("BySource[user] missing")
	}
	if _, ok := r.BySource["project"]; ok {
		t.Error("BySource[project] should not be present")
	}
}

func TestResolveSettings_Cascade(t *testing.T) {
	// Precedence (low → high): user → project → local → managed.
	// Each layer overrides matching top-level keys.
	cfg := t.TempDir()
	cwd := t.TempDir()

	writeFile(t, filepath.Join(cfg, "settings.json"), `{"theme": "user", "from_user": true}`)
	writeFile(t, filepath.Join(cwd, ".claude", "settings.json"), `{"theme": "project", "from_project": true}`)
	writeFile(t, filepath.Join(cwd, ".claude", "settings.local.json"), `{"theme": "local", "from_local": true}`)

	r, err := ResolveSettings(&ResolveSettingsOptions{
		ConfigDir:       cfg,
		Cwd:             cwd,
		ManagedSettings: `{"theme": "managed", "from_managed": true}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Highest precedence wins for the conflicting key.
	if r.Merged["theme"] != "managed" {
		t.Errorf("Merged[theme] = %v, want managed", r.Merged["theme"])
	}
	// Non-conflicting keys from each layer are preserved.
	for _, k := range []string{"from_user", "from_project", "from_local", "from_managed"} {
		if r.Merged[k] != true {
			t.Errorf("Merged[%s] = %v, want true", k, r.Merged[k])
		}
	}
	// BySource keeps each layer's raw values intact (theme should still read
	// "user" in the user-source map even though merged is "managed").
	if r.BySource["user"]["theme"] != "user" {
		t.Errorf("BySource[user][theme] = %v, want user", r.BySource["user"]["theme"])
	}
	if r.BySource["project"]["theme"] != "project" {
		t.Errorf("BySource[project][theme] = %v, want project", r.BySource["project"]["theme"])
	}
	if r.BySource["local"]["theme"] != "local" {
		t.Errorf("BySource[local][theme] = %v, want local", r.BySource["local"]["theme"])
	}
	if r.BySource["managed"]["theme"] != "managed" {
		t.Errorf("BySource[managed][theme] = %v, want managed", r.BySource["managed"]["theme"])
	}
}

func TestResolveSettings_CorruptUserFileReturnsError(t *testing.T) {
	// Regression: an existing-but-corrupt settings file used to be silently
	// skipped. After the issue #204 fix it must surface as an error so the
	// caller can act on it.
	cfg := t.TempDir()
	writeFile(t, filepath.Join(cfg, "settings.json"), `{ not valid json `)

	_, err := ResolveSettings(&ResolveSettingsOptions{
		ConfigDir: cfg,
		Cwd:       t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for corrupt user settings.json, got nil")
	}
	if !strings.Contains(err.Error(), "user settings") {
		t.Errorf("error should identify the source (user), got: %v", err)
	}
}

func TestResolveSettings_CorruptProjectFileReturnsError(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, ".claude", "settings.json"), `[invalid]`)

	_, err := ResolveSettings(&ResolveSettingsOptions{
		ConfigDir: t.TempDir(),
		Cwd:       cwd,
	})
	if err == nil {
		t.Fatal("expected error for corrupt project settings.json, got nil")
	}
	if !strings.Contains(err.Error(), "project settings") {
		t.Errorf("error should identify the source (project), got: %v", err)
	}
}

func TestResolveSettings_InvalidManagedJSONReturnsError(t *testing.T) {
	// Caller explicitly opted in by passing ManagedSettings — silently
	// dropping the input would mask a bug in whatever produced the JSON.
	_, err := ResolveSettings(&ResolveSettingsOptions{
		ConfigDir:       t.TempDir(),
		Cwd:             t.TempDir(),
		ManagedSettings: `{"unterminated":`,
	})
	if err == nil {
		t.Fatal("expected error for invalid ManagedSettings JSON, got nil")
	}
	if !strings.Contains(err.Error(), "managed settings") {
		t.Errorf("error should mention managed settings, got: %v", err)
	}
	// Underlying json.SyntaxError should be wrapped, so errors.As can reach it.
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Errorf("expected wrapped *json.SyntaxError, got: %v", err)
	}
}

func TestResolveSettings_EmptyManagedSettingsIsNotAnError(t *testing.T) {
	// Empty string means "no managed source provided" — the dominant case.
	r, err := ResolveSettings(&ResolveSettingsOptions{
		ConfigDir:       t.TempDir(),
		Cwd:             t.TempDir(),
		ManagedSettings: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := r.BySource["managed"]; ok {
		t.Error("BySource[managed] should be absent when ManagedSettings is empty")
	}
}

func TestResolveSettings_ShallowMerge(t *testing.T) {
	// Top-level keys are replaced wholesale; nested objects are NOT deep-merged.
	// This documents the contract called out in the GoDoc.
	cfg := t.TempDir()
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cfg, "settings.json"), `{"nested": {"a": 1, "b": 2}}`)
	writeFile(t, filepath.Join(cwd, ".claude", "settings.json"), `{"nested": {"c": 3}}`)

	r, err := ResolveSettings(&ResolveSettingsOptions{
		ConfigDir: cfg,
		Cwd:       cwd,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	nested, ok := r.Merged["nested"].(map[string]any)
	if !ok {
		t.Fatalf("Merged[nested] is not a map: %T", r.Merged["nested"])
	}
	// Project replaces user's nested object entirely — user's "a"/"b" are gone.
	if _, hasA := nested["a"]; hasA {
		t.Errorf("expected shallow merge to drop user keys; got nested[a] still present: %v", nested)
	}
	if nested["c"] != float64(3) {
		t.Errorf("nested[c] = %v, want 3", nested["c"])
	}
}
