package claude

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// query handles the bidirectional control protocol on top of Transport.
type query struct {
	transport  Transport
	canUseTool CanUseToolFunc
	hooks      map[string][]hookMatcherInternal
	mcpRouter  *sdkMcpRouter
	agents     map[string]map[string]any

	// Control protocol state
	pendingMu      sync.Mutex
	pendingEvents  map[string]chan struct{}
	pendingResults map[string]any // map[string](map[string]any | error)
	hookCallbacks  map[string]HookCallback
	// hookCallbackTimeouts holds the resolved per-callback timeout (mirroring
	// the value forwarded to the CLI at registration), keyed by the same
	// callback ID used in hookCallbacks. Enforced client-side so a hook
	// callback that never returns can't wedge its control request forever.
	hookCallbackTimeouts map[string]time.Duration
	nextCallbackID       int
	requestCounter       int

	// inFlightMu guards inFlightControlRequests, the set of inbound
	// control_request request_ids currently being handled. Guards against
	// duplicate delivery (e.g. a redelivered pending_permission_requests
	// entry, or a transport-level redelivery) invoking a permission/hook
	// callback twice for the same request. Port of TypeScript SDK v0.3.196.
	inFlightMu              sync.Mutex
	inFlightControlRequests map[string]struct{}

	// inFlightTasks tracks background task IDs (task_started with a
	// deferring task_type) that have not yet reached a terminal state via
	// task_notification or a terminated task_updated patch. A main-session
	// result frame ends one turn, not necessarily the run — a background
	// task keeps running past it and still needs stdin for hook/SDK-MCP
	// control responses — so waitForResultAndEndInput must not end input
	// while this set is non-empty. Only ever touched from the readMessages
	// goroutine, so no locking is required. Port of Python SDK commit
	// e6e07f1 (#1103, fixing #1088).
	inFlightTasks map[string]struct{}

	// Message channel
	messageCh chan map[string]any

	// State
	initTimeout            float64
	initialized            bool
	closed                 bool
	initializationResult   map[string]any
	firstResultCh          chan struct{}
	firstResultOnce        sync.Once
	mainResultCh           chan struct{}
	mainResultOnce         sync.Once
	streamCloseTimeout     float64
	excludeDynamicSections bool
	forwardSubagentText    bool
	// lastIsErrorResultDelivered is set (and never cleared) once a result
	// message with is_error:true has been delivered to messageCh in this
	// query's lifetime. Together with lastErrorResultMsg (which does clear,
	// on the next non-error result) it distinguishes "no is_error result was
	// ever delivered" from "one was delivered but a later non-error result
	// superseded it" when the subprocess subsequently exits — the former
	// still gets a plain ProcessError, the latter gets no processError at
	// all (the run recovered; an unrelated crash afterwards has nothing to
	// enrich it with). See processError's doc comment for the full decision.
	lastIsErrorResultDelivered bool
	// lastErrorResultText holds the most recently seen is_error result's
	// error text (see resultErrorText), cleared on the next non-error
	// result. Used to enrich the error handed to any still-pending control
	// request (e.g. an in-flight initialize()) when the subprocess
	// subsequently exits, instead of leaving it to time out.
	// Port of Python SDK commit be2d0df (anthropics/claude-agent-sdk-python#1198).
	lastErrorResultText string
	// lastErrorResultMsg holds the raw payload of the most recently seen
	// is_error result message, cleared on the next non-error result (same
	// lifetime as lastErrorResultText, kept separately since ResultError
	// needs the structured fields, not just the derived text). Used to build
	// a *ResultError carrying that payload when the subprocess subsequently
	// exits with the CLI's own error result as the last thing it reported.
	// Port of Python SDK commit 90ab957 (anthropics/claude-agent-sdk-python#1205).
	lastErrorResultMsg map[string]any
	// processError is set when the subprocess exits with a non-zero code.
	// It is a *ResultError carrying the last is_error result's payload when
	// that result was the most recently delivered message (the CLI's own
	// "why this run failed" report), a plain *ProcessError when no is_error
	// result was ever delivered (a genuine crash with no CLI-reported
	// cause), or left nil when an earlier is_error result was superseded by
	// a later non-error one (the run actually recovered; a subsequent exit
	// error in that case is unrelated to it — see lastErrorResultMsg).
	// Callers (e.g. Query) surface it after the message loop ends.
	processError error

	// Transcript mirror wiring. batcher is nil when Options.SessionStore is
	// unset; when non-nil, transcript_mirror frames are peeled off the
	// inbound stream and handed to the batcher, and synthesized
	// mirror_error frames are surfaced via injectedCh.
	batcher        *transcriptMirrorBatcher
	injectedCh     chan map[string]any
	flushTimeout   time.Duration
	stderrCallback func(string)

	ctx      context.Context
	cancelFn context.CancelFunc
	wg       sync.WaitGroup
}

type hookMatcherInternal struct {
	matcher string
	hooks   []HookCallback
	timeout *float64
}

type queryConfig struct {
	transport              Transport
	canUseTool             CanUseToolFunc
	hooks                  map[HookEvent][]HookMatcher
	mcpServers             map[string]*McpSdkServerConfig
	initTimeout            float64
	agents                 map[string]AgentDefinition
	excludeDynamicSections bool
	forwardSubagentText    bool
	// sessionStore, when non-nil, enables transcript mirroring. projectsDir
	// is the base directory the CLI emits transcript filePath values under
	// (resolved by [getProjectsDir]). loadTimeoutMs caps the batcher's
	// flush-before-result wait; 0 means the 10s default.
	sessionStore      SessionStore
	projectsDir       string
	loadTimeoutMs     int
	sessionStoreFlush SessionStoreFlushMode
	// stderr receives diagnostics emitted by the batcher when it encounters
	// a filePath it cannot resolve to a SessionKey. Optional.
	stderr func(string)
}

