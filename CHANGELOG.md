# Changelog

## [Unreleased]

### Added

- Model string constants `ModelFable5` (`"claude-fable-5"`), `ModelFable` (`"fable"`), `ModelOpus48` (`"claude-opus-4-8"`), `ModelOpus`, `ModelSonnet46` (`"claude-sonnet-4-6"`), `ModelSonnet`, `ModelHaiku45` (`"claude-haiku-4-5-20251001"`), and `ModelHaiku` exported from the package for use with `Options.Model`. Port of TypeScript SDK v0.3.170. ([#348](https://github.com/Flohs/claude-agent-sdk-go/issues/348))
- `HookEventMessage` typed struct for hook lifecycle events. When `Options.IncludeHookEvents` is `true`, the CLI emits `hook_started` and `hook_response` system messages for each hook invocation; the SDK now parses these into `*HookEventMessage` (which embeds `SystemMessage`). Fields: `HookEvent`, `HookID`, `HookName`, `Output` (response only), `ExitCode` (response only), `Outcome` (response only). Port of Python SDK PR anthropics/claude-agent-sdk-python#917. ([#328](https://github.com/Flohs/claude-agent-sdk-go/issues/328))
- `Client.GetSettings(ctx) (map[string]any, error)` queries the running CLI subprocess for the effective merged settings, including the `"applied"` section with runtime-resolved values (e.g. the actual model after alias resolution). Unlike `ResolveSettings` (which reads from disk), `GetSettings` returns what the live session is actually using. Port of TypeScript SDK v0.2.72. ([#344](https://github.com/Flohs/claude-agent-sdk-go/issues/344))
- `SkipMcpDiscovery bool` field on `SdkPluginConfig`. When `true`, the CLI skips re-reading the plugin's `.mcp.json` during plugin load, allowing SDK hosts that manage MCP server connections independently to load skills and hooks without triggering MCP discovery. Forwarded to the CLI via `--plugin-skip-mcp-discovery <path>`. Port of TypeScript SDK v0.3.172. ([#354](https://github.com/Flohs/claude-agent-sdk-go/issues/354))
- `Client.SetMcpServers(ctx, servers map[string]McpServerConfig) error` sends the `mcp_set_servers` control request, allowing callers to add or replace MCP servers in a live session without restarting. Servers omitted from the map are disconnected; builtin servers (e.g. `"claude-in-chrome"`) can be added even if the CLI was launched without them. SDK MCP servers with resources correctly advertise the resources capability, ensuring resource tools are injected for runtime-added servers. Port of TypeScript SDK v0.3.163 and v0.3.166. ([#329](https://github.com/Flohs/claude-agent-sdk-go/issues/329))
- `UsageDataExperimental` struct (fields: `TotalCostUSD *float64`, `PlanRateLimit map[string]any`, `LocalUsage map[string]any`) and `Client.GetUsageExperimental(ctx context.Context) (*UsageDataExperimental, error)` method. Sends the `get_usage` control request and returns structured session cost, plan rate-limit, and local usage-behavior data. The method name signals the experimental status; the returned data shape may change without notice. Port of TypeScript SDK v0.3.169. ([#350](https://github.com/Flohs/claude-agent-sdk-go/issues/350))
- `TaskUpdatedMessage` typed struct for `system/task_updated` events emitted by background tasks. Fields: `TaskID`, `Patch` (changed fields), `Status` (extracted from `patch.status` for convenience), `SessionID`, `UUID`. `TaskUpdatedStatus` type with constants `TaskUpdatedStatusPending`, `TaskUpdatedStatusRunning`, `TaskUpdatedStatusPaused`, `TaskUpdatedStatusCompleted`, `TaskUpdatedStatusFailed`, `TaskUpdatedStatusKilled`. `TerminalTaskStatuses` map covering completed, failed, killed, and stopped values. Background tasks may finish only via a `task_updated` message without a corresponding `TaskNotificationMessage`; checking `TerminalTaskStatuses` on this message prevents consumers from hanging. Port of Python SDK v0.2.101. ([#411](https://github.com/Flohs/claude-agent-sdk-go/issues/411))
- `ToolUseMetaEntry` struct (`Name string`, `IconURL string`) and `ToolUseMeta` type (`map[string]ToolUseMetaEntry`). `AssistantMessage` gains a `ToolUseMeta ToolUseMeta` field (`json:"tool_use_meta,omitempty"`) populated from the CLI's `tool_use_meta` sidecar when present. Enables SDK consumers to render human-readable tool call labels and icons sourced from MCP server directory metadata instead of raw wire names. Port of TypeScript SDK v0.3.179. ([#412](https://github.com/Flohs/claude-agent-sdk-go/issues/412))
- `ErrorCode *string`, `CanUserPurchaseCredits *bool`, and `HasChargeableSavedPaymentMethod *bool` fields on `RateLimitInfo`. These fields are present on credit-exhaustion rate-limit events, allowing callers to distinguish credit-limit blocks from API rate limits and surface payment prompts accordingly. Port of TypeScript SDK v0.3.181. ([#414](https://github.com/Flohs/claude-agent-sdk-go/issues/414))
- `DenialReason` type with `DenialReasonSafetyCheck` (`"safetyCheck"`) and `DenialReasonAsyncAgent` (`"asyncAgent"`) constants in `permissions.go`. `PermissionDeniedHookInput` gains a `DenialReason DenialReason` field (`json:"denial_reason,omitempty"`) exposing the machine-readable block category alongside the existing human-readable `Reason`. Port of TypeScript SDK v0.3.178. ([#415](https://github.com/Flohs/claude-agent-sdk-go/issues/415))
- `ApiRetryError` type and `ApiRetryErrorRateLimit` / `ApiRetryErrorOverloaded` constants for the machine-readable error category on `ApiRetryMessage`. `ApiRetryMessage` gains an `Error ApiRetryError` field (`json:"error,omitempty"`) exposing this value: `"overloaded"` for 529 and `"rate_limit"` for 429. Port of TypeScript SDK v0.3.150. ([#299](https://github.com/Flohs/claude-agent-sdk-go/issues/299))
- The `initialize` control request is now idempotent: a second call returns the cached initialization result instead of failing with an `"already initialized"` error, supporting reconnect scenarios. Control responses can now carry a `pending_permission_requests` field; the SDK dispatches each entry as a synthetic permission request so permission hook callbacks are not missed when they are queued before the session loop is fully running. Port of TypeScript SDK v0.3.161. ([#309](https://github.com/Flohs/claude-agent-sdk-go/issues/309))
- `StopDetails map[string]any` field on `AssistantMessage` and `ResultMessage`. Populated from the CLI's `stop_details` JSON key when present (e.g. when `StopReason` is `"refusal"`), enabling programmatic refusal detection without text-matching. Port of TypeScript SDK v0.3.162. ([#313](https://github.com/Flohs/claude-agent-sdk-go/issues/313))
- `AdvisorToolConfig` struct (fields: `Model`, `MaxUses`, `Caching`, `AllowedCallers`) and `Advisor any` field on `AgentDefinition`. Set `Advisor` to `true` to enable the advisor tool with default config, or provide `*AdvisorToolConfig` for fine-grained control. Enables the executor/advisor pattern without dropping to the raw Anthropic SDK. Port of Python SDK PR anthropics/claude-agent-sdk-python#880. ([#279](https://github.com/Flohs/claude-agent-sdk-go/issues/279))
- `AlwaysLoad bool` field (`json:"alwaysLoad,omitempty"`) on `McpStdioServerConfig`, `McpSSEServerConfig`, and `McpHTTPServerConfig`. When `true`, the CLI waits for the server to connect before executing the first query (blocking behavior). By default servers connect in the background since CLI v2.1.142. Port of TypeScript SDK v0.3.142. ([#272](https://github.com/Flohs/claude-agent-sdk-go/issues/272))
- `SandboxFilesystemConfig` struct with fields `AllowWrite`, `DenyWrite`, `DenyRead`, `AllowRead`, and `AllowManagedReadPathsOnly` for controlling filesystem access in sandboxed commands. Port of Python SDK PR anthropics/claude-agent-sdk-python#862. ([#280](https://github.com/Flohs/claude-agent-sdk-go/issues/280))
- `Filesystem *SandboxFilesystemConfig` and `EnableWeakerNetworkIsolation *bool` fields on `SandboxSettings`. `EnableWeakerNetworkIsolation` allows system TLS access on macOS when network isolation is enabled. Port of Python SDK PR anthropics/claude-agent-sdk-python#862. ([#280](https://github.com/Flohs/claude-agent-sdk-go/issues/280))
- `Timestamp string` field (ISO-8601, `json:"timestamp,omitempty"`) on `UserMessage`, `AssistantMessage`, `SystemMessage`, and `ResultMessage`. Populated from the `timestamp` field present in CLI transcript JSONL entries. Port of Python SDK PR anthropics/claude-agent-sdk-python#898. ([#277](https://github.com/Flohs/claude-agent-sdk-go/issues/277))
- `HookEventMessageDisplay` (`"MessageDisplay"`) hook event constant, `MessageDisplayHookInput` typed struct (fields: `TurnID`, `MessageID`, `Index`, `Final`, `Delta`), and `MessageDisplayHookOutput` typed output struct with `DisplayContent *string` and `ToHookJSONOutput()` helper. The `MessageDisplay` hook fires during assistant message streaming; returning a non-nil `DisplayContent` replaces the text shown to the user. Display-only: the stored message and what the model sees are untouched. Port of TypeScript SDK v0.3.152. ([#249](https://github.com/Flohs/claude-agent-sdk-go/issues/249))
- `HookEventSessionStart` (`"SessionStart"`) hook event constant, `SessionStartHookInput` typed struct, and `SessionStartHookOutput` typed output with `ReloadSkills bool`, `SessionTitle string`, and `ToHookJSONOutput()`. Returning `ReloadSkills: true` triggers a skill re-scan; `SessionTitle` sets the session title at initialization via `hookSpecificOutput.sessionTitle`. Port of TypeScript SDK v0.3.152. ([#250](https://github.com/Flohs/claude-agent-sdk-go/issues/250))
- `HookEventSessionEnd` (`"SessionEnd"`) hook event and `SessionEndHookInput` typed struct. Fires when a session ends (complements `SessionStart`). ([#252](https://github.com/Flohs/claude-agent-sdk-go/issues/252))
- `HookEventStopFailure` (`"StopFailure"`) hook event and `StopFailureHookInput` typed struct. Fires when the Stop hook itself encounters an error. ([#252](https://github.com/Flohs/claude-agent-sdk-go/issues/252))
- `HookEventPostCompact` (`"PostCompact"`) hook event and `PostCompactHookInput` typed struct with `Trigger` (`"manual"` or `"auto"`) and `CompactSummary` fields. Fires after a context compaction completes. ([#253](https://github.com/Flohs/claude-agent-sdk-go/issues/253))
- `HookEventPostToolBatch` (`"PostToolBatch"`) hook event, `PostToolBatchHookInput` struct (field: `ToolCalls []PostToolBatchToolCall`), and `PostToolBatchToolCall` type (`ToolName`, `ToolInput`, `ToolUseID`, `ToolResponse`). Fires once after a batch of tool calls completes, as opposed to `PostToolUse` which fires per tool. ([#254](https://github.com/Flohs/claude-agent-sdk-go/issues/254))
- `HookEventPermissionDenied` (`"PermissionDenied"`) hook event, `PermissionDeniedHookInput` struct (`ToolName`, `ToolInput`, `ToolUseID`, `Reason`), and `PermissionDeniedHookOutput` typed output with `Retry bool` and `ToHookJSONOutput()`. Fires when a tool call is blocked by a permission check; returning `Retry: true` asks the CLI to retry. ([#255](https://github.com/Flohs/claude-agent-sdk-go/issues/255))
- `HookEventElicitationResult` (`"ElicitationResult"`) hook event and `ElicitationResultHookInput` struct (`McpServerName`, `ElicitationID`, `Mode`, `Action`, `Content`). Fires when an MCP server elicitation request completes; complements `HookEventElicitation` which fires when the request is received. ([#256](https://github.com/Flohs/claude-agent-sdk-go/issues/256))
- `HookEventInstructionsLoaded` (`"InstructionsLoaded"`) hook event constant and `InstructionsLoadedHookInput` typed struct. Fires when Claude loads a CLAUDE.md or rules file during session initialization or directory traversal. Input fields: `FilePath`, `MemoryType` (`"User"`, `"Project"`, `"Local"`, `"Managed"`), `LoadReason` (`"session_start"`, `"nested_traversal"`, `"path_glob_match"`, `"include"`, `"compact"`), `Globs`, `TriggerFilePath`, `ParentFilePath`. ([#257](https://github.com/Flohs/claude-agent-sdk-go/issues/257))
- `HookEventCwdChanged` (`"CwdChanged"`) hook event constant and `CwdChangedHookInput` typed struct with `OldCwd` and `NewCwd` fields. Fires when the working directory changes during a session. ([#257](https://github.com/Flohs/claude-agent-sdk-go/issues/257))
- `HookEventFileChanged` (`"FileChanged"`) hook event constant and `FileChangedHookInput` typed struct with `FilePath` and `ChangeType` (`"modified"`, `"created"`, `"deleted"`) fields. Fires when a watched file changes. Watch paths are registered via the `SessionStart` hook output's `WatchPaths` field. ([#257](https://github.com/Flohs/claude-agent-sdk-go/issues/257))
- `HookEventWorktreeCreate` (`"WorktreeCreate"`) hook event constant, `WorktreeCreateHookInput` struct with `WorktreeName` and `IsolationLevel` fields, and `WorktreeCreateHookOutput` typed output struct. When the output's `WorktreePath` is non-empty the CLI uses that path instead of its default worktree location. ([#258](https://github.com/Flohs/claude-agent-sdk-go/issues/258))
- `HookEventWorktreeRemove` (`"WorktreeRemove"`) hook event constant and `WorktreeRemoveHookInput` struct with `WorktreePath` field. Fires when the CLI removes a git worktree; observability only. ([#258](https://github.com/Flohs/claude-agent-sdk-go/issues/258))
- `HookEventUserPromptExpansion` (`"UserPromptExpansion"`) hook event constant and `UserPromptExpansionHookInput` typed struct. Fires when a slash command or MCP prompt expands before reaching the model. Input fields: `ExpansionType` (`"slash_command"` or `"mcp_prompt"`), `CommandName`, `CommandArgs`, `CommandSource` (`"plugin"`, `"user"`, `"project"`, `"mcp_server"`), `Prompt`. Hook can block the expansion or inject additional context. ([#259](https://github.com/Flohs/claude-agent-sdk-go/issues/259))
- `HookEventSetup` (`"Setup"`) hook event constant and `SetupHookInput` typed struct with `Trigger string` (`"init"` or `"maintenance"`). Fires during one-time session initialization via `--init-only` or `-p --init/--maintenance` flags. Hook output is written to the debug log only. ([#259](https://github.com/Flohs/claude-agent-sdk-go/issues/259))
- `HookEventTaskCreated` (`"TaskCreated"`) hook event constant and `TaskCreatedHookInput` typed struct with `TaskName`, `TaskDescription`, and embedded `SubagentContext` fields. Fires when a new task is being created (e.g. via the TaskCreate tool). Returning `decision:"block"` rolls back the creation. ([#259](https://github.com/Flohs/claude-agent-sdk-go/issues/259))
- `InheritEnv *bool` field on `Options`. When set to `false`, the CLI subprocess starts with a clean environment containing only SDK-required variables plus any entries from `Options.Env` (instead of inheriting the full parent process environment). Defaults to `true` (nil treated as true) for backward compatibility. Port of Python SDK PR anthropics/claude-agent-sdk-python#944. ([#278](https://github.com/Flohs/claude-agent-sdk-go/issues/278))
- `ContentBlocksPrompt` type (`[]map[string]any`) implementing `SystemPrompt`. Allows passing a list of Anthropic content blocks as the system prompt. Serialized to a temporary JSON file and passed via `--system-prompt-file`. Port of Python SDK PRs anthropics/claude-agent-sdk-python#947 and #900. ([#283](https://github.com/Flohs/claude-agent-sdk-go/issues/283))
- `Client.ApplyFlagSettings(ctx, settings map[string]any) error` method for live session flag changes. Sends the `apply_flag_settings` control request; settings take effect on the next query turn. Pass `nil` as the value for a key to clear that override (e.g. `map[string]any{"agent": nil}` resets to the default agent). Port of TypeScript SDK v0.2.132 / v0.3.161. ([#308](https://github.com/Flohs/claude-agent-sdk-go/issues/308))
- `SupportsFastMode bool` field on `ServerCapabilities` (returned by `Client.GetServerCapabilities()`), indicating whether the active model supports fast mode (e.g. Opus fast mode). Port of TypeScript SDK v0.2.69. ([#307](https://github.com/Flohs/claude-agent-sdk-go/issues/307))
- `StopHookOutput` typed output struct for [HookEventStop] callbacks with `AdditionalContext` field and `ToHookJSONOutput()` helper. `SubagentStopHookOutput` typed output struct for [HookEventSubagentStop] with the same fields. Returning a non-empty `AdditionalContext` injects non-error feedback that continues the turn instead of halting it. Port of TypeScript SDK v0.3.163. ([#316](https://github.com/Flohs/claude-agent-sdk-go/issues/316))
- When `OutputFormat["type"]` is `"json_schema"`, the schema is now serialized to a temporary file and passed via `--json-schema-file` instead of being inlined in the `--json-schema` argument, avoiding argument-length limits and reducing log noise. Falls back to inline `--json-schema` if temp-file creation fails. Also adds a `"json_schema_file"` output format type that accepts a caller-provided `"path"` key for schemas already on disk. Port of Python SDK PR anthropics/claude-agent-sdk-python#992. ([#301](https://github.com/Flohs/claude-agent-sdk-go/issues/301))
- `ModelFallbackTrigger` type with five constants (`ModelFallbackTriggerModelNotFound`, `ModelFallbackTriggerPermissionDenied`, `ModelFallbackTriggerOverloaded`, `ModelFallbackTriggerServerError`, `ModelFallbackTriggerLastResort`) and `ModelFallbackMessage` typed struct for `system/model_fallback` events. Previously these arrived as generic `*SystemMessage`. SDK consumers now receive `ModelFallbackMessage` for all fallback triggers; the `Trigger` field exposes the reason and `Model`/`OriginalModel` expose which models were involved. Port of TypeScript SDK v0.3.174. ([#368](https://github.com/Flohs/claude-agent-sdk-go/issues/368))

### Documentation

- `CanUseToolFunc` and `Options.CanUseTool`: clarified in GoDoc that the callback fires only when the CLI's internal permission decision is `"ask"` — it is not invoked for tools already permitted by `AllowedTools`, `PermissionMode`, or `permissions.allow` rules. For a universal pre-tool interceptor, use a `PreToolUse` hook instead. `ToolPermissionContext.ToolUseID` is always non-empty when delivered via this callback. Port of Python SDK PR anthropics/claude-agent-sdk-python#912. ([#330](https://github.com/Flohs/claude-agent-sdk-go/issues/330))
- Deprecated adding `"Skill"` to `Options.AllowedTools` directly. Use `Options.Skills` instead, which automatically injects the appropriate `AllowedTools` entries and defaults `SettingSources`. Port of Python SDK v0.1.77 ([#274](https://github.com/Flohs/claude-agent-sdk-go/issues/274))
- `Options.Env`: expanded GoDoc to describe the four-layer merge order (SDK defaults → inherited `os.Environ()` → user-provided `Env` entries → SDK-controlled vars), clarify that `CLAUDE_AGENT_SDK_VERSION` is always set by the SDK and cannot be overridden via `Env`, and note that `CLAUDECODE` is filtered from the inherited env. Note that unlike the TypeScript SDK (where `options.env` replaces the subprocess environment), the Go SDK merges `Env` on top of the inherited environment. Port of TypeScript SDK v0.3.149. ([#251](https://github.com/Flohs/claude-agent-sdk-go/issues/251))

### Fixed

- Input starting with `"/ "` (slash followed by whitespace, e.g. `"/ add tests"`) is now escaped with a leading space before being sent to the CLI, so it is treated as a plain text prompt rather than a malformed slash command that the CLI silently discards. Port of TypeScript SDK v0.3.172. ([#364](https://github.com/Flohs/claude-agent-sdk-go/issues/364))
- `ReceiveResponse` no longer terminates the message stream when a background-agent `ResultMessage` arrives before the main-session result. Previously any `ResultMessage` caused the channel to close immediately, dropping the main turn's result if a concurrent background agent finished first. The fix checks `ResultMessage.Origin`: only an empty `Origin` (main session) ends the stream; non-empty origins (e.g. `"task_notification"`) are forwarded but do not close the channel. Correspondingly, `waitForResultAndEndInput` now waits for the main-session result (`mainResultCh`) instead of the first result of any origin (`firstResultCh`), preventing premature stdin closure during concurrent background agent runs. Port of TypeScript SDK v0.3.176. ([#375](https://github.com/Flohs/claude-agent-sdk-go/issues/375))
- `Client.StopTask` now treats CLI error responses of `"not_found"` and `"not_running"` as success, making the call idempotent for already-gone tasks. Port of TypeScript SDK v0.3.163. ([#317](https://github.com/Flohs/claude-agent-sdk-go/issues/317))
- Hook callback goroutines are now tracked in `query.wg` and are context-aware: when the query context is cancelled while a hook callback (or `CanUseTool` check) is executing, the goroutine writes an error response to the CLI so it can end the turn cleanly instead of hanging indefinitely. Port of TypeScript SDK v0.3.160. ([#298](https://github.com/Flohs/claude-agent-sdk-go/issues/298))
- When `Options.Skills` is set alongside an explicit `Options.Tools []string`, `"Skill"` is now automatically injected into the `--tools` argument so that skills are actually invocable by the model. Previously `Skill` appeared in `AllowedTools` but not in `--tools`, making skills authorised but unreachable. Port of Python SDK PR anthropics/claude-agent-sdk-python#985. ([#297](https://github.com/Flohs/claude-agent-sdk-go/issues/297))
- When the Claude Code binary exists on disk but `cmd.Start()` fails (e.g. libc mismatch between a musl-compiled binary and a glibc host), the error message now explains the likely cause and suggests setting `Options.CLIPath` to a compatible binary. Port of TypeScript SDK v0.3.178. ([#382](https://github.com/Flohs/claude-agent-sdk-go/issues/382))
- Server-level MCP specs in `Options.DisallowedTools` (e.g. `"mcp__server"`) are now expanded to the wildcard form (`"mcp__server__*"`) before being forwarded to the CLI, so they correctly deny all tools from the named server instead of being silently ignored. Tool-level specs and non-MCP names are unaffected. Port of TypeScript SDK v0.3.178. ([#383](https://github.com/Flohs/claude-agent-sdk-go/issues/383))
- Background agent, remote agent, and MCP task state is now restored when resuming a session via the SDK: `materializeResumeSession` detects still-active tasks by scanning the transcript for `task_started` events without matching terminal `task_notification`/`task_updated` entries, and writes their state to `<sessionDir>/tasks/<taskID>/.meta.json` in the temp config directory so the CLI can resume tracking them. Port of TypeScript SDK v0.3.176. ([#376](https://github.com/Flohs/claude-agent-sdk-go/issues/376))
- Subprocess CLI errors now include the captured stderr tail (last ~8 KB) in the error message, making it much easier to diagnose failures such as missing API keys, invalid flags, or CLI crashes. Port of Python SDK PR anthropics/claude-agent-sdk-python#961. ([#282](https://github.com/Flohs/claude-agent-sdk-go/issues/282))
- Forked session transcripts are now written atomically: all entries are staged in memory first and only committed to the `SessionStore` once the full set is ready. This prevents partial/corrupt forked sessions when a failure occurs mid-write. Port of Python SDK PR anthropics/claude-agent-sdk-python#960. ([#284](https://github.com/Flohs/claude-agent-sdk-go/issues/284))
- Session resume now sanitizes `tool_use.id` values in the loaded transcript to conform to the `toolu_[a-zA-Z0-9_-]+` format required by the Claude API, preventing `400 Bad Request` errors when resuming sessions that contain bare UUIDs or IDs with invalid characters. Port of Python SDK PR anthropics/claude-agent-sdk-python#876. ([#281](https://github.com/Flohs/claude-agent-sdk-go/issues/281))
- Message parser now returns a `MessageParseError` when a required field is present with the wrong type (e.g. `rate_limit_info` is a string instead of an object) instead of silently discarding the data. Also distinguishes between a missing `message` field and one with the wrong type in `parseUserMessage` / `parseAssistantMessage`. Port of Python SDK PR anthropics/claude-agent-sdk-python#988. ([#300](https://github.com/Flohs/claude-agent-sdk-go/issues/300))
- `message_parser.go`: `MemoryRecallMessage` switch-case branch was missing `}, nil` before `case "elicitation_complete":`, leaving the struct literal open and causing a parse error. Residual from the v2.1.0 rebase conflict resolution.
- `types.go`: `MemoryRecallMessage` struct definition was missing its closing `}`, causing a parse error. Residual from the v2.1.0 rebase conflict resolution.
- `hook_input_parser_test.go`: `TestExitPlanModeToolInput_Roundtrip` was missing the closing `}` on the inner if-block, causing a parse error. Residual from the v2.1.0 rebase conflict resolution.

## [2.1.0] - 2026-05-26

### Documentation

- `HookMatcher`: added a **"Dispatch order"** GoDoc section clarifying that all matchers registered for a given event are dispatched concurrently (in parallel), not sequentially. Hook latency is bounded by the slowest single hook, and registration order does not determine execution order. Ordering-dependent designs (e.g. rate-limiters gating subsequent hooks) are not supported. Port of Python SDK v0.2.82 PR #956. ([#222](https://github.com/Flohs/claude-agent-sdk-go/issues/222))

### Tests

- `TestHandleStderr_PanicInCallbackDoesNotAbortLoop`: regression test verifying that a panicking `Options.Stderr` callback does not abort the stderr-reader loop — all subsequent lines are still delivered. The behaviour was already correct (per-call `recover()` in `callStderr`), but there was no test to guard against a future regression. Port of Python SDK v0.2.82 PR #932. ([#223](https://github.com/Flohs/claude-agent-sdk-go/issues/223))

### Fixed

- `extractCreatedAtFromHead` now scans all lines in the head buffer when looking for a `timestamp` field, instead of stopping at the first valid JSON entry that happens to lack one. `ListSessions` and `GetSessionInfo` previously returned `CreatedAt: nil` for sessions whose first JSONL record was a metadata/summary entry without a `timestamp` key. Port of Python SDK v0.1.74 PR #907. ([#220](https://github.com/Flohs/claude-agent-sdk-go/issues/220))
- `transcriptMirrorBatcher`: eliminated a `send on closed channel` race between `Enqueue` and `CloseContext`. The flush-request channel send in `Enqueue` and the channel close in `CloseContext` now both occur while holding the batcher mutex, making concurrent calls safe. The race was benign in normal SDK use (message-reader exits before `CloseContext` runs) but would panic in tests or embedders that exercise both concurrently. ([#221](https://github.com/Flohs/claude-agent-sdk-go/issues/221))

### Added

- CLI subprocesses are now registered in a global process registry and automatically terminated on `SIGINT`/`SIGTERM`, preventing orphaned `claude` processes when the parent Go program exits without calling `Close()`. Port of Python SDK v0.1.74 PR #916. ([#238](https://github.com/Flohs/claude-agent-sdk-go/issues/238))
- `ApiRetryMessage` system message type emitted before each API retry attempt when the CLI encounters a transient error. Fields: `AttemptNumber`, `MaxAttempts`, `DelayMs`, `ErrorStatus *int`, `ErrorMessage`. Port of TypeScript SDK v0.2.77. ([#234](https://github.com/Flohs/claude-agent-sdk-go/issues/234))
- `HookOutputKeyDecision`, `HookOutputKeyReason`, `HookOutputKeyUpdatedToolOutput`, and `HookOutputKeyUpdatedMCPToolOutput` exported string constants for the well-known `HookJSONOutput` map keys, replacing magic string literals. `HookJSONOutput`'s GoDoc is expanded with concrete usage examples for blocking tool calls and replacing tool output. Port of TypeScript SDK v0.2.121 / Python SDK v0.1.74. ([#224](https://github.com/Flohs/claude-agent-sdk-go/issues/224))
- `ExitPlanModeToolInput` typed struct with `PlanFilePath string` field for decoding the `ExitPlanMode` tool's input in `PreToolUse` hook callbacks. Port of TypeScript SDK v0.2.76. ([#236](https://github.com/Flohs/claude-agent-sdk-go/issues/236))
- `MemoryRecallMessage` system message type emitted when Claude loads memory files during a session. Port of TypeScript SDK v0.2.105. ([#235](https://github.com/Flohs/claude-agent-sdk-go/issues/235))
- `MemoryPaths []string` field on `ServerCapabilities` (returned by `Client.GetServerCapabilities()`), exposing the list of memory files loaded at session initialization. Port of TypeScript SDK v0.2.105. ([#235](https://github.com/Flohs/claude-agent-sdk-go/issues/235))
- `McpServerConnectionStatusRequesting` (`"requesting"`) constant for the `McpServerConnectionStatus` type, covering the CLI state while it is actively authenticating or connecting to a remote MCP server. Port of TypeScript SDK v0.2.108. ([#206](https://github.com/Flohs/claude-agent-sdk-go/issues/206))
- `PermissionPolicy map[string]string` field on `McpSSEServerConfig` and `McpHTTPServerConfig` for per-tool permission policies. Keys are tool names; values are `"allow"`, `"ask"`, or `"deny"`. Forwarded to the CLI via the `--mcp-config` JSON blob so the CLI applies the policy to session allow/deny rules without requiring a `CanUseTool` callback. Port of TypeScript SDK v0.2.111. ([#208](https://github.com/Flohs/claude-agent-sdk-go/issues/208))
- `Client.AppendMessage(ctx, content)` method that enqueues a user message with `shouldQuery: false`, appending it to the conversation context without triggering an assistant response turn. Useful for injecting context (tool results, background information) into a session before the next `SendQuery` call. Requires CLI >= v2.1.110. Port of TypeScript SDK v0.2.110. ([#209](https://github.com/Flohs/claude-agent-sdk-go/issues/209))
- `Options.Debug bool` and `Options.DebugFile string` fields for programmatic debug logging control. When `Debug` is true, the CLI subprocess emits verbose debug output; `DebugFile` redirects that output to a file. Callers who previously used `ExtraArgs["debug"]` / `ExtraArgs["debug-file"]` can migrate to these typed fields. Port of TypeScript SDK v0.2.30. ([#207](https://github.com/Flohs/claude-agent-sdk-go/issues/207))
- `ServerCapabilities` struct with `SupportsEffort bool`, `SupportedEffortLevels []Effort`, and `SupportsAdaptiveThinking bool` fields, exposing model capability flags from the CLI initialization result. `Client.GetServerCapabilities()` parses these from the raw init response; the full map remains accessible via `Client.GetServerInfo()`. Port of TypeScript SDK v0.2.49. ([#210](https://github.com/Flohs/claude-agent-sdk-go/issues/210))
- `EffortLevel` type alias for [`Effort`] — provides the same type under the Python SDK naming convention so downstream wrappers that target multiple SDKs can reference it by a consistent name. Both `Effort` and `EffortLevel` resolve to the same underlying type and are interchangeable in all contexts. Port of Python SDK v0.2.82 PR #951. ([#216](https://github.com/Flohs/claude-agent-sdk-go/issues/216))
- `Startup(ctx, opts)` function and `WarmQuery` type for pre-warming the CLI subprocess before calling the one-shot `Query()`. `Startup` starts the subprocess and completes initialization (the expensive part); `WarmQuery.Query(ctx, prompt)` then sends the user message and returns message/error channels with the same shape as `Query`. Use `WarmQuery.Close()` if `Query` is never called. Port of TypeScript SDK v0.2.89. ([#239](https://github.com/Flohs/claude-agent-sdk-go/issues/239))
- `HookEventElicitation` (`"Elicitation"`) constant and `ElicitationHookInput` typed struct for handling MCP server elicitation requests via hook callbacks. `ElicitationHookInput` carries `RequestID`, `ServerName`, `Message`, and `RequestedSchema` fields. Port of TypeScript SDK v0.2.76. ([#237](https://github.com/Flohs/claude-agent-sdk-go/issues/237))
- `ElicitationCompleteMessage` system message type (subtype `"elicitation_complete"`) emitted when an MCP server's elicitation request completes. Carries `RequestID`, `ServerName`, and `Result` fields. Port of TypeScript SDK v0.2.76. ([#237](https://github.com/Flohs/claude-agent-sdk-go/issues/237))
- `PostToolUseHookOutput` typed struct for [PostToolUse hook](https://code.claude.com/docs/en/hooks) return values with `UpdatedToolOutput` (replaces the tool's output for **all** tool types) and deprecated `UpdatedMCPToolOutput` (MCP-only, use `UpdatedToolOutput` instead). `ToHookJSONOutput()` converts the struct to the `HookJSONOutput` map that `HookCallback` returns. Port of TypeScript SDK v0.2.121. ([#230](https://github.com/Flohs/claude-agent-sdk-go/issues/230))

### Documentation

- `HookCallback` and `HookMatcher`: added GoDoc clarifying that when multiple callbacks match the same hook event, the CLI dispatches them **concurrently** — each in its own goroutine from the SDK's perspective. Callbacks that share mutable state must use appropriate synchronisation. Port of Python SDK v0.2.82 PR #956. ([#231](https://github.com/Flohs/claude-agent-sdk-go/issues/231))
- `examples/session_stores/redis/` — Redis-backed `SessionStore` reference adapter. Stores transcript entries in Redis lists (RPUSH/LRANGE) keyed by `SessionKey`; maintains a per-project sorted set for `ListSessions`. Implements `SessionStore` + `SessionStoreLister` + `SessionStoreDeleter` + `SessionStoreSubkeys`. Dependency: `github.com/redis/go-redis/v9`. ([#217](https://github.com/Flohs/claude-agent-sdk-go/issues/217))
- `examples/session_stores/postgres/` — PostgreSQL-backed `SessionStore` reference adapter. Each transcript entry is stored as a JSONB row in a `claude_sessions` table ordered by auto-increment id. Includes `InitSchema` for one-time table/index creation. Implements all four extension interfaces. Dependency: `github.com/jackc/pgx/v5`. ([#217](https://github.com/Flohs/claude-agent-sdk-go/issues/217))
- `examples/session_stores/s3/` — Amazon S3-backed `SessionStore` reference adapter. Each `Append` call writes one S3 object (`part-<unix_nano>.jsonl`); `Load` issues `ListObjectsV2` + `GetObject` per part. `ListSessions` scans the project prefix to derive session mtimes from `LastModified`; `Delete` uses `DeleteObjects` in batches of 1000. Implements all four extension interfaces. Dependencies: `github.com/aws/aws-sdk-go-v2/config` + `github.com/aws/aws-sdk-go-v2/service/s3`. ([#217](https://github.com/Flohs/claude-agent-sdk-go/issues/217))

All three adapters live in standalone Go modules (`go.mod` with a `replace` directive pointing to the local SDK) so they do not pollute the root module's dependency graph. Copy the `adapter.go` file into your project and run `go get` to install the required client library. Port of TypeScript SDK PR anthropics/claude-agent-sdk-typescript#288.

## [2.0.0] - 2026-05-19

This release lands a broad set of parity ports from the Python and TypeScript
SDKs (v0.3.x line) plus three SDK-native features: `ResolveSettings` for
inspecting the effective merged settings without spawning the CLI,
`ImportSessionToStore` for migrating on-disk sessions into a `SessionStore`
adapter, and `SessionStoreFlush` for opt-in eager transcript mirroring.
Also adds deferred-tool-use plumbing (`HookDecisionDefer` +
`ResultMessage.DeferredToolUse`), typed structs for the Task tool
input/output schemas, richer `ToolPermissionContext` and message-parser
fields for end-to-end tracing, and a small but real behavior change to
`ResolveSettings` that surfaces JSON-parse errors instead of swallowing
them.

> **Breaking changes**: a single API-shape break —
> `PermissionRequestHookInput.PermissionSuggestions` is now
> `[]map[string]any` (was `[]any`). Most callers can drop their per-element
> type-assertion. See the `### Changed` section below for the migration note.

### Added

- `EffortXHigh` (`"xhigh"`) constant for the `Effort` type, enabling Opus 4.7's extended thinking effort level. Port of Python SDK v0.1.74 / TypeScript SDK v0.3.144. ([#170](https://github.com/Flohs/claude-agent-sdk-go/issues/170))
- `AssistantMessageErrorModelNotFound` constant for the `AssistantMessageError` type, surfacing `"model_not_found"` errors from the API. Port of TypeScript SDK v0.3.144. ([#171](https://github.com/Flohs/claude-agent-sdk-go/issues/171))
- `APIErrorStatus *int` field on `ResultMessage` that surfaces the underlying HTTP status code when `is_error` is true (e.g. `429`, `529`). Port of TypeScript SDK v0.3.144. ([#172](https://github.com/Flohs/claude-agent-sdk-go/issues/172))
- `AllowedDomains`, `DeniedDomains`, `AllowManagedDomainsOnly`, and `AllowMachLookup` fields on `SandboxNetworkConfig` for fine-grained network domain control in sandboxed environments. Port of TypeScript SDK v0.3.144. ([#182](https://github.com/Flohs/claude-agent-sdk-go/issues/182))
- `Options.StrictMcpConfig bool` field forwarded as `--strict-mcp-config` to the CLI, telling it to use only MCP servers passed via `McpServers` and ignore project, user, and global MCP configurations. Enables fully deterministic server sets for reproducible deployments. Port of TypeScript SDK v0.3.128. ([#178](https://github.com/Flohs/claude-agent-sdk-go/issues/178))
- `Options.ForwardSubagentText bool` field forwarded as `--forward-subagent-text` to the CLI, enabling forwarding of subagent text messages to the parent session stream. Port of TypeScript SDK v0.3.130. ([#179](https://github.com/Flohs/claude-agent-sdk-go/issues/179))
- `Origin string` field on `ResultMessage` that surfaces the triggering message's `SDKMessageOrigin`, allowing consumers to distinguish user-prompted results from task-notification followups. Port of TypeScript SDK v0.2.126. ([#180](https://github.com/Flohs/claude-agent-sdk-go/issues/180))
- `RequestID string` field on `AssistantMessage` and `ResultMessage` for end-to-end request tracing. `SubagentType string` and `TaskDescription string` fields on `TaskStartedMessage`, `TaskProgressMessage`, and `TaskNotificationMessage` for richer task event context. Port of TypeScript SDK v0.3.142. ([#181](https://github.com/Flohs/claude-agent-sdk-go/issues/181))
- `DecisionReason`, `BlockedPath`, `Title`, `DisplayName`, and `Description` fields on `ToolPermissionContext` for richer permission callback context. Permission suggestions are now fully parsed into typed `[]PermissionUpdate` values. Port of Python SDK v0.2.82. ([#176](https://github.com/Flohs/claude-agent-sdk-go/issues/176))
- `TaskCreateInput`, `TaskCreateOutput`, `TaskGetInput`, `TaskGetOutput`, `TaskUpdateInput`, `TaskUpdateOutput`, `TaskListInput`, `TaskListOutput` typed structs in `types.go` for the Task tool input/output schemas (`TaskCreate`, `TaskUpdate`, `TaskGet`, `TaskList`). Also adds `TaskStatus` constants. Port of TypeScript SDK v0.2.141 / v0.3.142. ([#185](https://github.com/Flohs/claude-agent-sdk-go/issues/185))
- `DeferredToolUse` struct and `ResultMessage.DeferredToolUse` field: when a `PreToolUse` hook returns `{"decision": "defer"}`, the CLI pauses tool execution and echoes the pending call on the result message so the SDK caller can prompt the user and resume. Also adds `HookDecision` type with `HookDecisionApprove`, `HookDecisionBlock`, `HookDecisionDefer` constants. Port of Python SDK v0.1.74 PR #865. ([#177](https://github.com/Flohs/claude-agent-sdk-go/issues/177))
- `Options.SessionStoreFlush` (`SessionStoreFlushMode`): controls how the transcript mirror batcher delivers frames to the `SessionStore`. `"batched"` (default) flushes at turn boundaries; `"eager"` flushes after every incoming frame for near-real-time delivery, enabling live-tail UIs, cross-process resume, and crash-durability use cases. Port of Python SDK v0.1.73 PR #905. ([#183](https://github.com/Flohs/claude-agent-sdk-go/issues/183))
- `ImportSessionToStore(ctx, store, sessionID, directory...)` function that imports a local on-disk session (JSONL transcript file) into any `SessionStore` adapter, including all subagent transcripts under the sibling `<session_id>/subagents/` directory. Enables migration from local storage to remote store adapters. Port of Python SDK v0.1.65 PR #858. ([#184](https://github.com/Flohs/claude-agent-sdk-go/issues/184))
- `ResolveSettings(opts *ResolveSettingsOptions) (*ResolvedSettings, error)` (alpha): inspects the effective merged settings without spawning the Claude CLI. Reads the standard cascade — user (`~/.claude/settings.json`), project (`<cwd>/.claude/settings.json`), local (`<cwd>/.claude/settings.local.json`), and inline managed settings — and returns `ResolvedSettings` with the fully merged map and per-source raw maps. Port of TypeScript SDK v0.2.136. ([#186](https://github.com/Flohs/claude-agent-sdk-go/issues/186))

### Changed

- `.github/dependabot.yml` — added `cooldown.default-days: 2` to both `gomod` and `github-actions` ecosystems so Dependabot waits at least 2 days after a release before proposing the update. ([#168](https://github.com/Flohs/claude-agent-sdk-go/issues/168))
- **Breaking:** `PermissionRequestHookInput.PermissionSuggestions` changed from `[]any` to `[]map[string]any` for correct type fidelity with the CLI protocol. Callers that previously type-asserted each element to `map[string]any` can drop the assertion; callers that handled non-map entries will need to adapt. ([#175](https://github.com/Flohs/claude-agent-sdk-go/issues/175))

### Fixed

- Panics inside user-supplied `Options.StderrCallback` functions are now recovered and logged, preventing a crashing callback from terminating the subprocess reader goroutine. Port of TypeScript SDK v0.3.144. ([#173](https://github.com/Flohs/claude-agent-sdk-go/issues/173))
- `Query()` now surfaces a `*ProcessError` on the errors channel when the CLI subprocess exits with a non-zero code and no `is_error: true` result was already delivered. When an error result was delivered, the subprocess exit error is suppressed so callers don't receive both. Port of Python SDK v0.2.82. ([#174](https://github.com/Flohs/claude-agent-sdk-go/issues/174))
- `ResolveSettings` no longer silently swallows JSON parse errors. Corrupt `settings.json` files now surface a wrapped error identifying the source (`user` / `project` / `local`); invalid `ManagedSettings` JSON also returns an error since the caller explicitly opted in. File-not-found is still treated as a non-error skip. ([#204](https://github.com/Flohs/claude-agent-sdk-go/issues/204))

### Documentation

- `ImportSessionToStore`: added inline rationale for the `directory ...string` variadic signature, distinguishing it from the `*ViaStore` mutators that migrated to `StoreMutationOptions` in #162. The variadic is intentional here because `directory` is genuinely consumed to locate the on-disk JSONL source — the silent-discard foot-gun that motivated the v1.6.0 fix does not apply. ([#204](https://github.com/Flohs/claude-agent-sdk-go/issues/204))

### Tests

- Added coverage for the [Unreleased] additions: `ResolveSettings` (cascade, shallow merge, corrupt-file and managed-settings parse errors), `ImportSessionToStore` (main transcript, subagent transcripts, error paths, zero-arg variadic), the new message-parser fields on `AssistantMessage` and `ResultMessage` (`RequestID`, `Origin`, `APIErrorStatus`, `DeferredToolUse`) and on `TaskStartedMessage` / `TaskProgressMessage` / `TaskNotificationMessage` (`SubagentType`, `TaskDescription`), the `*ProcessError` suppression branch in the query loop, and the `parsePermissionUpdate` typed-parser helper. ([#204](https://github.com/Flohs/claude-agent-sdk-go/issues/204))


## [1.6.0] - 2026-04-27

This release lands feature-parity with the Python and TypeScript SDKs across
21 features audited against `anthropics/claude-agent-sdk-python` and
`anthropics/claude-agent-sdk-typescript`, plus the **full SessionStore**
adapter system (port of Python SDK v0.1.64 PR #837), one critical content-
parser bugfix, and runnable examples. Total ~12,000 LOC across 27 merged
PRs.

> **SessionStore stability**: the SessionStore interface family
> (`SessionStore`, `SessionStoreLister`, `SessionStoreSummarizer`,
> `SessionStoreDeleter`, `SessionStoreSubkeys`), the `*FromStore` /
> `*ViaStore` helpers, and `Options.SessionStore` ship under SemVer
> compatibility but may evolve in subsequent minor releases based on
> adapter-author feedback. Treat the API as load-bearing for embedders
> and pin a minor version if you depend on its exact shape.

### Added

- `examples/session_store/` demonstrating end-to-end SessionStore usage with `InMemorySessionStore`: mirror mode wiring, store-backed read/mutation helpers, `MirrorErrorMessage` handling, and `Client.CloseContext` deadline behavior. Plus `examples/session_store_filesystem/` showing how to write a custom adapter (pure-Go filesystem-backed implementation of `SessionStore` + `SessionStoreLister` + `SessionStoreDeleter` + `SessionStoreSubkeys`). ([#164](https://github.com/Flohs/claude-agent-sdk-go/issues/164))
- `Client.CloseContext(ctx)` — cancellable variant of `Close` that bounds how long the caller waits for the SessionStore mirror batcher to drain. When the context expires the batcher worker continues in the background; subprocess teardown and resume cleanup proceed regardless. ([#162](https://github.com/Flohs/claude-agent-sdk-go/issues/162))
- Store-backed mutation helpers: `RenameSessionViaStore`, `TagSessionViaStore`, `DeleteSessionViaStore`, `ForkSessionViaStore`. Mirror the filesystem mutators with `context.Context` + `SessionStore` parameters. Delete cascades to subkeys when the store implements `SessionStoreSubkeys`. Fork rewrites `sessionId` on deep-copied entries and appends to the new session key. Completes the SessionStore umbrella (#110). Port of Python SDK v0.1.64. ([#156](https://github.com/Flohs/claude-agent-sdk-go/issues/156))
- Store-backed read helpers: `ListSessionsFromStore`, `GetSessionMessagesFromStore`, `ListSubagentsFromStore`, `GetSubagentMessagesFromStore`. Mirror the filesystem helpers with `context.Context` + `SessionStore` parameters. `ListSessionsFromStore` uses the `SessionStoreSummarizer` fast path when available (O(1) per session via sidecars) and falls back to full `Load` otherwise. Shared chain-builder refactor avoids duplicating the JSONL → conversation-chain pipeline between disk and store paths. Part of SessionStore umbrella (#110). Port of Python SDK v0.1.64. ([#155](https://github.com/Flohs/claude-agent-sdk-go/issues/155))
- Resume-from-store for `SessionStore`: when `Options.SessionStore` is set alongside `Options.Resume` or `Options.ContinueConversation`, the SDK materializes the requested session from the store into a temporary `CLAUDE_CONFIG_DIR` (with subagent subkeys and copied auth files), points the CLI subprocess at that temp dir via env, and cleans up on teardown with Windows-safe retries. Pre-flight validation now also requires `SessionStoreLister` when `ContinueConversation` is combined with a store. Port of Python SDK v0.1.64. ([#154](https://github.com/Flohs/claude-agent-sdk-go/issues/154))
- Write-path for `SessionStore`: `Options.SessionStore` + `Options.LoadTimeoutMs`, `--session-mirror` CLI flag forwarding, internal `transcriptMirrorBatcher` with 500-entry/1-MiB flush thresholds and 3-retry × 0.2/0.8s backoff (60s per-append timeout, timeouts do not consume retries), `transcript_mirror` stdout frame interception that never surfaces to callers, and `MirrorErrorMessage` system message synthesized on terminal mirror failures. Pre-flight validation rejects `SessionStore` + `EnableFileCheckpointing`. Part of SessionStore umbrella (#110). Port of Python SDK v0.1.64 / TypeScript SDK v0.2.113. ([#153](https://github.com/Flohs/claude-agent-sdk-go/issues/153))
- `SessionStore` interface + 4 optional extension interfaces (`SessionStoreLister`, `SessionStoreSummarizer`, `SessionStoreDeleter`, `SessionStoreSubkeys`), `InMemorySessionStore` reference adapter, and foundational types/helpers (`SessionKey`, `SessionStoreEntry`, `SessionStoreListEntry`, `SessionSummaryEntry`, `SessionListSubkeysKey`, `ProjectKeyForDirectory`, `FilePathToSessionKey`, `FoldSessionSummary`). Foundation for SessionStore support (umbrella #110). Port of Python SDK v0.1.64 (PR #837). No runtime effect yet — wiring lands in Sub B. ([#152](https://github.com/Flohs/claude-agent-sdk-go/issues/152))
- `examples/skills` demonstrating the top-level `Options.Skills` shortcut (`"all"`, named list, and mixing with explicit `AllowedTools` / `SettingSources`). ([#150](https://github.com/Flohs/claude-agent-sdk-go/issues/150))
- `examples/hooks` extended with a `lifecycleEventsExample` that wires up the new `HookEventTaskCompleted` and `HookEventConfigChange` hooks. ([#150](https://github.com/Flohs/claude-agent-sdk-go/issues/150))
- `Options.TraceParent` and `Options.TraceState` fields for W3C trace context propagation to the CLI subprocess (forwarded as `TRACEPARENT` / `TRACESTATE` env vars). OpenTelemetry users can inject the active span context; when unset, externally-set trace env vars still flow through the inherited environment. Port of Python SDK v0.1.60 / PR #821 and TypeScript SDK v0.2.113. ([#129](https://github.com/Flohs/claude-agent-sdk-go/issues/129))
- `ListSubagents(sessionID, opts)` and `GetSubagentMessages(sessionID, agentID, opts)` session helpers for reading subagent transcripts from the sibling `{session_id}/subagents/` directory (supports nested layouts such as `workflows/<runId>/`). Port of Python SDK v0.1.60 / PR #825. ([#126](https://github.com/Flohs/claude-agent-sdk-go/issues/126))
- `IncludeSystemMessages` field on `GetSessionMessagesOptions`. When set, system-subtype transcript entries (task notifications, hook events, etc.) are included in the returned slice. Port of TypeScript SDK v0.2.89. ([#127](https://github.com/Flohs/claude-agent-sdk-go/issues/127))
- `ServerToolUseBlock` and `ServerToolResultBlock` content block types and `ServerToolName` enum constants (`advisor`, `web_search`, `web_fetch`, `code_execution`, `bash_code_execution`, `text_editor_code_execution`, `tool_search_tool_regex`, `tool_search_tool_bm25`). Port of Python SDK v0.1.65 / PR #836. ([#109](https://github.com/Flohs/claude-agent-sdk-go/issues/109))
- `Client.SeedReadState(ctx, entries)` method and `ReadStateEntry` type. Sends the `seed_read_state` control request to populate the CLI's `readFileState` with path/mtime pairs so Edit-style tools work across context compactions. Port of TypeScript SDK v0.2.83. ([#123](https://github.com/Flohs/claude-agent-sdk-go/issues/123))
- `Client.StopAsyncMessage(ctx, uuid)` method that drops a queued user message by UUID before it reaches execution via the `cancel_async_message` control request. Port of TypeScript SDK v0.2.76. ([#122](https://github.com/Flohs/claude-agent-sdk-go/issues/122))
- `Client.PromptSuggestion(ctx)` method that requests prompt suggestions based on the current conversation context. Port of TypeScript SDK v0.2.47. ([#121](https://github.com/Flohs/claude-agent-sdk-go/issues/121))
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

- **Breaking (pre-release):** `RenameSessionViaStore`, `TagSessionViaStore`, `DeleteSessionViaStore`, `ForkSessionViaStore` no longer accept a variadic `directory ...string`. They now take an explicit `StoreMutationOptions` struct (`{Directory: "..."}`) which removes the silent-discard foot-gun where extra directory args were ignored. Callers using the default cwd-derived project key pass `StoreMutationOptions{}`. ([#162](https://github.com/Flohs/claude-agent-sdk-go/issues/162))
- `DeleteSession` now also removes the sibling `{session_id}/` directory (where subagent transcripts live) on a best-effort basis, matching the Python SDK and TypeScript SDK. Failures removing the sibling directory are swallowed so the primary `.jsonl` delete still counts. Port of Python SDK [anthropics/claude-agent-sdk-python#805](https://github.com/anthropics/claude-agent-sdk-python/pull/805). ([#105](https://github.com/Flohs/claude-agent-sdk-go/issues/105))

### Fixed

- **Silent mirror data loss** when `Options.SessionStore` is combined with `Options.Env["CLAUDE_CONFIG_DIR"]` (without using Resume): the parent SDK previously resolved `projectsDir` from its own process environment via `getProjectsDir()`, while the spawned CLI saw the caller-provided env. Mirror frames whose `filePath` fell outside the parent-derived `projectsDir` were silently dropped before reaching the batcher's retry layer, so no `MirrorErrorMessage` ever surfaced. The new `resolveProjectsDir` helper mirrors the CLI's precedence (`Options.Env > parent env > $HOME/.claude`). ([#162](https://github.com/Flohs/claude-agent-sdk-go/issues/162))
- `safeRemoveAll` reported a stale `lastErr` from the prior retry instead of the actual error from the final best-effort sweep, leading to misleading warnings. Now reports the most recent error. ([#162](https://github.com/Flohs/claude-agent-sdk-go/issues/162))
- Documentation: the `SessionStore.Append` GoDoc now states the context-honoring contract explicitly so adapters know that ignoring `ctx` causes a per-call goroutine leak in the SDK's mirror batcher. The `isSafeSubpath` GoDoc no longer claims a symlink-escape check that the lexical implementation does not perform. ([#162](https://github.com/Flohs/claude-agent-sdk-go/issues/162))
- Assistant messages containing server-side tool blocks (`web_search`, `web_fetch`, `advisor`, etc.) previously had those blocks silently dropped by the content parser, which could produce `AssistantMessage{Content: []}` for messages that only carried server-tool blocks. The parser now emits typed `ServerToolUseBlock` / `ServerToolResultBlock` blocks. ([#109](https://github.com/Flohs/claude-agent-sdk-go/issues/109))
- Test helper `buildTestEnv` was out of sync with `Connect()`'s env-building logic after the trace-propagation change. The helper now mirrors the `TRACEPARENT` / `TRACESTATE` forwarding and a new `TestConnectEnv_TraceContext` covers all three cases (unset, TraceParent only, both headers). Also adds `TestParseMessage_TaskProgress_Summary` for the `Summary` field introduced with `AgentProgressSummaries`. ([#150](https://github.com/Flohs/claude-agent-sdk-go/issues/150))
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
