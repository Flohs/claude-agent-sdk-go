package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// activeChildren tracks all live CLI subprocess Commands so they can be
// terminated when the parent Go process exits unexpectedly.
// This mirrors Python SDK v0.1.74 PR #916 / TypeScript SDK parent-exit cleanup.
var (
	activeChildrenMu sync.Mutex
	activeChildren   []*exec.Cmd
)

func registerChild(cmd *exec.Cmd) {
	activeChildrenMu.Lock()
	activeChildren = append(activeChildren, cmd)
	activeChildrenMu.Unlock()
}

func unregisterChild(cmd *exec.Cmd) {
	activeChildrenMu.Lock()
	for i, c := range activeChildren {
		if c == cmd {
			activeChildren = append(activeChildren[:i], activeChildren[i+1:]...)
			break
		}
	}
	activeChildrenMu.Unlock()
}

func killActiveChildren() {
	activeChildrenMu.Lock()
	cmds := make([]*exec.Cmd, len(activeChildren))
	copy(cmds, activeChildren)
	activeChildren = activeChildren[:0]
	activeChildrenMu.Unlock()
	for _, cmd := range cmds {
		if cmd.Process != nil {
			if runtime.GOOS == "windows" {
				_ = cmd.Process.Kill()
			} else {
				_ = cmd.Process.Signal(os.Interrupt)
			}
		}
	}
}

func init() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		killActiveChildren()
		// Re-raise the signal with the default handler so the process exits
		// with the correct status code.
		signal.Reset(os.Interrupt, syscall.SIGTERM)
		proc, _ := os.FindProcess(os.Getpid())
		if proc != nil {
			_ = proc.Signal(syscall.SIGTERM)
		}
	}()
}

const (
	defaultMaxBufferSize     = 1024 * 1024 // 1MB
	minimumClaudeCodeVersion = "2.1.90"
	sdkVersion               = "2.0.0"

	// stderrMaxBytes caps the rolling stderr buffer at ~8 KB.
	stderrMaxBytes = 8 * 1024
)

// stderrBuffer is a fixed-capacity rolling buffer that retains the tail of
// stderr output up to stderrMaxBytes bytes.
type stderrBuffer struct {
	mu    sync.Mutex
	lines []string
	size  int
}

func (b *stderrBuffer) add(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
	b.size += len(line) + 1 // +1 for the newline separator
	// Evict oldest lines until we are within the cap.
	for b.size > stderrMaxBytes && len(b.lines) > 0 {
		b.size -= len(b.lines[0]) + 1
		b.lines = b.lines[1:]
	}
}

func (b *stderrBuffer) tail() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Join(b.lines, "\n")
}

// SubprocessTransport implements Transport using the Claude Code CLI subprocess.
type SubprocessTransport struct {
	options *Options
	cliPath string
	cwd     string

	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       io.ReadCloser
	stderr       io.ReadCloser
	stderrBuf    stderrBuffer
	ready        bool
	maxBufSize   int
	mu           sync.Mutex
	stdinClosed  bool
	cleanupFuncs []func()
}

// NewSubprocessTransport creates a new subprocess transport.
func NewSubprocessTransport(opts *Options) (*SubprocessTransport, error) {
	if opts == nil {
		opts = &Options{}
	}

	t := &SubprocessTransport{
		options:    opts,
		cwd:       opts.Cwd,
		maxBufSize: defaultMaxBufferSize,
	}

	if opts.MaxBufferSize != nil {
		t.maxBufSize = *opts.MaxBufferSize
	}

	if opts.CLIPath != "" {
		t.cliPath = opts.CLIPath
	} else {
		path, err := findCLI()
		if err != nil {
			return nil, err
		}
		t.cliPath = path
	}

	return t, nil
}

