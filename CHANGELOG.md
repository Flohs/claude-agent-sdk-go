# Changelog

## [Unreleased]

### Added

- `RenameSession` function to programmatically set custom session titles by appending a `custom-title` entry to the JSONL transcript. Port of Python SDK [#668](https://github.com/anthropics/claude-agent-sdk-python/pull/668). ([#40](https://github.com/Flohs/claude-agent-sdk-go/issues/40))

### Changed

- `RateLimitEvent` now uses typed `RateLimitInfo` struct with `Status`, `ResetsAt`, `RateLimitType`, `Utilization`, `OverageStatus`, `OverageResetsAt`, and `OverageDisabledReason` fields instead of `Data map[string]any`. Adds `RateLimitStatus` type constants. Port of Python SDK [#648](https://github.com/anthropics/claude-agent-sdk-python/pull/648). ([#41](https://github.com/Flohs/claude-agent-sdk-go/issues/41))

## [1.0.0] - 2026-03-18

### Added

- Per-turn `Usage` field on `AssistantMessage` to expose token usage per conversation turn. ([#24](https://github.com/Flohs/claude-agent-sdk-go/issues/24))
- `Skills`, `Memory`, and `MCPServers` fields on `AgentDefinition` for per-agent skill, memory, and MCP server configuration. ([#25](https://github.com/Flohs/claude-agent-sdk-go/issues/25))
- Typed `RateLimitEvent` message type for handling rate limit status changes from the CLI. ([#26](https://github.com/Flohs/claude-agent-sdk-go/issues/26))
- `RenameSession` function to assign a custom title to a session transcript. ([#27](https://github.com/Flohs/claude-agent-sdk-go/issues/27))
- `TagSession` function with Unicode sanitization to add tags to session transcripts. ([#28](https://github.com/Flohs/claude-agent-sdk-go/issues/28))
- New examples: `include_partial_messages`, `tools_option`, `setting_sources`, `stderr_callback`, `plugins`, and `filesystem_agents`.
- Extended `streaming` example with interrupt, server info, and timeout sub-examples.

### Changed

- Minimum Go version upgraded from 1.24 to 1.26.1. ([#37](https://github.com/Flohs/claude-agent-sdk-go/pull/37))

### Fixed

- `CLAUDE_CODE_ENTRYPOINT` is now only set when not already present, allowing callers to provide custom entrypoint values. ([#29](https://github.com/Flohs/claude-agent-sdk-go/issues/29))

## [0.2.1] - 2026-03-09

### Fixed

- Inject `CLAUDE_CODE_ENABLE_FINE_GRAINED_TOOL_STREAMING=1` into the CLI subprocess environment when `IncludePartialMessages` is enabled. Without this, tool input parameters are buffered instead of streamed on CLI versions >= v2.1.40. Uses setdefault semantics so user-provided values take precedence. ([#13](https://github.com/Flohs/claude-agent-sdk-go/issues/13))

## [0.2.0] - 2026-03-04

### Fixed

- Prevent stdin closure before MCP server initialization in one-shot `Query()`. When SDK MCP servers or hooks were configured, `Query()` closed stdin immediately after writing the user message, which could cause the CLI to fail during the MCP initialization handshake. The fix extracts the existing wait-for-first-result logic from `streamInput()` into a shared `waitForResultAndEndInput()` method, now used by both the interactive and one-shot code paths. ([#2](https://github.com/Flohs/claude-agent-sdk-go/issues/2))

### Added

- Typed hook input structs for all 10 hook event types: `PreToolUseHookInput`, `PostToolUseHookInput`, `PostToolUseFailureHookInput`, `PermissionRequestHookInput`, `UserPromptSubmitHookInput`, `StopHookInput`, `SubagentStopHookInput`, `SubagentStartHookInput`, `PreCompactHookInput`, `NotificationHookInput`.
- `BaseHookInput` struct with common fields shared across all hook events.
- `SubagentContext` struct with `AgentID` and `AgentType` fields for correlating tool calls to sub-agents running in parallel. Embedded in `PreToolUseHookInput`, `PostToolUseHookInput`, `PostToolUseFailureHookInput`, and `PermissionRequestHookInput`.
- `TypedHookInput` marker interface implemented by all typed hook input structs.
- `ParseHookInput` function to convert a raw `HookInput` map into the appropriate typed struct.
- No breaking changes: `HookInput` (`map[string]any`) and `HookCallback` signature remain unchanged.

### Documentation

- Document minimum CLI version requirement (>= 2.0.0) in README. ([#3](https://github.com/Flohs/claude-agent-sdk-go/issues/3))