func newQuery(cfg queryConfig) *query {
	ctx, cancel := context.WithCancel(context.Background())

	streamCloseTimeoutMs, _ := strconv.ParseFloat(os.Getenv("CLAUDE_CODE_STREAM_CLOSE_TIMEOUT"), 64)
	if streamCloseTimeoutMs == 0 {
		streamCloseTimeoutMs = 60000
	}

	initTimeout := cfg.initTimeout
	if initTimeout == 0 {
		initTimeoutMs, _ := strconv.ParseFloat(os.Getenv("CLAUDE_AGENT_SDK_INITIALIZE_TIMEOUT"), 64)
		if initTimeoutMs == 0 {
			initTimeoutMs, _ = strconv.ParseFloat(os.Getenv("CLAUDE_CODE_STREAM_CLOSE_TIMEOUT"), 64)
		}
		if initTimeoutMs == 0 {
			initTimeoutMs = 60000
		}
		initTimeout = max(initTimeoutMs/1000.0, 60.0)
	}

	flushTimeout := 10 * time.Second
	if cfg.loadTimeoutMs > 0 {
		flushTimeout = time.Duration(cfg.loadTimeoutMs) * time.Millisecond
	}

	q := &query{
		transport:               cfg.transport,
		canUseTool:              cfg.canUseTool,
		hookCallbacks:           make(map[string]HookCallback),
		hookCallbackTimeouts:    make(map[string]time.Duration),
		pendingEvents:           make(map[string]chan struct{}),
		pendingResults:          make(map[string]any),
		inFlightControlRequests: make(map[string]struct{}),
		inFlightTasks:           make(map[string]struct{}),
		messageCh:               make(chan map[string]any, 100),
		initTimeout:             initTimeout,
		firstResultCh:           make(chan struct{}),
		mainResultCh:            make(chan struct{}),
		streamCloseTimeout:      streamCloseTimeoutMs / 1000.0,
		excludeDynamicSections:  cfg.excludeDynamicSections,
		forwardSubagentText:     cfg.forwardSubagentText,
		flushTimeout:            flushTimeout,
		stderrCallback:          cfg.stderr,
		ctx:                     ctx,
		cancelFn:                cancel,
	}

	if cfg.sessionStore != nil {
		// Buffered so the batcher worker never blocks; when full we drop +
		// log so the mirror I/O path cannot stall the message pump.
		q.injectedCh = make(chan map[string]any, 100)
		q.batcher = newTranscriptMirrorBatcher(
			cfg.sessionStore,
			cfg.projectsDir,
			q.reportMirrorError,
			cfg.stderr,
			cfg.sessionStoreFlush == SessionStoreFlushModeEager,
		)
	}

	// Convert hooks
	if cfg.hooks != nil {
		q.hooks = make(map[string][]hookMatcherInternal)
		for event, matchers := range cfg.hooks {
			internal := make([]hookMatcherInternal, len(matchers))
			for i, m := range matchers {
				internal[i] = hookMatcherInternal{
					matcher: m.Matcher,
					hooks:   m.Hooks,
					timeout: m.Timeout,
				}
			}
			q.hooks[string(event)] = internal
		}
	}

	// Set up MCP router
	q.mcpRouter = newSdkMcpRouter()
	for name, server := range cfg.mcpServers {
		q.mcpRouter.addServer(name, server)
	}

	// Convert agents
	if cfg.agents != nil {
		q.agents = make(map[string]map[string]any, len(cfg.agents))
		for name, def := range cfg.agents {
			m := map[string]any{
				"description": def.Description,
				"prompt":      def.Prompt,
			}
			if len(def.Tools) > 0 {
				m["tools"] = def.Tools
			}
			if def.Model != "" {
				m["model"] = def.Model
			}
			if len(def.Skills) > 0 {
				m["skills"] = def.Skills
			}
			if def.Memory != "" {
				m["memory"] = def.Memory
			}
			if len(def.MCPServers) > 0 {
				m["mcpServers"] = def.MCPServers
			}
			if def.Background {
				m["background"] = true
			}
			if def.Effort != "" {
				m["effort"] = def.Effort
			}
			if def.PermissionMode != "" {
				m["permissionMode"] = def.PermissionMode
			}
			if len(def.DisallowedTools) > 0 {
				m["disallowedTools"] = def.DisallowedTools
			}
			if def.MaxTurns != nil {
				m["maxTurns"] = *def.MaxTurns
			}
			if def.InitialPrompt != "" {
				m["initialPrompt"] = def.InitialPrompt
			}
			q.agents[name] = m
		}
	}

	return q
}

func (q *query) start() {
	q.wg.Add(1)
	go q.readMessages()
	if q.injectedCh != nil {
		q.wg.Add(1)
		go q.forwardInjected()
	}
}

// forwardInjected drains SDK-synthesized frames (currently mirror_error) to
// the inbound message channel. It runs as long as the query's context is
// live; on context cancellation it exits so close() can proceed.
func (q *query) forwardInjected() {
	defer q.wg.Done()
	for {
		select {
		case msg, ok := <-q.injectedCh:
			if !ok {
				return
			}
			select {
			case q.messageCh <- msg:
			case <-q.ctx.Done():
				return
			}
		case <-q.ctx.Done():
			return
		}
	}
}

// reportMirrorError is the mirrorErrorFunc handed to the batcher. It
// constructs a mirror_error frame matching the Python reference and pushes
// it onto the injected-frames channel for forwardInjected to deliver. When
// the channel is full the message is dropped and the stderr callback is
// notified so the mirror I/O path cannot stall the message pump.
func (q *query) reportMirrorError(key SessionKey, err error) {
	if err == nil {
		return
	}
	keyCopy := key
	frame := map[string]any{
		"type":       "system",
		"subtype":    "mirror_error",
		"error":      err.Error(),
		"uuid":       newMirrorErrorUUID(),
		"session_id": keyCopy.SessionID,
		"key": map[string]any{
			"project_key": keyCopy.ProjectKey,
			"session_id":  keyCopy.SessionID,
			"subpath":     keyCopy.Subpath,
		},
	}
	select {
	case q.injectedCh <- frame:
	default:
		if q.stderrCallback != nil {
			q.stderrCallback(fmt.Sprintf(
				"[SessionStore] dropping mirror_error (injection buffer full): %s/%s",
				keyCopy.ProjectKey, keyCopy.SessionID,
			))
		}
	}
}