// Connect starts the CLI subprocess.
func (t *SubprocessTransport) Connect(ctx context.Context) error {
	if t.cmd != nil {
		return nil
	}

	if os.Getenv("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK") == "" {
		checkClaudeVersion(t.cliPath)
	}

	args := t.buildCommand()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	// Merge environment using layered ordering:
	// 1. SDK defaults (overridable by system env or user env)
	env := []string{"CLAUDE_CODE_ENTRYPOINT=sdk-go"}

	inheritEnv := t.options.InheritEnv == nil || *t.options.InheritEnv
	if inheritEnv {
		// 2. System environment (filter CLAUDECODE to prevent interference with nested instances)
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "CLAUDECODE=") {
				env = append(env, e)
			}
		}
	}
	// 3. User-provided env vars (override defaults and system env)
	for k, v := range t.options.Env {
		env = append(env, k+"="+v)
	}
	// 4. SDK-controlled vars (never overridable)
	env = append(env, "CLAUDE_AGENT_SDK_VERSION="+sdkVersion)
	if t.options.EnableFileCheckpointing {
		env = append(env, "CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING=true")
	}
	if t.options.TraceParent != "" {
		env = append(env, "TRACEPARENT="+t.options.TraceParent)
	}
	if t.options.TraceState != "" {
		env = append(env, "TRACESTATE="+t.options.TraceState)
	}
	if t.cwd != "" {
		env = append(env, "PWD="+t.cwd)
	}

	cmd.Env = env
	if t.cwd != "" {
		cmd.Dir = t.cwd
	}

	var err error
	t.stdin, err = cmd.StdinPipe()
	if err != nil {
		return &ConnectionError{SDKError: SDKError{Message: fmt.Sprintf("Failed to create stdin pipe: %v", err)}}
	}

	t.stdout, err = cmd.StdoutPipe()
	if err != nil {
		return &ConnectionError{SDKError: SDKError{Message: fmt.Sprintf("Failed to create stdout pipe: %v", err)}}
	}

	// Always pipe stderr so we can capture it for error reporting.
	// The handleStderr goroutine also forwards lines to opts.Stderr callback when set.
	t.stderr, err = cmd.StderrPipe()
	if err != nil {
		return &ConnectionError{SDKError: SDKError{Message: fmt.Sprintf("Failed to create stderr pipe: %v", err)}}
	}

	if err := cmd.Start(); err != nil {
		if t.cwd != "" {
			if _, statErr := os.Stat(t.cwd); os.IsNotExist(statErr) {
				return &ConnectionError{SDKError: SDKError{Message: fmt.Sprintf("Working directory does not exist: %s", t.cwd)}}
			}
		}
		return &NotFoundError{
			ConnectionError: ConnectionError{SDKError: SDKError{Message: "Claude Code not found"}},
			CLIPath:         t.cliPath,
		}
	}

	t.cmd = cmd
	registerChild(cmd)
	t.ready = true

	// Handle stderr in background
	if t.stderr != nil {
		go t.handleStderr()
	}

	return nil
}

// Write sends raw data to stdin.
func (t *SubprocessTransport) Write(data string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.ready || t.stdin == nil || t.stdinClosed {
		return &ConnectionError{SDKError: SDKError{Message: "Transport is not ready for writing"}}
	}

	if t.cmd != nil && t.cmd.ProcessState != nil {
		return &ConnectionError{SDKError: SDKError{Message: fmt.Sprintf("Cannot write to terminated process (exit code: %d)", t.cmd.ProcessState.ExitCode())}}
	}

	_, err := io.WriteString(t.stdin, data)
	if err != nil {
		t.ready = false
		return &ConnectionError{SDKError: SDKError{Message: fmt.Sprintf("Failed to write to process stdin: %v", err)}}
	}
	return nil
}

