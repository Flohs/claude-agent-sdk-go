# Changelog

## Unreleased

### Added

- Typed hook input structs for all 10 hook event types: `PreToolUseHookInput`, `PostToolUseHookInput`, `PostToolUseFailureHookInput`, `PermissionRequestHookInput`, `UserPromptSubmitHookInput`, `StopHookInput`, `SubagentStopHookInput`, `SubagentStartHookInput`, `PreCompactHookInput`, `NotificationHookInput`.
- `BaseHookInput` struct with common fields shared across all hook events.
- `SubagentContext` struct with `AgentID` and `AgentType` fields for correlating tool calls to sub-agents running in parallel. Embedded in `PreToolUseHookInput`, `PostToolUseHookInput`, `PostToolUseFailureHookInput`, and `PermissionRequestHookInput`.
- `TypedHookInput` marker interface implemented by all typed hook input structs.
- `ParseHookInput` function to convert a raw `HookInput` map into the appropriate typed struct.
- No breaking changes: `HookInput` (`map[string]any`) and `HookCallback` signature remain unchanged.