// newMirrorErrorUUID generates a random v4 UUID for synthesized
// mirror_error frames. Reuses the existing UUID pattern in sessions.go.
func newMirrorErrorUUID() string {
	return generateUUID()
}

// deferringTaskTypes are task_started task_type values whose lifetime must
// be tracked before ending input on a main-session result: these task types
// keep running, and needing the control channel, past the main session's
// own turn-ending result. Mirrors the Python SDK's DEFERRING_TASK_TYPES.
var deferringTaskTypes = map[string]bool{
	"local_agent":    true,
	"local_workflow": true,
}

// trackTaskLifecycle updates q.inFlightTasks from a raw system message, so
// waitForResultAndEndInput can tell whether a background task is still
// running when a main-session result arrives. Mirrors the Python SDK's
// _track_task_lifecycle. Must only be called from the readMessages
// goroutine (no locking; see inFlightTasks doc comment).
func (q *query) trackTaskLifecycle(msg map[string]any) {
	taskID, _ := msg["task_id"].(string)
	if taskID == "" {
		return
	}

	switch subtype, _ := msg["subtype"].(string); subtype {
	case "task_started":
		taskType, _ := msg["task_type"].(string)
		if deferringTaskTypes[taskType] {
			q.inFlightTasks[taskID] = struct{}{}
		}
	case "task_notification":
		delete(q.inFlightTasks, taskID)
	case "task_updated":
		if patch, ok := msg["patch"].(map[string]any); ok {
			if status, ok := patch["status"].(string); ok && TerminalTaskStatuses[TaskUpdatedStatus(status)] {
				delete(q.inFlightTasks, taskID)
			}
		}
	}
}

func (q *query) readMessages() {
	defer q.wg.Done()
	defer func() {
		// Signal end of stream
		select {
		case q.messageCh <- map[string]any{"type": "end"}:
		default:
		}
	}()

	msgCh := q.transport.ReadMessages(q.ctx)

	for msg := range msgCh {
		if q.closed {
			break
		}

		msgType, _ := msg["type"].(string)

		// Route control responses
		if msgType == "control_response" {
			response, _ := msg["response"].(map[string]any)
			requestID, _ := response["request_id"].(string)

			q.pendingMu.Lock()
			if ch, ok := q.pendingEvents[requestID]; ok {
				subtype, _ := response["subtype"].(string)
				if subtype == "error" {
					errMsg, _ := response["error"].(string)
					q.pendingResults[requestID] = fmt.Errorf("%s", errMsg)
				} else {
					q.pendingResults[requestID] = response
				}
				close(ch)
			}
			q.pendingMu.Unlock()
			continue
		}

		// Handle incoming control requests from CLI
		if msgType == "control_request" {
			q.wg.Add(1)
			go func() {
				defer q.wg.Done()
				q.handleControlRequest(msg)
			}()
			continue
		}

		if msgType == "control_cancel_request" {
			requestID, _ := msg["request_id"].(string)
			if requestID != "" {
				q.pendingMu.Lock()
				if ch, ok := q.pendingEvents[requestID]; ok {
					q.pendingResults[requestID] = fmt.Errorf("request cancelled by CLI")
					close(ch)
				}
				q.pendingMu.Unlock()
			}
			continue
		}

		// Peel transcript_mirror frames off the stream — they never surface
		// to callers. Frames are handed to the batcher, which persists them
		// to the configured SessionStore in the background.
		if msgType == "transcript_mirror" {
			q.handleTranscriptMirrorFrame(msg)
			continue
		}

		// Subprocess exit error: replace it with the CLI's own error result
		// when the last thing it reported was one (rather than a bare "exit
		// code 1"), and suppress it entirely when an earlier is_error result
		// was since superseded by a non-error one (the run recovered, so an
		// unrelated exit error afterwards has nothing left to enrich it
		// with). Otherwise record the raw exit error so the caller (e.g.
		// Query) can surface a ProcessError after the loop ends.
		if msgType == "error" {
			errStr, _ := msg["error"].(string)

			// Resolve any still-pending control request (e.g. an in-flight
			// initialize()) with the most actionable text available, instead
			// of leaving it to time out. Unconditional: whether an is_error
			// result was already forwarded to messageCh is irrelevant here —
			// a pending control request has no visibility into messageCh.
			pendingErrText := errStr
			if q.lastErrorResultText != "" {
				pendingErrText = fmt.Sprintf("Claude Code returned an error result: %s", q.lastErrorResultText)
			}
			q.failPendingControlRequests(fmt.Errorf("%s", pendingErrText))

			var exitCode *int
			if c, ok := msg["exit_code"].(int); ok {
				exitCode = &c
			}
			switch {
			case q.lastErrorResultMsg != nil:
				errText := fmt.Sprintf("Claude Code returned an error result: %s", resultErrorText(q.lastErrorResultMsg))
				q.processError = newResultError(errText, q.lastErrorResultMsg, exitCode)
			case !q.lastIsErrorResultDelivered:
				q.processError = &ProcessError{SDKError: SDKError{Message: errStr}}
			}
			// Don't forward to messageCh; the deferred "end" frame handles close.
			continue
		}

		// Track background-task lifecycle from system messages so a
		// main-session result arriving while a task is still in flight
		// doesn't prematurely close stdin (see inFlightTasks doc comment).
		if msgType == "system" {
			q.trackTaskLifecycle(msg)
		}

		// Flush the batcher before yielding each result message so any
		// entries emitted during the turn land in the store before the
		// caller observes the result. Runs in a goroutine with a bounded
		// wait so a slow adapter cannot block result delivery.
		if msgType == "result" {
			q.flushBeforeResult()
			if isErr, _ := msg["is_error"].(bool); isErr {
				q.lastIsErrorResultDelivered = true
				q.lastErrorResultText = resultErrorText(msg)
				q.lastErrorResultMsg = msg
			} else {
				q.lastErrorResultText = ""
				q.lastErrorResultMsg = nil
			}
		}

		// Regular SDK messages
		select {
		case q.messageCh <- msg:
		case <-q.ctx.Done():
			return
		}

		// Signal firstResultCh AFTER the result message is forwarded to
		// messageCh, so waitForResultAndEndInput / EndInput cannot fire
		// until the result (including block-feedback from UserPromptSubmit
		// hooks) is already accessible to consumers.
		if msgType == "result" {
			q.firstResultOnce.Do(func() { close(q.firstResultCh) })
			// Close mainResultCh only for a run-ending main-session result:
			// one with no origin (background-agent results carry a
			// structured origin object and must not trigger stdin close, as
			// the main turn is still running) AND no background tasks still
			// in flight (a result frame ends one turn, not necessarily the
			// run — a background task keeps running past it and still needs
			// stdin for hook/SDK-MCP control responses). If tasks are still
			// in flight, this result is skipped and a later main-session
			// result (every task completion wakes the parent for a
			// follow-up turn, which ends in such a result) will close it
			// instead. Port of Python SDK commit e6e07f1 (#1103, fixing
			// #1088).
			if parseMessageOrigin(msg) == nil && len(q.inFlightTasks) == 0 {
				q.mainResultOnce.Do(func() { close(q.mainResultCh) })
			}
		}
	}
}