// ReadMessages returns a channel that receives parsed JSON messages from stdout.
func (t *SubprocessTransport) ReadMessages(ctx context.Context) <-chan map[string]any {
	ch := make(chan map[string]any, 100)

	go func() {
		defer close(ch)

		if t.stdout == nil {
			return
		}

		scanner := bufio.NewScanner(t.stdout)
		scanner.Buffer(make([]byte, 0, t.maxBufSize), t.maxBufSize)

		jsonBuffer := ""

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			// Skip non-JSON lines when not accumulating a partial object
			if jsonBuffer == "" && len(line) > 0 && line[0] != '{' {
				continue
			}

			jsonBuffer += line

			if len(jsonBuffer) > t.maxBufSize {
				jsonBuffer = ""
				continue
			}

			var data map[string]any
			if err := json.Unmarshal([]byte(jsonBuffer), &data); err != nil {
				// Partial JSON, keep accumulating
				continue
			}

			jsonBuffer = ""

			select {
			case ch <- data:
			case <-ctx.Done():
				return
			}
		}

		// Wait for process to finish and check exit code
		if t.cmd != nil {
			cmd := t.cmd
			if err := cmd.Wait(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					code := exitErr.ExitCode()
					stderrTail := t.stderrBuf.tail()
					procErr := &ProcessError{
						SDKError: SDKError{Message: "Command failed"},
						ExitCode: &code,
						Stderr:   stderrTail,
					}
					select {
					case ch <- map[string]any{
						"type":  "error",
						"error": procErr.Error(),
					}:
					case <-ctx.Done():
					}
				}
			}
			unregisterChild(cmd)
		}
		t.runCleanup()
	}()

	return ch
}

// Close terminates the subprocess and cleans up resources.
func (t *SubprocessTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.ready = false

	if t.stdin != nil && !t.stdinClosed {
		_ = t.stdin.Close()
		t.stdinClosed = true
	}

	if t.cmd != nil && t.cmd.Process != nil {
		cmd := t.cmd
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		// Wait for process to exit naturally after stdin close
		select {
		case <-done:
			// Process exited cleanly — no signal needed
		case <-time.After(5 * time.Second):
			// Grace period expired — escalate to SIGINT
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
		}
		unregisterChild(cmd)
	}

	t.cmd = nil
	t.stdout = nil
	t.stdin = nil
	t.stderr = nil

	t.runCleanup()

	return nil
}

// runCleanup executes and clears all registered cleanup functions.
// Must be called without holding t.mu (cleanup functions may need to acquire it).
func (t *SubprocessTransport) runCleanup() {
	fns := t.cleanupFuncs
	t.cleanupFuncs = nil
	for _, fn := range fns {
		fn()
	}
}

// IsReady returns whether the transport is ready.
func (t *SubprocessTransport) IsReady() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ready
}

// EndInput closes stdin to signal end of input.
func (t *SubprocessTransport) EndInput() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stdin != nil && !t.stdinClosed {
		err := t.stdin.Close()
		t.stdinClosed = true
		return err
	}
	return nil
}

// applySkillsDefaults computes the effective AllowedTools and SettingSources
// when Options.Skills is set. When Skills is "all", it injects the bare Skill
// tool; when it is a []string, it injects Skill(name) for each entry. In either
// case SettingSources defaults to [user, project] if unset so the CLI
// discovers installed skills without the caller having to wire both options
// manually. Returns copies — the original options are not mutated.
func applySkillsDefaults(opts *Options) ([]string, []SettingSource) {
	allowed := append([]string(nil), opts.AllowedTools...)
	settingSources := opts.SettingSources

	if opts.Skills == nil {
		return allowed, settingSources
	}

	injected := false
	switch s := opts.Skills.(type) {
	case string:
		if s == "all" {
			if !stringSliceContains(allowed, "Skill") {
				allowed = append(allowed, "Skill")
			}
			injected = true
		}
	case []string:
		for _, name := range s {
			pattern := "Skill(" + name + ")"
			if !stringSliceContains(allowed, pattern) {
				allowed = append(allowed, pattern)
			}
		}
		injected = len(s) > 0
	}

	if injected && settingSources == nil {
		settingSources = []SettingSource{SettingSourceUser, SettingSourceProject}
	}

	return allowed, settingSources
}

func stringSliceContains(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

func (t *SubprocessTransport) handleStderr() {
	if t.stderr == nil {
		return
	}
	scanner := bufio.NewScanner(t.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		// Always capture into the rolling buffer for error reporting.
		t.stderrBuf.add(line)
		// Forward to the caller's callback if provided.
		if t.options.Stderr != nil {
			t.callStderr(line)
		}
	}
}

func (t *SubprocessTransport) callStderr(line string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Warning: StderrCallback panicked: %v\n", r)
		}
	}()
	t.options.Stderr(line)
}

