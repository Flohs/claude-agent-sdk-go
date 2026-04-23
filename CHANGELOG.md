# Changelog

## [Unreleased]

### Added

- `Client.SupportedAgents(ctx)` and `Client.SupportedCommands(ctx)` methods for querying available subagents and slash commands in the running session. Port of TypeScript SDK v0.2.63 / v0.2.74. ([#120](https://github.com/Flohs/claude-agent-sdk-go/issues/120))
- `Client.EnableMcpChannel(ctx, serverName, channel)` method and `Capabilities []string` field on `McpServerStatus` for activating SDK-driven MCP channels. Port of TypeScript SDK v0.2.84. ([#119](https://github.com/Flohs/claude-agent-sdk-go/issues/119))
- `Client.ReloadPlugins(ctx)` method that reloads plugins and returns refreshed commands, agents, and MCP server status via the `reload_plugins` control request. Port of TypeScript SDK v0.2.85. ([#118](https://github.com/Flohs/claude-agent-sdk-go/issues/118))
- New hook event constants `HookEventTeammateIdle`, `HookEventTaskCompleted`, `HookEventConfigChange` with typed input structs `TeammateIdleHookInput`, `TaskCompletedHookInput`, `ConfigChangeHookInput`. Port of TypeScript SDK v0.2.33 and v0.2.49. ([#128](https://github.com/Flohs/claude-agent-sdk-go/issues/128))
- `TerminalReason` field on `ResultMessage` (e.g. `completed`, `aborted_tools`, `max_turns`, `blocking_limit`). Previously accessible only via `RawData`. Port of TypeScript SDK v0.2.91. ([#125](https://github.com/Flohs/claude-agent-sdk-go/issues/125))
- Typed `MessageID`, `SessionID`, `UUID`, and `StopReason` fields on `AssistantMessage`. Previously accessible only via `RawData`. Port of Python SDK PRs #619/#685/#718. ([#124](https://github.com/Flohs/claude-agent-sdk-go/issues/124))
- `FailIfUnavailable` field on `SandboxSettings`. When set alongside `Enabled: true`, the CLI emits an error result instead of silently running commands unsandboxed on systems without bwrap/Seatbelt. Port of TypeScript SDK v0.2.91. ([#117](https://github.com/Flohs/claude-agent-sdk-go/issues/117))
- `Display` field on `ThinkingConfigAdaptive` and `ThinkingConfigEnabled`, plus `ThinkingDisplay` type with `ThinkingDisplaySummarized`/`ThinkingDisplayOmitted` constants. Forwarded as `--thinking-display` to let callers override Opus 4.7's default `omitted` thinking text. Port of Python SDK v0.1.65. ([#116](https://github.com/Flohs/claude-agent-sdk-go/issues/116))
- `Options.AgentProgressSummaries` field that enables periodic AI-generated progress summaries on `task_progress` messages, forwarded as `--agent-progress-summaries`. Also adds a typed `Summary` field on `TaskProgressMessage`. Port of TypeScript SDK v0.2.72. ([#115](https://github.com/Flohs/claude-agent-sdk-go/issues/115))
- `Options.IncludeHookEvents` field that enables hook lifecycle system messages (`hook_started`, `hook_progress`, `hook_response`) for all hook event types, forwarded as `--include-hook-events` to the CLI. Port of TypeScript SDK v0.2.89. ([#114](https://github.com/Flohs/claude-agent-sdk-go/issues/114))
- Top-level `Options.Skills` field for enabling skills on the main session without manually configuring `AllowedTools` and `SettingSources`. Accepts `"all"` for every discovered skill or `[]string` of named skills. When set, the SDK auto-injects `Skill` / `Skill(name)` entries into `AllowedTools` and defaults `SettingSources` to `[user, project]` if unset. Port of Python SDK v0.1.62. ([#113](https://github.com/Flohs/claude-agent-sdk-go/issues/113))
- `Options.ManagedSettings` field for passing policy-tier settings to the spawned CLI in-memory, forwarded as `--managed-settings`. Honored below IT-controlled managed sources. Port of TypeScript SDK v0.2.118. ([#112](https://github.com/Flohs/claude-agent-sdk-go/issues/112))
- `Options.Title` field that sets the session title and skips auto-generation, forwarded as `--title` to the CLI. Port of TypeScript SDK v0.2.113. ([#111](https://github.com/Flohs/claude-agent-sdk-go/issues/111))
- `ExcludeDynamicSections` field on `PresetPrompt` for cross-user prompt caching. When set, the SDK sends `excludeDynamicSections` in the initialize request to tell Claude Code to omit user-specific dynamic sections from the system prompt. ([#98](https://github.com/Flohs/claude-agent-sdk-go/issues/98))

### Changed

- `DeleteSession` now also removes the sibling `{session_id}/` directory (where subagent transcripts live) on a best-effort basis, matching the Python SDK and TypeScript SDK. Failures removing the sibling directory are swallowed so the primary `.jsonl` delete still counts. Port of Python SDK [anthropics/claude-agent-sdk-python#805](https://github.com/anthropics/claude-agent-sdk-python/pull/805). ([#105](https://github.com/Flohs/claude-agent-sdk-go/issues/105))

### Fixed

- `ThinkingConfigAdaptive` and `ThinkingConfigDisabled` now correctly map to `--thinking adaptive` / `--thinking disabled` CLI flags instead of incorrectly using `--max-thinking-tokens`. `ThinkingConfigEnabled` and the deprecated `MaxThinkingTokens` field continue to use `--max-thinking-tokens`. ([#99](https://github.com/Flohs/claude-agent-sdk-go/issues/99))

## [1.5.0] - 2026-04-08

### Added

- `SendQueryWithContent` method on `Client` for sending multimodal messages (text, images, documents). ([#96](https://github.com/Flohs/claude-agent-sdk-go/issues/96))
- `NewTextContent` and `NewBase64Content` helper constructors for building content blocks. Block type (`image` vs `document`) is inferred from the media type. ([#96](https://github.com/Flohs/claude-agent-sdk-go/issues/96))
- `Base64Block` and `Base64Source` types implementing the `ContentBlock` interface for image and document content. ([#96](https://github.com/Flohs/claude-agent-sdk-go/issues/96))
- Image and document content block parsing in the message parser. ([#96](https://github.com/Flohs/claude-agent-sdk-go/issues/96))
- Input validation for `SendQueryWithContent` (rejects invalid content types) and `NewBase64Content` (rejects empty media type or data). ([#96](https://github.com/Flohs/claude-agent-sdk-go/issues/96))
- `examples/multimodal_input` example demonstrating image and document input. ([#96](https://github.com/Flohs/claude-agent-sdk-go/issues/96))

### Changed

- `sdkVersion` constant updated from `1.4.0` to `1.5.0`.

## [1.4.0] - 2026-04-07

### Added

- `CLAUDE_AGENT_SDK_INITIALIZE_TIMEOUT` environment variable support for configuring the initialization timeout, with fallback to `CLAUDE_CODE_STREAM_CLOSE_TIMEOUT` for backwards compatibility. Port of Python SDK [anthropics/claude-agent-sdk-python#743](https://github.com/anthropics/claude-agent-sdk-python/pull/743). ([#92](https://github.com/Flohs/claude-agent-sdk-go/issues/92))
- `PermissionModeAuto` constant for the `auto` permission mode supported by CLI 2.1.90+. Port of Python SDK [anthropics/claude-agent-sdk-python#785](https://github.com/anthropics/claude-agent-sdk-python/pull/785). ([#90](https://github.com/Flohs/claude-agent-sdk-go/issues/90))
- `SdkMcpToolAnnotations` type and `Annotations` field on `SdkMcpTool` for configuring MCP tool annotations including `MaxResultSizeChars`, which is forwarded via `_meta["anthropic/maxResultSizeChars"]` to bypass Zod annotation stripping in the CLI. Port of Python SDK [anthropics/claude-agent-sdk-python#756](https://github.com/anthropics/claude-agent-sdk-python/pull/756). ([#91](https://github.com/Flohs/claude-agent-sdk-go/issues/91))

### Changed

- Minimum Claude CLI version bumped from `2.1.0` to `2.1.90` to align with Python SDK and ensure compatibility with v1.3.0 features (TaskBudget, ForkSession, DeleteSession, GetContextUsage, control_cancel_request, Errors on ResultMessage). ([#88](https://github.com/Flohs/claude-agent-sdk-go/issues/88))
- `sdkVersion` constant updated from `1.3.0` to `1.4.0`.

### Fixed

- Prevent deadlock in `Query()` when many messages arrive before the result. When SDK MCP servers or hooks triggered >100 tool calls, the `messageCh` buffer filled before the consumer started draining, blocking `readMessages()` from ever reaching the `result` message. Port of Python SDK [anthropics/claude-agent-sdk-python#780](https://github.com/anthropics/claude-agent-sdk-python/pull/780). ([#85](https://github.com/Flohs/claude-agent-sdk-go/issues/85))

## [1.3.0] - 2026-03-31

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
- `Errors` field on `ResultMessage` to capture structured error information from the CLI. Port of Python SDK v0.1.51. ([#62](https://github.com/Flohs/claude-agent-sdk-go/issues/62))
- `RawData` field on `AssistantMessage` and `ResultMessage` preserving the full raw message map for forward compatibility with fields not yet modeled by the SDK. Port of Python SDK v0.1.51. ([#65](https://github.com/Flohs/claude-agent-sdk-go/issues/65))
- `PermissionModeDontAsk` constant for the `dontAsk` permission mode. Port of Python SDK v0.1.51. ([#66](https://github.com/Flohs/claude-agent-sdk-go/issues/66))
- `SdkMcpResource` and `SdkMcpResourceHandler` types for defining MCP server resources. ([#68](https://github.com/Flohs/claude-agent-sdk-go/issues/68))
- `NewSdkMcpServerWithResources` constructor for creating MCP servers with both tools and resources. ([#68](https://github.com/Flohs/claude-agent-sdk-go/issues/68))
- `resources/list` and `resources/read` MCP method handling in SDK MCP servers. Port of Python SDK v0.1.51. ([#68](https://github.com/Flohs/claude-agent-sdk-go/issues/68))

### Fixed

- `--setting-sources` flag is no longer sent when `SettingSources` is not explicitly configured, aligning with Python SDK v0.1.53 fix. Previously an empty string was always sent. ([#60](https://github.com/Flohs/claude-agent-sdk-go/issues/60))
- `control_cancel_request` messages from the CLI now properly cancel pending control requests instead of being silently ignored. Port of Python SDK v0.1.52. ([#61](https://github.com/Flohs/claude-agent-sdk-go/issues/61))
- `CLAUDECODE` environment variable is now filtered from the subprocess environment to prevent interference with nested SDK/CLI instances. Port of Python SDK v0.1.51. ([#63](https://github.com/Flohs/claude-agent-sdk-go/issues/63))
- Non-JSON lines on CLI stdout (e.g. native module warnings) are now skipped instead of accumulating in the JSON parse buffer. Port of Python SDK v0.1.51. ([#64](https://github.com/Flohs/claude-agent-sdk-go/issues/64))
- SDK MCP tool handler errors are now returned as MCP tool results with `isError: true` instead of JSONRPC protocol errors, conforming to the MCP specification. **Note:** code inspecting raw JSONRPC responses from SDK MCP tool handlers will see a `"result"` with `"isError": true` instead of a JSONRPC `"error"` object. Port of Python SDK v0.1.51. ([#67](https://github.com/Flohs/claude-agent-sdk-go/issues/67))

### Changed

- `sdkVersion` constant updated from `1.2.0` to `1.3.0`.

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