// handleTranscriptMirrorFrame processes a transcript_mirror frame from the
// CLI stdout. When the batcher is nil (SessionStore unset) or the frame is
// malformed, the frame is silently dropped. Frames whose filePath cannot be
// resolved to a SessionKey are also dropped with a stderr warning — this is
// treated as an SDK bug path, not a user-visible error.
func (q *query) handleTranscriptMirrorFrame(msg map[string]any) {
	if q.batcher == nil {
		return
	}
	filePath, _ := msg["filePath"].(string)
	if filePath == "" {
		return
	}
	rawEntries, ok := msg["entries"].([]any)
	if !ok {
		return
	}
	entries := make([]SessionStoreEntry, 0, len(rawEntries))
	for _, raw := range rawEntries {
		if m, ok := raw.(map[string]any); ok {
			entries = append(entries, m)
		}
	}
	if len(entries) == 0 {
		return
	}
	q.batcher.Enqueue(filePath, entries)
}

// flushBeforeResult waits (up to flushTimeout) for the batcher to drain
// every pending entry so the store sees the session's work before the
// caller sees the result. Runs the Flush in a goroutine so result delivery
// is never blocked by a slow adapter — on timeout the result still flows
// and a diagnostic is emitted via stderr.
func (q *query) flushBeforeResult() {
	if q.batcher == nil {
		return
	}
	ctx, cancel := context.WithTimeout(q.ctx, q.flushTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- q.batcher.Flush(ctx)
	}()
	select {
	case err := <-done:
		if err != nil && q.stderrCallback != nil {
			q.stderrCallback(fmt.Sprintf(
				"[SessionStore] flush-before-result failed: %v", err,
			))
		}
	case <-ctx.Done():
		if q.stderrCallback != nil {
			q.stderrCallback(fmt.Sprintf(
				"[SessionStore] flush-before-result timed out after %s", q.flushTimeout,
			))
		}
	}
}

func (q *query) handleControlRequest(msg map[string]any) {
	requestID, _ := msg["request_id"].(string)
	request, _ := msg["request"].(map[string]any)
	subtype, _ := request["subtype"].(string)

	// Guard against duplicate delivery of the same in-flight request_id (e.g.
	// a redelivered pending_permission_requests entry, or a transport-level
	// redelivery), which would otherwise invoke the callback twice and write
	// two control_responses for the same ID. An empty request_id is never
	// deduped: the synthetic pending_permission_requests dispatch does not
	// currently set one, and treating every empty ID as the same key would
	// incorrectly drop unrelated redelivered requests. Port of TypeScript SDK
	// v0.3.196.
	if requestID != "" {
		q.inFlightMu.Lock()
		if _, dup := q.inFlightControlRequests[requestID]; dup {
			q.inFlightMu.Unlock()
			return
		}
		q.inFlightControlRequests[requestID] = struct{}{}
		q.inFlightMu.Unlock()
		defer func() {
			q.inFlightMu.Lock()
			delete(q.inFlightControlRequests, requestID)
			q.inFlightMu.Unlock()
		}()
	}

	type callbackResult struct {
		resp map[string]any
		err  error
	}

	resultCh := make(chan callbackResult, 1)
	go func() {
		var responseData map[string]any
		var err error
		switch subtype {
		case "can_use_tool":
			responseData, err = q.handleCanUseTool(request, requestID)
		case "hook_callback":
			responseData, err = q.handleHookCallback(request)
		case "mcp_message":
			responseData, err = q.handleMcpMessage(request)
		default:
			err = fmt.Errorf("unsupported control request subtype: %s", subtype)
		}
		resultCh <- callbackResult{responseData, err}
	}()

	var response map[string]any
	select {
	case result := <-resultCh:
		if errors.Is(result.err, errSuppressControlResponse) {
			// The consumer already answered this can_use_tool request
			// out-of-band; do not write a competing control_response.
			return
		}
		if result.err != nil {
			response = map[string]any{
				"type": "control_response",
				"response": map[string]any{
					"subtype":    "error",
					"request_id": requestID,
					"error":      result.err.Error(),
				},
			}
		} else {
			response = map[string]any{
				"type": "control_response",
				"response": map[string]any{
					"subtype":    "success",
					"request_id": requestID,
					"response":   result.resp,
				},
			}
		}
	case <-q.ctx.Done():
		// Context was cancelled while the callback was executing. Send an
		// error response so the CLI subprocess can end the turn cleanly
		// rather than waiting indefinitely for a reply that will never come.
		response = map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype":    "error",
				"request_id": requestID,
				"error":      "context cancelled",
			},
		}
	}

	data, _ := json.Marshal(response)
	_ = q.transport.Write(string(data) + "\n")
}