func (t *SubprocessTransport) buildCommand() []string {
	cmd := []string{t.cliPath, "--output-format", "stream-json", "--verbose"}

	opts := t.options

	// System prompt
	if opts.SystemPromptFile != "" {
		cmd = append(cmd, "--system-prompt-file", opts.SystemPromptFile)
	} else if opts.SystemPrompt == nil {
		cmd = append(cmd, "--system-prompt", "")
	} else {
		switch sp := opts.SystemPrompt.(type) {
		case StringPrompt:
			cmd = append(cmd, "--system-prompt", string(sp))
		case PresetPrompt:
			if sp.Append != "" {
				cmd = append(cmd, "--append-system-prompt", sp.Append)
			}
		case ContentBlocksPrompt:
			data, err := json.Marshal([]map[string]any(sp))
			if err == nil {
				tmpFile, err := os.CreateTemp("", "claude-system-prompt-*.json")
				if err == nil {
					_, writeErr := tmpFile.Write(data)
					closeErr := tmpFile.Close()
					if writeErr == nil && closeErr == nil {
						tmpPath := tmpFile.Name()
						cmd = append(cmd, "--system-prompt-file", tmpPath)
						t.cleanupFuncs = append(t.cleanupFuncs, func() {
							_ = os.Remove(tmpPath)
						})
					} else {
						_ = os.Remove(tmpFile.Name())
					}
				}
			}
		}
	}

	// Tools
	if opts.Tools != nil {
		switch tools := opts.Tools.(type) {
		case []string:
			toolList := append([]string(nil), tools...)
			// When skills are active and an explicit tool list is given, inject
			// "Skill" so the model can invoke skills. Without this, Skill is
			// authorised via AllowedTools but never loadable.
			if opts.Skills != nil && len(toolList) > 0 {
				hasSkill := false
				for _, t := range toolList {
					if t == "Skill" {
						hasSkill = true
						break
					}
				}
				if !hasSkill {
					toolList = append(toolList, "Skill")
				}
			}
			if len(toolList) == 0 {
				cmd = append(cmd, "--tools", "")
			} else {
				cmd = append(cmd, "--tools", strings.Join(toolList, ","))
			}
		case *ToolsPreset:
			cmd = append(cmd, "--tools", "default")
		}
	}

	effectiveAllowedTools, effectiveSettingSources := applySkillsDefaults(opts)
	if len(effectiveAllowedTools) > 0 {
		cmd = append(cmd, "--allowedTools", strings.Join(effectiveAllowedTools, ","))
	}

	if opts.MaxTurns != nil {
		cmd = append(cmd, "--max-turns", strconv.Itoa(*opts.MaxTurns))
	}

	if opts.MaxBudgetUSD != nil {
		cmd = append(cmd, "--max-budget-usd", strconv.FormatFloat(*opts.MaxBudgetUSD, 'f', -1, 64))
	}

	if opts.TaskBudget != nil {
		cmd = append(cmd, "--task-budget", strconv.Itoa(*opts.TaskBudget))
	}

	if len(opts.DisallowedTools) > 0 {
		cmd = append(cmd, "--disallowedTools", strings.Join(opts.DisallowedTools, ","))
	}

	if opts.Model != "" {
		cmd = append(cmd, "--model", opts.Model)
	}

	if opts.FallbackModel != "" {
		cmd = append(cmd, "--fallback-model", opts.FallbackModel)
	}

	if len(opts.Betas) > 0 {
		betas := make([]string, len(opts.Betas))
		for i, b := range opts.Betas {
			betas[i] = string(b)
		}
		cmd = append(cmd, "--betas", strings.Join(betas, ","))
	}

	if opts.PermissionPromptToolName != "" {
		cmd = append(cmd, "--permission-prompt-tool", opts.PermissionPromptToolName)
	}

	if opts.PermissionMode != "" {
		cmd = append(cmd, "--permission-mode", string(opts.PermissionMode))
	}

	if opts.ContinueConversation {
		cmd = append(cmd, "--continue")
	}

	if opts.Resume != "" {
		cmd = append(cmd, "--resume", opts.Resume)
	}

	if opts.SessionID != "" {
		cmd = append(cmd, "--session-id", opts.SessionID)
	}

	if opts.Title != "" {
		cmd = append(cmd, "--title", opts.Title)
	}

	// Settings and sandbox
	settingsValue := t.buildSettingsValue()
	if settingsValue != "" {
		cmd = append(cmd, "--settings", settingsValue)
	}

	if opts.ManagedSettings != "" {
		cmd = append(cmd, "--managed-settings", opts.ManagedSettings)
	}

	for _, dir := range opts.AddDirs {
		cmd = append(cmd, "--add-dir", dir)
	}

	// MCP servers
	if opts.McpServers != nil {
		switch servers := opts.McpServers.(type) {
		case map[string]McpServerConfig:
			if len(servers) > 0 {
				serversForCLI := make(map[string]any, len(servers))
				for name, config := range servers {
					serversForCLI[name] = config
				}
				mcpConfig := map[string]any{"mcpServers": serversForCLI}
				data, _ := json.Marshal(mcpConfig)
				cmd = append(cmd, "--mcp-config", string(data))
			}
		case string:
			if servers != "" {
				cmd = append(cmd, "--mcp-config", servers)
			}
		}
	}

	if opts.IncludePartialMessages {
		cmd = append(cmd, "--include-partial-messages")
	}

	if opts.IncludeHookEvents {
		cmd = append(cmd, "--include-hook-events")
	}

	if opts.AgentProgressSummaries {
		cmd = append(cmd, "--agent-progress-summaries")
	}

	if opts.StrictMcpConfig {
		cmd = append(cmd, "--strict-mcp-config")
	}

	if opts.ForwardSubagentText {
		cmd = append(cmd, "--forward-subagent-text")
	}

	if opts.ForkSession {
		cmd = append(cmd, "--fork-session")
	}

	if opts.SessionStore != nil {
		cmd = append(cmd, "--session-mirror")
	}

	// Setting sources
	if effectiveSettingSources != nil {
		sources := make([]string, len(effectiveSettingSources))
		for i, s := range effectiveSettingSources {
			sources[i] = string(s)
		}
		cmd = append(cmd, "--setting-sources", strings.Join(sources, ","))
	}

	// Plugins
	for _, plugin := range opts.Plugins {
		if plugin.Type == "local" {
			cmd = append(cmd, "--plugin-dir", plugin.Path)
		}
	}

	if opts.Debug {
		cmd = append(cmd, "--debug")
		if opts.DebugFile != "" {
			cmd = append(cmd, "--debug-file", opts.DebugFile)
		}
	}

	// Extra args
	for flag, value := range opts.ExtraArgs {
		if value == "" {
			cmd = append(cmd, "--"+flag)
		} else {
			cmd = append(cmd, "--"+flag, value)
		}
	}

	// Thinking config
	var thinkingDisplay ThinkingDisplay
	if opts.Thinking != nil {
		switch tc := opts.Thinking.(type) {
		case ThinkingConfigAdaptive:
			cmd = append(cmd, "--thinking", "adaptive")
			thinkingDisplay = tc.Display
		case ThinkingConfigDisabled:
			cmd = append(cmd, "--thinking", "disabled")
		case ThinkingConfigEnabled:
			cmd = append(cmd, "--max-thinking-tokens", strconv.Itoa(tc.BudgetTokens))
			thinkingDisplay = tc.Display
		}
	} else if opts.MaxThinkingTokens != nil {
		cmd = append(cmd, "--max-thinking-tokens", strconv.Itoa(*opts.MaxThinkingTokens))
	}
	if thinkingDisplay != "" {
		cmd = append(cmd, "--thinking-display", string(thinkingDisplay))
	}

	if opts.Effort != "" {
		cmd = append(cmd, "--effort", string(opts.Effort))
	}

	// Output format / JSON schema
	if opts.OutputFormat != nil {
		switch opts.OutputFormat["type"] {
		case "json_schema":
			if schema, ok := opts.OutputFormat["schema"]; ok {
				data, err := json.Marshal(schema)
				if err == nil {
					// Prefer writing to a temp file to avoid arg-length limits and log noise.
					tmpFile, tmpErr := os.CreateTemp("", "claude-json-schema-*.json")
					if tmpErr == nil {
						_, writeErr := tmpFile.Write(data)
						closeErr := tmpFile.Close()
						if writeErr == nil && closeErr == nil {
							tmpPath := tmpFile.Name()
							cmd = append(cmd, "--json-schema-file", tmpPath)
							t.cleanupFuncs = append(t.cleanupFuncs, func() {
								_ = os.Remove(tmpPath)
							})
						} else {
							_ = os.Remove(tmpFile.Name())
							// Fall back to inline if temp file write/close failed.
							cmd = append(cmd, "--json-schema", string(data))
						}
					} else {
						// Fall back to inline if temp file creation failed.
						cmd = append(cmd, "--json-schema", string(data))
					}
				}
			}
		case "json_schema_file":
			if path, ok := opts.OutputFormat["path"]; ok {
				if pathStr, ok := path.(string); ok && pathStr != "" {
					cmd = append(cmd, "--json-schema-file", pathStr)
				}
			}
		}
	}

	// Always use streaming mode
	cmd = append(cmd, "--input-format", "stream-json")

	return cmd
}

