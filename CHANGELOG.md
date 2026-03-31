# Changelog

## [Unreleased]

### Added

- `GetContextUsage` method on `Client` to query context window utilization by category. Port of Python SDK v0.1.52. ([#53](https://github.com/Flohs/claude-agent-sdk-go/issues/53))
- `DeleteSession` function to delete a session's transcript file. ([#54](https://github.com/Flohs/claude-agent-sdk-go/issues/54))
- `ForkSession` function to create a copy of a session transcript with a new session ID. ([#54](https://github.com/Flohs/claude-agent-sdk-go/issues/54))
- `Offset` field on `ListSessionsOptions` for offset-based pagination. ([#54](https://github.com/Flohs/claude-agent-sdk-go/issues/54))
- `TaskBudget` option for per-task token budget management via `--task-budget` CLI flag. Port of Python SDK v0.1.51. ([#55](https://github.com/Flohs/claude-agent-sdk-go/issues/55))
- `SessionID` option to specify a custom session ID for conversations. Port of Python SDK v0.1.52. ([#56](https://github.com/Flohs/claude-agent-sdk-go/issues/56))
- `ToolUseID` and `AgentID` fields on `ToolPermissionContext` to identify which tool-use and sub-agent is requesting permission. Port of Python SDK v0.1.52. ([#57](https://github.com/Flohs/claude-agent-sdk-go/issues/57))
- `Background`, `Effort`, `PermissionMode`, `DisallowedTools`, `MaxTurns`, and `InitialPrompt` fields on `AgentDefinition` for full agent configuration parity. Port of Python SDK v0.1.51/v0.1.53. ([#58](https://github.com/Flohs/claude-agent-sdk-go/issues/58))
- `SystemPromptFile` option to load system prompts from a file via `--system-prompt-file` CLI flag. Mutually exclusive with `SystemPrompt`. Port of Python SDK v0.1.51. ([#59](https://github.com/Flohs/claude-agent-sdk-go/issues/59))

### Fixed

- `--setting-sources` flag is no longer sent when `SettingSources` is not explicitly configured, aligning with Python SDK v0.1.53 fix. Previously an empty string was always sent. ([#60](https://github.com/Flohs/claude-agent-sdk-go/issues/60))

## [1.2.0] - 2026-03-25

### Added

- `GetSessionInfo` function to retrieve metadata for a single session by ID without scanning all directories. ([#46](https://github.com/Flohs/claude-agent-sdk-go/issues/46))
- `Tag *string` and `CreatedAt *int64` fields on `SDKSessionInfo`, populated by both `ListSessions` and `GetSessionInfo`. Tag is extracted from `type:"tag"` transcript entries; CreatedAt from the first entry's timestamp. Port of Python SDK [#667](https://github.com/anthropics/claude-agent-sdk-python/pull/667). ([#46](https://github.com/Flohs/claude-agent-sdk-go/issues/46))

### Changed

- **Breaking:** `SDKSessionInfo.FileSize` changed from `int64` to `*int64` to align with the Python SDK. ([#46](https://github.com/Flohs/claude-agent-sdk-go/issues/46))
- Minimum Claude CLI version bumped from `2.0.0` to `2.1.0` to ensure compatibility with features like skills, memory, mcpServers in agent definitions, typed `RateLimitEvent`, and `GetSessionInfo` with `tag/created_at`. ([#50](https://github.com/Flohs/claude-agent-sdk-go/issues/50))
- `sdkVersion` constant updated from `1.1.0` to `1.2.0`.

### Fixed

- `ReceiveResponse` and `ReceiveMessages` now check `ctx.Done()` in the inner receive loop, fixing indefinite hangs when context is cancelled while waiting for subprocess messages. ([#48](https://github.com/Flohs/claude-agent-sdk-go/pull/48))
- `SendQuery` now checks `transport.IsReady()` before writing, returning an error if the subprocess has exited instead of silently writing to a dead pipe. ([#48](https://github.com/Flohs/claude-agent-sdk-go/pull/48))
- `Interrupt` now respects the caller's context for both deadline expiry and explicit cancellation, and uses a 30-second default timeout (down from 60s). ([#48](https://github.com/Flohs/claude-agent-sdk-go/pull/48))
- `Close()` now waits up to 5 seconds for the subprocess to exit naturally after closing stdin before sending SIGINT, preventing loss of the last assistant message when the CLI is still writing the session file. Aligns with Python SDK fix [anthropics/claude-agent-sdk-python@40cc6f5](https://github.com/anthropics/claude-agent-sdk-python/commit/40cc6f5). ([#49](https://github.com/Flohs/claude-agent-sdk-go/issues/49))

## [1.1.0] - 2026-03-20

### Added

- `RenameSession` function to programmatically set custom session titles by appending a `custom-title` entry to the JSONL transcript. Port of Python SDK [#668](https://github.com/anthropics/claude-agent-sdk-python/pull/668). ([#40](https://github.com/Flohs/claude-agent-sdk-go/issues/40))

### Changed

- **Breaking:** `RateLimitEvent` now uses typed `RateLimitInfo` struct with `Status`, `ResetsAt`, `RateLimitType`, `Utilization`, `OverageStatus`, `OverageResetsAt`, and `OverageDisabledReason` fields instead of `Data map[string]any`. Adds `RateLimitStatus` type constants. Port of Python SDK [#648](https://github.com/anthropics/claude-agent-sdk-python/pull/648). ([#41](https://github.com/Flohs/claude-agent-sdk-go/issues/41))
- `sdkVersion` constant updated from `0.2.0` to `1.1.0`.

### Fixed

- Refactored env variable merging to use layered ordering: `CLAUDE_CODE_ENTRYPOINT` is set first as a default so users can override it via `Options.Env`, while `CLAUDE_AGENT_SDK_VERSION` remains last and SDK-controlled. Port of Python SDK [#686](https://github.com/anthropics/claude-agent-sdk-python/pull/686). ([#42](https://github.com/Flohs/claude-agent-sdk-go/issues/42))

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
