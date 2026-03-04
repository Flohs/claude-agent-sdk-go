# Changelog

## Unreleased

### Fixed

- Prevent stdin closure before MCP server initialization in one-shot `Query()`. When SDK MCP servers or hooks were configured, `Query()` closed stdin immediately after writing the user message, which could cause the CLI to fail during the MCP initialization handshake. The fix extracts the existing wait-for-first-result logic from `streamInput()` into a shared `waitForResultAndEndInput()` method, now used by both the interactive and one-shot code paths. ([#2](https://github.com/Flohs/claude-agent-sdk-go/issues/2))

### Added

- Typed hook input structs for all 10 hook event types: `PreToolUseHookInput`, `PostToolUseHookInput`, `PostToolUseFailureHookInput`, `PermissionRequestHookInput`, `UserPromptSubmitHookInput`, `StopHookInput`, `SubagentStopHookInput`, `SubagentStartHookInput`, `PreCompactHookInput`, `NotificationHookInput`.
- `BaseHookInput` struct with common fields shared across all hook events.
- `SubagentContext` struct with `AgentID` and `AgentType` fields for correlating tool calls to sub-agents running in parallel. Embedded in `PreToolUseHookInput`, `PostToolUseHookInput`, `PostToolUseFailureHookInput`, and `PermissionRequestHookInput`.
- `TypedHookInput` marker interface implemented by all typed hook input structs.
- `ParseHookInput` function to convert a raw `HookInput` map into the appropriate typed struct.
- No breaking changes: `HookInput` (`map[string]any`) and `HookCallback` signature remain unchanged.