func (t *SubprocessTransport) buildSettingsValue() string {
	hasSettings := t.options.Settings != ""
	hasSandbox := t.options.Sandbox != nil

	if !hasSettings && !hasSandbox {
		return ""
	}

	if hasSettings && !hasSandbox {
		return t.options.Settings
	}

	settingsObj := make(map[string]any)

	if hasSettings {
		str := strings.TrimSpace(t.options.Settings)
		if strings.HasPrefix(str, "{") && strings.HasSuffix(str, "}") {
			_ = json.Unmarshal([]byte(str), &settingsObj)
		} else {
			data, err := os.ReadFile(str)
			if err == nil {
				_ = json.Unmarshal(data, &settingsObj)
			}
		}
	}

	if hasSandbox {
		settingsObj["sandbox"] = t.options.Sandbox
	}

	data, _ := json.Marshal(settingsObj)
	return string(data)
}

// findCLI locates the Claude Code CLI binary.
func findCLI() (string, error) {
	// Check PATH first
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}

	home, _ := os.UserHomeDir()
	locations := []string{
		filepath.Join(home, ".npm-global", "bin", "claude"),
		"/usr/local/bin/claude",
		filepath.Join(home, ".local", "bin", "claude"),
		filepath.Join(home, "node_modules", ".bin", "claude"),
		filepath.Join(home, ".yarn", "bin", "claude"),
		filepath.Join(home, ".claude", "local", "claude"),
	}

	for _, path := range locations {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}

	cliName := "claude"
	if runtime.GOOS == "windows" {
		cliName = "claude.exe"
	}

	return "", &NotFoundError{
		ConnectionError: ConnectionError{SDKError: SDKError{
			Message: fmt.Sprintf(
				"Claude Code not found. Install with:\n"+
					"  npm install -g @anthropic-ai/claude-code\n\n"+
					"If already installed, try:\n"+
					"  export PATH=\"$HOME/node_modules/.bin:$PATH\"\n\n"+
					"Or provide the path via Options:\n"+
					"  Options{CLIPath: \"/path/to/%s\"}", cliName),
		}},
	}
}

var versionRegexp = regexp.MustCompile(`^([0-9]+\.[0-9]+\.[0-9]+)`)

func checkClaudeVersion(cliPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, cliPath, "-v").Output()
	if err != nil {
		return
	}

	match := versionRegexp.FindStringSubmatch(strings.TrimSpace(string(out)))
	if len(match) < 2 {
		return
	}

	version := match[1]
	if compareVersions(version, minimumClaudeCodeVersion) < 0 {
		fmt.Fprintf(os.Stderr,
			"Warning: Claude Code version %s is unsupported in the Agent SDK. "+
				"Minimum required version is %s. Some features may not work correctly.\n",
			version, minimumClaudeCodeVersion)
	}
}

func compareVersions(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var av, bv int
		if i < len(aParts) {
			av, _ = strconv.Atoi(aParts[i])
		}
		if i < len(bParts) {
			bv, _ = strconv.Atoi(bParts[i])
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
