package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ResolveSettingsOptions configures [ResolveSettings].
type ResolveSettingsOptions struct {
	// Cwd is the working directory used to locate project-scoped and
	// local-scoped settings files. Defaults to the current directory when empty.
	Cwd string
	// ConfigDir overrides the Claude config directory (default ~/.claude).
	// Equivalent to setting the CLAUDE_CONFIG_DIR environment variable.
	ConfigDir string
	// ManagedSettings is an inline JSON string of managed/policy-tier
	// settings. When set, these are applied with the highest precedence,
	// mirroring the CLI's IT-controlled managed source. Must be a valid JSON
	// object string or empty.
	ManagedSettings string
}

// ResolvedSettings holds the effective merged settings and per-source raw data.
type ResolvedSettings struct {
	// Merged is the fully merged settings map. Later sources (higher
	// precedence) overwrite keys from earlier sources. The merge is a
	// shallow top-level merge: nested objects are replaced, not deep-merged.
	Merged map[string]any
	// BySource holds the raw settings read from each source before merging.
	// Keys are "user", "project", "local", and "managed" when present.
	BySource map[string]map[string]any
}

// ResolveSettings inspects the effective merged settings for a session without
// spawning the Claude CLI subprocess. It reads the standard settings cascade
// (user → project → local → managed) from disk, merges them in precedence
// order, and returns the result.
//
// Settings are read from:
//   - User:    <configDir>/settings.json
//   - Project: <cwd>/.claude/settings.json
//   - Local:   <cwd>/.claude/settings.local.json
//   - Managed: opts.ManagedSettings (inline JSON, highest precedence)
//
// Platform-specific MDM sources (macOS plist, Windows HKLM/HKCU registry)
// are not read by this implementation — pass ManagedSettings explicitly if
// you need to simulate a managed policy environment.
//
// NOTE: This is an alpha feature. The settings schema and merge semantics may
// change in future releases as the CLI evolves.
func ResolveSettings(opts *ResolveSettingsOptions) (*ResolvedSettings, error) {
	if opts == nil {
		opts = &ResolveSettingsOptions{}
	}

	configDir := opts.ConfigDir
	if configDir == "" {
		configDir = getClaudeConfigHomeDir()
	}

	cwd := opts.Cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "."
		}
	}

	result := &ResolvedSettings{
		Merged:   make(map[string]any),
		BySource: make(map[string]map[string]any),
	}

	// Load sources in ascending precedence order; each layer overwrites
	// the previous for top-level keys (shallow merge).
	sources := []struct {
		name string
		path string
	}{
		{"user", filepath.Join(configDir, "settings.json")},
		{"project", filepath.Join(cwd, ".claude", "settings.json")},
		{"local", filepath.Join(cwd, ".claude", "settings.local.json")},
	}

	for _, src := range sources {
		m, err := readSettingsFile(src.path)
		if err != nil {
			// File exists but is unreadable or contains invalid JSON.
			// Surface this — a corrupt settings file is a real user-facing
			// bug, not something to silently skip. readSettingsFile returns
			// (nil, nil) for ENOENT, which is the path that falls through.
			return nil, fmt.Errorf("reading %s settings (%s): %w", src.name, src.path, err)
		}
		if m == nil {
			continue
		}
		result.BySource[src.name] = m
		for k, v := range m {
			result.Merged[k] = v
		}
	}

	// Managed settings have the highest precedence. Parse errors here are
	// returned because the caller explicitly opted in by passing a non-empty
	// ManagedSettings string — silently dropping their input would mask a bug
	// in whatever produced the JSON.
	if opts.ManagedSettings != "" {
		m, err := parseSettingsJSON(opts.ManagedSettings)
		if err != nil {
			return nil, fmt.Errorf("parsing managed settings: %w", err)
		}
		if m != nil {
			result.BySource["managed"] = m
			for k, v := range m {
				result.Merged[k] = v
			}
		}
	}

	return result, nil
}

// readSettingsFile reads and parses a JSON settings file. Returns (nil, nil)
// when the file does not exist; only genuine read/parse errors are returned.
func readSettingsFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseSettingsJSON(string(data))
}

// parseSettingsJSON parses a JSON object string into a map. Returns (nil, err)
// for invalid JSON, (nil, nil) for an empty or null document.
func parseSettingsJSON(raw string) (map[string]any, error) {
	if raw == "" {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	return m, nil
}
