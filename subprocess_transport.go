package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxBufferSize       = 1024 * 1024 // 1MB
	minimumClaudeCodeVersion   = "2.0.0"
	sdkVersion                 = "1.1.0"
)

// SubprocessTransport implements Transport using the Claude Code CLI subprocess.
type SubprocessTransport struct {
	options *Options
	cliPath string
	cwd     string

	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       io.ReadCloser
	stderr       io.ReadCloser
	ready        bool
	maxBufSize   int
	mu           sync.Mutex
	stdinClosed  bool
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
	// 2. System environment
	env = append(env, os.Environ()...)
	// 3. User-provided env vars (override defaults and system env)
	for k, v := range t.options.Env {
		env = append(env, k+"="+v)
	}
	// 4. SDK-controlled vars (never overridable)
	env = append(env, "CLAUDE_AGENT_SDK_VERSION="+sdkVersion)
	if t.options.EnableFileCheckpointing {
		env = append(env, "CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING=true")
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

	// Pipe stderr if callback is set or debug mode
	if t.options.Stderr != nil || t.hasExtraArg("debug-to-stderr") {
		t.stderr, err = cmd.StderrPipe()
		if err != nil {
			return &ConnectionError{SDKError: SDKError{Message: fmt.Sprintf("Failed to create stderr pipe: %v", err)}}
		}
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
			if err := t.cmd.Wait(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					code := exitErr.ExitCode()
					select {
					case ch <- map[string]any{
						"type":  "error",
						"error": fmt.Sprintf("Command failed with exit code %d", code),
					}:
					case <-ctx.Done():
					}
				}
			}
		}
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
		_ = t.cmd.Process.Signal(os.Interrupt)
		// Give it a moment to exit gracefully
		done := make(chan error, 1)
		go func() { done <- t.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = t.cmd.Process.Kill()
		}
	}

	t.cmd = nil
	t.stdout = nil
	t.stdin = nil
	t.stderr = nil
	return nil
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

func (t *SubprocessTransport) hasExtraArg(key string) bool {
	if t.options.ExtraArgs == nil {
		return false
	}
	_, ok := t.options.ExtraArgs[key]
	return ok
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
		if t.options.Stderr != nil {
			t.options.Stderr(line)
		}
	}
}

func (t *SubprocessTransport) buildCommand() []string {
	cmd := []string{t.cliPath, "--output-format", "stream-json", "--verbose"}

	opts := t.options

	// System prompt
	if opts.SystemPrompt == nil {
		cmd = append(cmd, "--system-prompt", "")
	} else {
		switch sp := opts.SystemPrompt.(type) {
		case StringPrompt:
			cmd = append(cmd, "--system-prompt", string(sp))
		case PresetPrompt:
			if sp.Append != "" {
				cmd = append(cmd, "--append-system-prompt", sp.Append)
			}
		}
	}

	// Tools
	if opts.Tools != nil {
		switch tools := opts.Tools.(type) {
		case []string:
			if len(tools) == 0 {
				cmd = append(cmd, "--tools", "")
			} else {
				cmd = append(cmd, "--tools", strings.Join(tools, ","))
			}
		case *ToolsPreset:
			cmd = append(cmd, "--tools", "default")
		}
	}

	if len(opts.AllowedTools) > 0 {
		cmd = append(cmd, "--allowedTools", strings.Join(opts.AllowedTools, ","))
	}

	if opts.MaxTurns != nil {
		cmd = append(cmd, "--max-turns", strconv.Itoa(*opts.MaxTurns))
	}

	if opts.MaxBudgetUSD != nil {
		cmd = append(cmd, "--max-budget-usd", strconv.FormatFloat(*opts.MaxBudgetUSD, 'f', -1, 64))
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

	// Settings and sandbox
	settingsValue := t.buildSettingsValue()
	if settingsValue != "" {
		cmd = append(cmd, "--settings", settingsValue)
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

	if opts.ForkSession {
		cmd = append(cmd, "--fork-session")
	}

	// Setting sources
	if opts.SettingSources != nil {
		sources := make([]string, len(opts.SettingSources))
		for i, s := range opts.SettingSources {
			sources[i] = string(s)
		}
		cmd = append(cmd, "--setting-sources", strings.Join(sources, ","))
	} else {
		cmd = append(cmd, "--setting-sources", "")
	}

	// Plugins
	for _, plugin := range opts.Plugins {
		if plugin.Type == "local" {
			cmd = append(cmd, "--plugin-dir", plugin.Path)
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
	resolvedMaxThinkingTokens := opts.MaxThinkingTokens
	if opts.Thinking != nil {
		switch tc := opts.Thinking.(type) {
		case ThinkingConfigAdaptive:
			if resolvedMaxThinkingTokens == nil {
				v := 32000
				resolvedMaxThinkingTokens = &v
			}
		case ThinkingConfigEnabled:
			resolvedMaxThinkingTokens = &tc.BudgetTokens
		case ThinkingConfigDisabled:
			v := 0
			resolvedMaxThinkingTokens = &v
		}
	}
	if resolvedMaxThinkingTokens != nil {
		cmd = append(cmd, "--max-thinking-tokens", strconv.Itoa(*resolvedMaxThinkingTokens))
	}

	if opts.Effort != "" {
		cmd = append(cmd, "--effort", string(opts.Effort))
	}

	// Output format / JSON schema
	if opts.OutputFormat != nil {
		if opts.OutputFormat["type"] == "json_schema" {
			if schema, ok := opts.OutputFormat["schema"]; ok {
				data, _ := json.Marshal(schema)
				cmd = append(cmd, "--json-schema", string(data))
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