// errSuppressControlResponse signals that CanUseToolFunc already answered
// this can_use_tool request out-of-band, so handleControlRequest must skip
// writing its own control_response.
var errSuppressControlResponse = errors.New("claudeagent: control response suppressed by consumer")

func (q *query) handleCanUseTool(request map[string]any, requestID string) (map[string]any, error) {
	if q.canUseTool == nil {
		return nil, fmt.Errorf("canUseTool callback is not provided")
	}

	toolName, _ := request["tool_name"].(string)
	input, _ := request["input"].(map[string]any)
	originalInput := input

	permCtx := ToolPermissionContext{
		ToolUseID:               stringField(request, "tool_use_id"),
		RequestID:               requestID,
		AgentID:                 stringField(request, "agent_id"),
		DecisionReason:          stringField(request, "decision_reason"),
		BlockedPath:             stringField(request, "blocked_path"),
		Title:                   stringField(request, "title"),
		DisplayName:             stringField(request, "display_name"),
		Description:             stringField(request, "description"),
		SuppressAlwaysAllowRule: boolField(request, "suppress_always_allow_rule"),
		DefaultToNo:             boolField(request, "default_to_no"),
	}
	if suggestions, ok := request["permission_suggestions"].([]any); ok {
		permCtx.Suggestions = make([]PermissionUpdate, 0, len(suggestions))
		for _, s := range suggestions {
			if sm, ok := s.(map[string]any); ok {
				permCtx.Suggestions = append(permCtx.Suggestions, parsePermissionUpdate(sm))
			}
		}
	}
	if rule, ok := request["matched_ask_rule"].(map[string]any); ok {
		permCtx.MatchedAskRule = &MatchedAskRule{
			Source:      stringField(rule, "source"),
			ToolName:    stringField(rule, "tool_name"),
			RuleContent: stringField(rule, "rule_content"),
		}
	}

	result, err := q.canUseTool(q.ctx, toolName, input, permCtx)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errSuppressControlResponse
	}

	switch r := result.(type) {
	case PermissionResultAllow:
		resp := map[string]any{
			"behavior": "allow",
		}
		if r.UpdatedInput != nil {
			resp["updatedInput"] = r.UpdatedInput
		} else {
			resp["updatedInput"] = originalInput
		}
		if len(r.UpdatedPermissions) > 0 {
			perms := make([]map[string]any, len(r.UpdatedPermissions))
			for i, p := range r.UpdatedPermissions {
				perms[i] = p.ToDict()
			}
			resp["updatedPermissions"] = perms
		}
		return resp, nil

	case PermissionResultDeny:
		resp := map[string]any{
			"behavior": "deny",
			"message":  r.Message,
		}
		if r.Interrupt {
			resp["interrupt"] = true
		}
		return resp, nil

	default:
		return nil, fmt.Errorf("unexpected permission result type: %T", result)
	}
}

// defaultHookCallbackTimeout is the fallback per-callback hook timeout when a
// [HookMatcher] does not set Timeout, matching the documented default on
// [HookMatcher.Timeout].
const defaultHookCallbackTimeout = 60 * time.Second

func (q *query) handleHookCallback(request map[string]any) (map[string]any, error) {
	callbackID, _ := request["callback_id"].(string)
	callback, ok := q.hookCallbacks[callbackID]
	if !ok {
		return nil, fmt.Errorf("no hook callback found for ID: %s", callbackID)
	}

	timeout := defaultHookCallbackTimeout
	if t, ok := q.hookCallbackTimeouts[callbackID]; ok {
		timeout = t
	}

	input, _ := request["input"].(map[string]any)
	toolUseID, _ := request["tool_use_id"].(string)

	ctx, cancel := context.WithTimeout(q.ctx, timeout)
	defer cancel()

	type callbackResult struct {
		output HookJSONOutput
		err    error
	}
	resultCh := make(chan callbackResult, 1)
	go func() {
		output, err := callback(ctx, HookInput(input), toolUseID, HookContext{})
		resultCh <- callbackResult{output, err}
	}()

	select {
	case result := <-resultCh:
		if result.err != nil {
			return nil, result.err
		}
		// Convert Go-safe field names if needed
		return map[string]any(result.output), nil
	case <-ctx.Done():
		if q.ctx.Err() != nil {
			// The query itself was cancelled, not just this callback's timeout.
			return nil, q.ctx.Err()
		}
		return nil, fmt.Errorf("hook callback timed out after %s", timeout)
	}
}

func (q *query) handleMcpMessage(request map[string]any) (map[string]any, error) {
	serverName, _ := request["server_name"].(string)
	message, _ := request["message"].(map[string]any)

	if serverName == "" || message == nil {
		return nil, fmt.Errorf("missing server_name or message for MCP request")
	}

	mcpResponse := q.mcpRouter.handleRequest(q.ctx, serverName, message)
	return map[string]any{"mcp_response": mcpResponse}, nil
}

