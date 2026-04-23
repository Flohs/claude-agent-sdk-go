package claude

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

// query handles the bidirectional control protocol on top of Transport.
type query struct {
	transport     Transport
	canUseTool    CanUseToolFunc
	hooks         map[string][]hookMatcherInternal
	mcpRouter     *sdkMcpRouter
	agents        map[string]map[string]any

	// Control protocol state
	pendingMu       sync.Mutex
	pendingEvents   map[string]chan struct{}
	pendingResults  map[string]any // map[string](map[string]any | error)
	hookCallbacks   map[string]HookCallback
	nextCallbackID  int
	requestCounter  int

	// Message channel
	messageCh chan map[string]any

	// State
	initTimeout            float64
	initialized            bool
	closed                 bool
	initializationResult   map[string]any
	firstResultCh          chan struct{}
	firstResultOnce        sync.Once
	streamCloseTimeout     float64
	excludeDynamicSections bool

	ctx       context.Context
	cancelFn  context.CancelFunc
	wg        sync.WaitGroup
}

type hookMatcherInternal struct {
	matcher  string
	hooks    []HookCallback
	timeout  *float64
}

type queryConfig struct {
	transport              Transport
	canUseTool             CanUseToolFunc
	hooks                  map[HookEvent][]HookMatcher
	mcpServers             map[string]*McpSdkServerConfig
	initTimeout            float64
	agents                 map[string]AgentDefinition
	excludeDynamicSections bool
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

	q := &query{
		transport:              cfg.transport,
		canUseTool:             cfg.canUseTool,
		hookCallbacks:          make(map[string]HookCallback),
		pendingEvents:          make(map[string]chan struct{}),
		pendingResults:         make(map[string]any),
		messageCh:              make(chan map[string]any, 100),
		initTimeout:            initTimeout,
		firstResultCh:          make(chan struct{}),
		streamCloseTimeout:     streamCloseTimeoutMs / 1000.0,
		excludeDynamicSections: cfg.excludeDynamicSections,
		ctx:                    ctx,
		cancelFn:               cancel,
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
			go q.handleControlRequest(msg)
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

		// Track results for proper stream closure
		if msgType == "result" {
			q.firstResultOnce.Do(func() { close(q.firstResultCh) })
		}

		// Regular SDK messages
		select {
		case q.messageCh <- msg:
		case <-q.ctx.Done():
			return
		}
	}
}

func (q *query) handleControlRequest(msg map[string]any) {
	requestID, _ := msg["request_id"].(string)
	request, _ := msg["request"].(map[string]any)
	subtype, _ := request["subtype"].(string)

	var responseData map[string]any
	var err error

	switch subtype {
	case "can_use_tool":
		responseData, err = q.handleCanUseTool(request)
	case "hook_callback":
		responseData, err = q.handleHookCallback(request)
	case "mcp_message":
		responseData, err = q.handleMcpMessage(request)
	default:
		err = fmt.Errorf("unsupported control request subtype: %s", subtype)
	}

	var response map[string]any
	if err != nil {
		response = map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype":    "error",
				"request_id": requestID,
				"error":      err.Error(),
			},
		}
	} else {
		response = map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype":    "success",
				"request_id": requestID,
				"response":   responseData,
			},
		}
	}

	data, _ := json.Marshal(response)
	_ = q.transport.Write(string(data) + "\n")
}

func (q *query) handleCanUseTool(request map[string]any) (map[string]any, error) {
	if q.canUseTool == nil {
		return nil, fmt.Errorf("canUseTool callback is not provided")
	}

	toolName, _ := request["tool_name"].(string)
	input, _ := request["input"].(map[string]any)
	originalInput := input

	permCtx := ToolPermissionContext{
		ToolUseID: stringField(request, "tool_use_id"),
		AgentID:   stringField(request, "agent_id"),
	}
	if suggestions, ok := request["permission_suggestions"].([]any); ok {
		for _, s := range suggestions {
			if sm, ok := s.(map[string]any); ok {
				_ = sm // TODO: parse permission suggestions
			}
		}
	}

	result, err := q.canUseTool(q.ctx, toolName, input, permCtx)
	if err != nil {
		return nil, err
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

func (q *query) handleHookCallback(request map[string]any) (map[string]any, error) {
	callbackID, _ := request["callback_id"].(string)
	callback, ok := q.hookCallbacks[callbackID]
	if !ok {
		return nil, fmt.Errorf("no hook callback found for ID: %s", callbackID)
	}

	input, _ := request["input"].(map[string]any)
	toolUseID, _ := request["tool_use_id"].(string)

	output, err := callback(q.ctx, HookInput(input), toolUseID, HookContext{})
	if err != nil {
		return nil, err
	}

	// Convert Go-safe field names if needed
	return map[string]any(output), nil
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
				for _, cb := range m.hooks {
					id := fmt.Sprintf("hook_%d", q.nextCallbackID)
					q.nextCallbackID++
					q.hookCallbacks[id] = cb
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

	response, err := q.sendControlRequest(request, time.Duration(q.initTimeout*float64(time.Second)))
	if err != nil {
		return nil, err
	}

	q.initialized = true
	q.initializationResult = response
	return response, nil
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
			if msgType == "error" {
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

func (q *query) interrupt(ctx context.Context) error {
	// Run sendControlRequest in a goroutine so we can select on ctx.Done()
	// for both deadline expiry and explicit cancellation. The underlying
	// request still runs to completion (best-effort signal to the subprocess),
	// but the caller is unblocked immediately.
	type result struct {
		resp map[string]any
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		resp, err := q.sendControlRequest(map[string]any{"subtype": "interrupt"}, 30*time.Second)
		ch <- result{resp, err}
	}()
	select {
	case r := <-ch:
		return r.err
	case <-ctx.Done():
		return ctx.Err()
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

func (q *query) getContextUsage() (*ContextUsage, error) {
	resp, err := q.sendControlRequest(map[string]any{"subtype": "get_context_usage"}, 60*time.Second)
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

func (q *query) stopTask(taskID string) error {
	_, err := q.sendControlRequest(map[string]any{
		"subtype": "stop_task",
		"task_id": taskID,
	}, 60*time.Second)
	return err
}

func (q *query) rewindFiles(userMessageID string) error {
	_, err := q.sendControlRequest(map[string]any{
		"subtype":         "rewind_files",
		"user_message_id": userMessageID,
	}, 60*time.Second)
	return err
}

func (q *query) streamInput(inputCh <-chan map[string]any) {
	for msg := range inputCh {
		if q.closed {
			break
		}
		data, _ := json.Marshal(msg)
		_ = q.transport.Write(string(data) + "\n")
	}

	q.waitForResultAndEndInput()
}

// waitForResultAndEndInput waits for the first result before closing stdin
// when SDK MCP servers or hooks are configured. This prevents closing stdin
// before the CLI completes the MCP initialization handshake.
func (q *query) waitForResultAndEndInput() {
	hasHooks := len(q.hooks) > 0
	hasMcpServers := len(q.mcpRouter.servers) > 0

	if hasMcpServers || hasHooks {
		select {
		case <-q.firstResultCh:
		case <-time.After(time.Duration(q.streamCloseTimeout * float64(time.Second))):
		case <-q.ctx.Done():
		}
	}

	_ = q.transport.EndInput()
}

func (q *query) close() error {
	q.closed = true
	q.cancelFn()
	q.wg.Wait()
	return q.transport.Close()
}