func (q *query) initialize() (map[string]any, error) {
	// Build hooks config
	var hooksConfig map[string]any
	if len(q.hooks) > 0 {
		hooksConfig = make(map[string]any)
		for event, matchers := range q.hooks {
			matcherConfigs := make([]map[string]any, 0, len(matchers))
			for _, m := range matchers {
				callbackIDs := make([]string, 0, len(m.hooks))
				timeout := defaultHookCallbackTimeout
				if m.timeout != nil {
					timeout = time.Duration(*m.timeout * float64(time.Second))
				}
				for _, cb := range m.hooks {
					id := fmt.Sprintf("hook_%d", q.nextCallbackID)
					q.nextCallbackID++
					q.hookCallbacks[id] = cb
					q.hookCallbackTimeouts[id] = timeout
					callbackIDs = append(callbackIDs, id)
				}
				mc := map[string]any{
					"matcher":         m.matcher,
					"hookCallbackIds": callbackIDs,
				}
				if m.timeout != nil {
					mc["timeout"] = *m.timeout
				}
				matcherConfigs = append(matcherConfigs, mc)
			}
			hooksConfig[event] = matcherConfigs
		}
	}

	request := map[string]any{
		"subtype": "initialize",
		"hooks":   hooksConfig,
	}

	if q.excludeDynamicSections {
		request["excludeDynamicSections"] = true
	}

	if q.agents != nil {
		request["agents"] = q.agents
	}

	if q.forwardSubagentText {
		request["forwardSubagentText"] = true
	}

	response, err := q.sendControlRequest(request, time.Duration(q.initTimeout*float64(time.Second)))
	if err != nil {
		// A second initialize call returns "Already initialized" — treat as success.
		errMsg := err.Error()
		if strings.Contains(strings.ToLower(errMsg), "already initialized") {
			if q.initializationResult != nil {
				return q.initializationResult, nil
			}
		}
		return nil, err
	}

	q.initialized = true
	q.initializationResult = response
	return response, nil
}

// resultErrorText picks the most informative text from an is_error result
// message. Terminal errors the CLI raises itself (error_max_turns,
// error_during_execution, ...) carry their prose in "errors". A run that
// ends on an API failure instead arrives as subtype "success" with
// is_error true, an empty "errors" and the "API Error: ..." prose in
// "result" — falling back to the subtype there produced the
// self-contradictory "Claude Code returned an error result: success".
// Prefer "errors", then "result", then a non-"success" "subtype", then the
// HTTP status, then "unknown error". Mirrors the Python SDK's
// _error_result_text (commit 90ab957, superseding _last_error_result_text
// from commit be2d0df).
func resultErrorText(msg map[string]any) string {
	if errs := normalizeResultErrors(msg["errors"]); len(errs) > 0 {
		return strings.Join(errs, "; ")
	}
	if result, ok := msg["result"].(string); ok {
		if trimmed := strings.TrimSpace(result); trimmed != "" {
			return trimmed
		}
	}
	if subtype, _ := msg["subtype"].(string); subtype != "" && subtype != "success" {
		return subtype
	}
	if v, ok := msg["api_error_status"]; ok {
		if status := intFromAny(v); status != 0 {
			return fmt.Sprintf("API error (HTTP %d)", status)
		}
	}
	return "unknown error"
}

// failPendingControlRequests resolves every still-outstanding control
// request (one with no result recorded yet) with err and wakes its waiter,
// instead of leaving it to its own timeout. Called when the subprocess
// exits/errors during the read loop so a startup failure (e.g. a resume
// refused by --resume-drops-turn) surfaces immediately with the CLI's
// actual error text rather than a generic timeout. Port of Python SDK
// commit be2d0df (anthropics/claude-agent-sdk-python#1198).
func (q *query) failPendingControlRequests(err error) {
	q.pendingMu.Lock()
	defer q.pendingMu.Unlock()
	for requestID, ch := range q.pendingEvents {
		if _, already := q.pendingResults[requestID]; already {
			continue
		}
		q.pendingResults[requestID] = err
		close(ch)
	}
}

func (q *query) sendControlRequest(request map[string]any, timeout time.Duration) (map[string]any, error) {
	q.requestCounter++
	randBytes := make([]byte, 4)
	_, _ = rand.Read(randBytes)
	requestID := fmt.Sprintf("req_%d_%s", q.requestCounter, hex.EncodeToString(randBytes))

	// Create event channel
	ch := make(chan struct{})
	q.pendingMu.Lock()
	q.pendingEvents[requestID] = ch
	q.pendingMu.Unlock()

	// Build and send request
	controlRequest := map[string]any{
		"type":       "control_request",
		"request_id": requestID,
		"request":    request,
	}

	data, _ := json.Marshal(controlRequest)
	if err := q.transport.Write(string(data) + "\n"); err != nil {
		q.pendingMu.Lock()
		delete(q.pendingEvents, requestID)
		q.pendingMu.Unlock()
		return nil, err
	}

	// Wait for response
	select {
	case <-ch:
		q.pendingMu.Lock()
		result := q.pendingResults[requestID]
		delete(q.pendingResults, requestID)
		delete(q.pendingEvents, requestID)
		q.pendingMu.Unlock()

		if err, ok := result.(error); ok {
			return nil, err
		}

		resp, _ := result.(map[string]any)
		responseData, _ := resp["response"].(map[string]any)
		if responseData == nil {
			responseData = map[string]any{}
		}

		// Dispatch any permission requests that were queued before the session
		// loop was fully running. Port of TypeScript SDK v0.3.161.
		if pending, ok := responseData["pending_permission_requests"].([]any); ok {
			for _, pr := range pending {
				if prMap, ok := pr.(map[string]any); ok {
					syntheticMsg := map[string]any{
						"type":    "control_request",
						"request": prMap,
					}
					q.wg.Add(1)
					go func(msg map[string]any) {
						defer q.wg.Done()
						q.handleControlRequest(msg)
					}(syntheticMsg)
				}
			}
		}

		return responseData, nil

	case <-time.After(timeout):
		q.pendingMu.Lock()
		delete(q.pendingEvents, requestID)
		delete(q.pendingResults, requestID)
		q.pendingMu.Unlock()
		subtype, _ := request["subtype"].(string)
		return nil, fmt.Errorf("control request timeout: %s", subtype)

	case <-q.ctx.Done():
		q.pendingMu.Lock()
		delete(q.pendingEvents, requestID)
		delete(q.pendingResults, requestID)
		q.pendingMu.Unlock()
		return nil, q.ctx.Err()
	}
}

func (q *query) receiveMessages() <-chan map[string]any {
	out := make(chan map[string]any, 100)
	go func() {
		defer close(out)
		for msg := range q.messageCh {
			msgType, _ := msg["type"].(string)
			if msgType == "end" {
				break
			}
			select {
			case out <- msg:
			case <-q.ctx.Done():
				return
			}
		}
	}()
	return out
}

func (q *query) interrupt(ctx context.Context, cancelQueued bool) (*InterruptReceipt, error) {
	// Run sendControlRequest in a goroutine so we can select on ctx.Done()
	// for both deadline expiry and explicit cancellation. The underlying
	// request still runs to completion (best-effort signal to the subprocess),
	// but the caller is unblocked immediately.
	type result struct {
		resp map[string]any
		err  error
	}
	req := map[string]any{"subtype": "interrupt"}
	if cancelQueued {
		req["cancel_queued"] = true
	}
	ch := make(chan result, 1)
	go func() {
		resp, err := q.sendControlRequest(req, 30*time.Second)
		ch <- result{resp, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		receipt := &InterruptReceipt{}
		if stillQueued, ok := r.resp["still_queued"].([]any); ok {
			for _, v := range stillQueued {
				if s, ok := v.(string); ok {
					receipt.StillQueued = append(receipt.StillQueued, s)
				}
			}
		}
		if cancelled, ok := r.resp["cancelled"].([]any); ok {
			for _, v := range cancelled {
				if s, ok := v.(string); ok {
					receipt.Cancelled = append(receipt.Cancelled, s)
				}
			}
		}
		return receipt, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (q *query) setPermissionMode(mode string) error {
	_, err := q.sendControlRequest(map[string]any{
		"subtype": "set_permission_mode",
		"mode":    mode,
	}, 60*time.Second)
	return err
}

func (q *query) setModel(model string) error {
	_, err := q.sendControlRequest(map[string]any{
		"subtype": "set_model",
		"model":   model,
	}, 60*time.Second)
	return err
}

func (q *query) applyFlagSettings(settings map[string]any) error {
	req := map[string]any{"subtype": "apply_flag_settings"}
	for k, v := range settings {
		req[k] = v
	}
	_, err := q.sendControlRequest(req, 30*time.Second)
	return err
}

func (q *query) getMcpStatus() (*McpStatusResponse, error) {
	resp, err := q.sendControlRequest(map[string]any{"subtype": "mcp_status"}, 60*time.Second)
	if err != nil {
		return nil, err
	}

	// Marshal and unmarshal to get proper typed response
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}

	var status McpStatusResponse
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (q *query) getContextUsage(detail string) (*ContextUsage, error) {
	req := map[string]any{"subtype": "get_context_usage"}
	if detail != "" {
		req["detail"] = detail
	}
	resp, err := q.sendControlRequest(req, 60*time.Second)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}

	var usage ContextUsage
	if err := json.Unmarshal(data, &usage); err != nil {
		return nil, err
	}
	return &usage, nil
}

func (q *query) getSettings() (map[string]any, error) {
	resp, err := q.sendControlRequest(map[string]any{
		"subtype": "get_settings",
	}, 60*time.Second)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (q *query) reloadPlugins() (map[string]any, error) {
	resp, err := q.sendControlRequest(map[string]any{
		"subtype": "reload_plugins",
	}, 60*time.Second)
	return resp, err
}

func (q *query) enableMcpChannel(serverName, channel string) error {
	_, err := q.sendControlRequest(map[string]any{
		"subtype":    "mcp_enable_channel",
		"serverName": serverName,
		"channel":    channel,
	}, 60*time.Second)
	return err
}

func (q *query) supportedAgents() ([]string, error) {
	resp, err := q.sendControlRequest(map[string]any{
		"subtype": "supported_agents",
	}, 60*time.Second)
	if err != nil {
		return nil, err
	}
	return stringSliceFromResponse(resp, "agents")
}

func (q *query) supportedCommands() ([]string, error) {
	resp, err := q.sendControlRequest(map[string]any{
		"subtype": "supported_commands",
	}, 60*time.Second)
	if err != nil {
		return nil, err
	}
	return stringSliceFromResponse(resp, "commands")
}

func (q *query) promptSuggestion() ([]string, error) {
	resp, err := q.sendControlRequest(map[string]any{
		"subtype": "prompt_suggestion",
	}, 60*time.Second)
	if err != nil {
		return nil, err
	}
	return stringSliceFromResponse(resp, "suggestions")
}

func (q *query) stopAsyncMessage(uuid string) error {
	_, err := q.sendControlRequest(map[string]any{
		"subtype": "cancel_async_message",
		"uuid":    uuid,
	}, 60*time.Second)
	return err
}

func (q *query) seedReadState(entries []ReadStateEntry) error {
	payload := make([]map[string]any, len(entries))
	for i, e := range entries {
		payload[i] = map[string]any{"path": e.Path, "mtime": e.Mtime}
	}
	_, err := q.sendControlRequest(map[string]any{
		"subtype": "seed_read_state",
		"entries": payload,
	}, 60*time.Second)
	return err
}

func stringSliceFromResponse(resp map[string]any, key string) ([]string, error) {
	raw, ok := resp[key].([]any)
	if !ok {
		return nil, nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func (q *query) reconnectMcpServer(serverName string) error {
	_, err := q.sendControlRequest(map[string]any{
		"subtype":    "mcp_reconnect",
		"serverName": serverName,
	}, 60*time.Second)
	return err
}

func (q *query) toggleMcpServer(serverName string, enabled bool) error {
	_, err := q.sendControlRequest(map[string]any{
		"subtype":    "mcp_toggle",
		"serverName": serverName,
		"enabled":    enabled,
	}, 60*time.Second)
	return err
}

func (q *query) setMcpServers(servers map[string]McpServerConfig) error {
	serialized := make(map[string]any, len(servers))
	for name, cfg := range servers {
		// SDK servers need special handling: include capabilities so the CLI
		// knows to call resources/list and resources/read when applicable.
		// This fixes issue #349 (resource tools not injected for runtime servers).
		if sdkCfg, ok := cfg.(*McpSdkServerConfig); ok {
			capabilities := map[string]any{
				"tools": map[string]any{},
			}
			if len(sdkCfg.resources) > 0 || sdkCfg.resourceHandler != nil {
				capabilities["resources"] = map[string]any{}
			}
			serialized[name] = map[string]any{
				"type":         "sdk",
				"name":         sdkCfg.Name,
				"capabilities": capabilities,
			}
			// Register the server in the router so subsequent mcp_message
			// control requests for it are routed correctly.
			q.mcpRouter.addServer(sdkCfg.Name, sdkCfg)
		} else {
			data, err := json.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshal MCP server %q: %w", name, err)
			}
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				return fmt.Errorf("unmarshal MCP server %q: %w", name, err)
			}
			serialized[name] = m
		}
	}
	_, err := q.sendControlRequest(map[string]any{
		"subtype":    "mcp_set_servers",
		"mcpServers": serialized,
	}, 60*time.Second)
	return err
}

func (q *query) stopTask(taskID string) error {
	_, err := q.sendControlRequest(map[string]any{
		"subtype": "stop_task",
		"task_id": taskID,
	}, 60*time.Second)
	if err != nil {
		// not_found and not_running are idempotent-stop success cases.
		// The task is already gone; there is nothing to stop.
		errMsg := err.Error()
		if errMsg == "not_found" || errMsg == "not_running" {
			return nil
		}
	}
	return err
}

func (q *query) rewindFiles(userMessageID string) (*RewindFilesResult, error) {
	resp, err := q.sendControlRequest(map[string]any{
		"subtype":         "rewind_files",
		"user_message_id": userMessageID,
	}, 60*time.Second)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}

	var result RewindFilesResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (q *query) rewindConversation(userMessageID string) error {
	_, err := q.sendControlRequest(map[string]any{
		"subtype":         "rewind_conversation",
		"user_message_id": userMessageID,
	}, 60*time.Second)
	return err
}

func (q *query) getUsageExperimental() (*UsageDataExperimental, error) {
	resp, err := q.sendControlRequest(map[string]any{
		"subtype": "get_usage",
	}, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return &UsageDataExperimental{}, nil
	}
	result := &UsageDataExperimental{}
	if v, ok := resp["total_cost_usd"].(float64); ok {
		result.TotalCostUSD = &v
	}
	if v, ok := resp["plan_rate_limit"].(map[string]any); ok {
		result.PlanRateLimit = v
	}
	if v, ok := resp["local_usage"].(map[string]any); ok {
		result.LocalUsage = v
	}
	if entries, ok := resp["model_scoped"].([]any); ok {
		for _, entry := range entries {
			if m, ok := entry.(map[string]any); ok {
				u := ModelScopedUsage{}
				if s, ok := m["model"].(string); ok {
					u.Model = s
				}
				if n, ok := m["input_tokens"].(float64); ok {
					u.InputTokens = int(n)
				}
				if n, ok := m["output_tokens"].(float64); ok {
					u.OutputTokens = int(n)
				}
				if n, ok := m["cache_creation_input_tokens"].(float64); ok {
					u.CacheCreationInputTokens = int(n)
				}
				if n, ok := m["cache_read_input_tokens"].(float64); ok {
					u.CacheReadInputTokens = int(n)
				}
				result.ModelScoped = append(result.ModelScoped, u)
			}
		}
	}
	return result, nil
}

func (q *query) streamInput(inputCh <-chan map[string]any) {
	written := false
	for msg := range inputCh {
		if q.closed {
			break
		}
		data, _ := json.Marshal(msg)
		_ = q.transport.Write(string(data) + "\n")
		written = true
	}

	// Nothing was ever written, so no result will ever arrive to release a
	// wait — end input immediately instead of blocking for the full
	// streamCloseTimeout. Mirrors the TypeScript SDK's messageCount guard
	// and the Python SDK's written guard.
	if !written {
		_ = q.transport.EndInput()
		return
	}

	q.waitForResultAndEndInput()
}

// waitForResultAndEndInput waits for a run-ending result — a main-session
// result with no background tasks in flight — before closing stdin, when
// SDK MCP servers, hooks, or a CanUseTool callback are configured. This
// prevents closing stdin before the CLI completes the MCP initialization
// handshake, before it can send a can_use_tool control_request and read
// back the SDK's control_response, and prevents closing it while a
// background task still needs the control channel for hook/SDK-MCP
// responses (mainResultCh is only closed under that condition; see its
// gating in readMessages). Bounded by streamCloseTimeout and ctx
// cancellation so a run that never satisfies this condition can't hang
// forever.
func (q *query) waitForResultAndEndInput() {
	hasHooks := len(q.hooks) > 0
	hasMcpServers := len(q.mcpRouter.servers) > 0
	hasCanUseTool := q.canUseTool != nil

	if hasMcpServers || hasHooks || hasCanUseTool {
		select {
		case <-q.mainResultCh:
		case <-time.After(time.Duration(q.streamCloseTimeout * float64(time.Second))):
		case <-q.ctx.Done():
		}
	}

	_ = q.transport.EndInput()
}

func (q *query) close() error {
	return q.closeContext(context.Background())
}

// closeContext is the cancellable variant of close. The caller's ctx
// bounds how long we wait for the mirror batcher to drain. When the
// context fires, the batcher worker is left to finish in the background
// and we proceed straight to subprocess teardown.
func (q *query) closeContext(ctx context.Context) error {
	q.closed = true
	// Drain the batcher BEFORE cancelling the context / closing stdin so
	// in-flight mirror writes have a chance to reach the store before the
	// CLI subprocess is torn down. CloseContext respects the caller's
	// deadline; on timeout we abandon the wait but the worker keeps
	// draining in the background.
	if q.batcher != nil {
		_ = q.batcher.CloseContext(ctx)
	}
	q.cancelFn()
	q.wg.Wait()
	return q.transport.Close()
}
