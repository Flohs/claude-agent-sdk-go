package claude

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	liteReadBufSize    = 65536
	maxSanitizedLength = 200
)

var (
	uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	skipFirstPromptPattern = regexp.MustCompile(
		`^(?:<local-command-stdout>|<session-start-hook>|<tick>|<goal>|` +
			`\[Request interrupted by user[^\]]*\]|` +
			`\s*<ide_opened_file>[\s\S]*</ide_opened_file>\s*$|` +
			`\s*<ide_selection>[\s\S]*</ide_selection>\s*$)`)

	commandNameRE = regexp.MustCompile(`<command-name>(.*?)</command-name>`)
	sanitizeRE    = regexp.MustCompile(`[^a-zA-Z0-9]`)
)

// ListSessionsOptions configures session listing.
type ListSessionsOptions struct {
	Directory        string
	Limit            *int
	Offset           int
	IncludeWorktrees bool // defaults to true
}

// ListSubagentsOptions configures subagent listing.
type ListSubagentsOptions struct {
	// Directory scopes the search to a specific project directory (and its
	// worktrees). When empty, all project directories are searched.
	Directory string
}

// GetSubagentMessagesOptions configures subagent transcript retrieval.
type GetSubagentMessagesOptions struct {
	Directory             string
	Limit                 *int
	Offset                int
	IncludeSystemMessages bool
}

// ListSubagents returns the agent IDs of subagents that wrote transcripts
// during the given session. Subagent transcripts live at
// `<projectDir>/<sessionId>/subagents/agent-<agentId>.jsonl` (and may be
// nested under subdirectories such as `workflows/<runId>/`).
//
// Returns an empty slice when the session is not found, the sessionID is
// not a valid UUID, or the session has no subagents.
func ListSubagents(sessionID string, opts ListSubagentsOptions) []string {
	if !isValidUUID(sessionID) {
		return nil
	}
	subagentsDir := resolveSubagentsDir(sessionID, opts.Directory)
	if subagentsDir == "" {
		return nil
	}
	files := collectAgentFiles(subagentsDir)
	ids := make([]string, 0, len(files))
	for _, af := range files {
		ids = append(ids, af.agentID)
	}
	return ids
}

// GetSubagentMessages reads a single subagent's conversation from its JSONL
// transcript. Messages are returned in chronological order with the same
// filtering behavior as GetSessionMessages (user/assistant by default;
// system entries added when IncludeSystemMessages is set).
func GetSubagentMessages(sessionID, agentID string, opts GetSubagentMessagesOptions) ([]SessionMessage, error) {
	if !isValidUUID(sessionID) {
		return nil, nil
	}
	subagentsDir := resolveSubagentsDir(sessionID, opts.Directory)
	if subagentsDir == "" {
		return nil, nil
	}
	var agentFile string
	for _, af := range collectAgentFiles(subagentsDir) {
		if af.agentID == agentID {
			agentFile = af.path
			break
		}
	}
	if agentFile == "" {
		return nil, nil
	}

	data, err := os.ReadFile(agentFile)
	if err != nil {
		return nil, err
	}

	entries := parseTranscriptEntries(string(data))
	return entriesToSessionMessages(entries, opts.IncludeSystemMessages, opts.Limit, opts.Offset), nil
}

// resolveSubagentsDir returns the on-disk path of the `subagents/` folder
// for a given session, or "" if the session file cannot be located.
func resolveSubagentsDir(sessionID, directory string) string {
	filePath := findSessionFilePath(sessionID, directory)
	if filePath == "" {
		return ""
	}
	// Strip the .jsonl suffix to derive the session directory.
	sessionDir := strings.TrimSuffix(filePath, ".jsonl")
	return filepath.Join(sessionDir, "subagents")
}

type agentFile struct {
	agentID string
	path    string
}

// collectAgentFiles walks a subagents directory tree and returns every
// `agent-<id>.jsonl` file it finds. Agent IDs are extracted from the
// filename. Results are sorted by filename for deterministic ordering.
func collectAgentFiles(baseDir string) []agentFile {
	var result []agentFile
	_ = filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "agent-"), ".jsonl")
		result = append(result, agentFile{agentID: id, path: path})
		return nil
	})
	return result
}

// GetSessionMessagesOptions configures session message retrieval.
type GetSessionMessagesOptions struct {
	Directory string
	Limit     *int
	Offset    int
	// IncludeSystemMessages includes system-subtype transcript entries
	// (hooks, summaries, status updates, etc.) in the returned slice.
	// Default is false, which matches the prior user+assistant-only
	// behavior.
	IncludeSystemMessages bool
}

// ListSessions lists sessions with metadata extracted from stat + head/tail reads.
func ListSessions(opts ListSessionsOptions) ([]SDKSessionInfo, error) {
	if opts.Directory != "" {
		includeWorktrees := opts.IncludeWorktrees
		return listSessionsForProject(opts.Directory, opts.Offset, opts.Limit, includeWorktrees), nil
	}
	return listAllSessions(opts.Offset, opts.Limit), nil
}

// GetSessionMessages reads a session's conversation messages from its JSONL transcript file.
func GetSessionMessages(sessionID string, opts GetSessionMessagesOptions) ([]SessionMessage, error) {
	if !isValidUUID(sessionID) {
		return nil, nil
	}

	content := readSessionFile(sessionID, opts.Directory)
	if content == "" {
		return nil, nil
	}

	entries := parseTranscriptEntries(content)
	return entriesToSessionMessages(entries, opts.IncludeSystemMessages, opts.Limit, opts.Offset), nil
}

// TagSession adds a tag to a session by appending a tag entry to the session's
// JSONL transcript file. The tag is sanitized to remove potentially problematic
// Unicode characters and normalized using NFKC.
func TagSession(sessionID string, tag *string, directory *string) error {
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}

	dir := ""
	if directory != nil {
		dir = *directory
	}

	filePath := findSessionFilePath(sessionID, dir)
	if filePath == "" {
		return fmt.Errorf("session file not found for session %s", sessionID)
	}

	sanitizedTag := ""
	if tag != nil {
		sanitizedTag = sanitizeTag(*tag)
	}

	entry := map[string]any{
		"type":      "tag",
		"tag":       sanitizedTag,
		"sessionId": sessionID,
	}

	return appendJSONLEntry(filePath, entry)
}

// RenameSession sets a custom title for a session by appending a custom-title
// entry to the session's JSONL transcript file.
func RenameSession(sessionID string, title string, directory *string) error {
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}

	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		return fmt.Errorf("title cannot be empty or whitespace-only")
	}

	dir := ""
	if directory != nil {
		dir = *directory
	}

	filePath := findSessionFilePath(sessionID, dir)
	if filePath == "" {
		return fmt.Errorf("session file not found for session %s", sessionID)
	}

	entry := map[string]any{
		"type":        "custom-title",
		"customTitle": trimmedTitle,
		"sessionId":   sessionID,
	}

	return appendJSONLEntry(filePath, entry)
}

// DeleteSession deletes a session's JSONL transcript file and any subagent
// transcripts stored in the sibling {session_id}/ directory.
//
// The JSONL file is a hard delete — an error is returned if it cannot be
// removed. The sibling directory (if any) is removed on a best-effort basis;
// failures there are swallowed so the primary delete still counts as a
// success. This mirrors the Python SDK's `shutil.rmtree(..., ignore_errors=True)`
// behavior.
func DeleteSession(sessionID string, directory ...string) error {
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}

	dir := ""
	if len(directory) > 0 {
		dir = directory[0]
	}

	filePath := findSessionFilePath(sessionID, dir)
	if filePath == "" {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if err := os.Remove(filePath); err != nil {
		return err
	}

	// Subagent transcripts live in a sibling {session_id}/ dir alongside the
	// .jsonl file. Often absent; remove best-effort.
	siblingDir := filepath.Join(filepath.Dir(filePath), sessionID)
	_ = os.RemoveAll(siblingDir)

	return nil
}

// ForkSession creates a copy of a session's transcript file with a new session ID.
// Returns the new session ID.
func ForkSession(sessionID string, directory ...string) (string, error) {
	if !isValidUUID(sessionID) {
		return "", fmt.Errorf("invalid session ID: %s", sessionID)
	}

	dir := ""
	if len(directory) > 0 {
		dir = directory[0]
	}

	sourcePath := findSessionFilePath(sessionID, dir)
	if sourcePath == "" {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}

	// Generate new UUID
	newID := generateUUID()

	// Copy file
	sourceDir := filepath.Dir(sourcePath)
	destPath := filepath.Join(sourceDir, newID+".jsonl")

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to read session file: %w", err)
	}

	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write forked session file: %w", err)
	}

	return newID, nil
}

// sanitizeTag removes potentially problematic Unicode characters and normalizes using NFKC.
func sanitizeTag(s string) string {
	// Apply NFKC normalization
	s = norm.NFKC.String(s)

	// Remove zero-width characters, directionality markers, and private-use characters
	s = strings.Map(func(r rune) rune {
		// Zero-width characters
		if r == '\u200B' || r == '\u200C' || r == '\u200D' || r == '\uFEFF' {
			return -1
		}
		// Directionality markers
		if r == '\u200E' || r == '\u200F' || (r >= '\u202A' && r <= '\u202E') ||
			(r >= '\u2066' && r <= '\u2069') {
			return -1
		}
		// Private-use characters
		if (r >= '\uE000' && r <= '\uF8FF') ||
			(r >= 0xF0000 && r <= 0xFFFFF) ||
			(r >= 0x100000 && r <= 0x10FFFF) {
			return -1
		}
		return r
	}, s)

	return strings.TrimSpace(s)
}

// Internal types and functions

type transcriptEntry = map[string]any

func isValidUUID(s string) bool {
	return uuidRE.MatchString(s)
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func simpleHash(s string) string {
	var h int32
	for _, ch := range s {
		h = (h << 5) - h + ch
	}
	if h < 0 {
		h = -h
	}
	if h == 0 {
		return "0"
	}
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	var out []byte
	n := h
	for n > 0 {
		out = append(out, digits[n%36])
		n /= 36
	}
	// reverse
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func sanitizePath(name string) string {
	sanitized := sanitizeRE.ReplaceAllString(name, "-")
	if len(sanitized) <= maxSanitizedLength {
		return sanitized
	}
	h := simpleHash(name)
	return fmt.Sprintf("%s-%s", sanitized[:maxSanitizedLength], h)
}

func normalizeNFC(s string) string {
	return norm.NFC.String(s)
}

func getClaudeConfigHomeDir() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return normalizeNFC(dir)
	}
	home, _ := os.UserHomeDir()
	return normalizeNFC(filepath.Join(home, ".claude"))
}

func getProjectsDir() string {
	return filepath.Join(getClaudeConfigHomeDir(), "projects")
}

// resolveProjectsDir resolves the CLI's projects directory using the same
// precedence the spawned subprocess will see: caller-provided
// Options.Env["CLAUDE_CONFIG_DIR"] beats the parent process's
// CLAUDE_CONFIG_DIR env, which beats the default $HOME/.claude. The
// returned path is the `projects/` subdirectory under the resolved
// CLAUDE_CONFIG_DIR. This must match where the CLI writes its JSONL —
// otherwise the mirror frame-peel cannot map filePath back to a SessionKey
// and silently drops every entry.
func resolveProjectsDir(envOverride map[string]string) string {
	if dir := envOverride["CLAUDE_CONFIG_DIR"]; dir != "" {
		return filepath.Join(normalizeNFC(dir), "projects")
	}
	return getProjectsDir()
}

func getProjectDir(projectPath string) string {
	return filepath.Join(getProjectsDir(), sanitizePath(projectPath))
}

func canonicalizePath(d string) string {
	resolved, err := filepath.EvalSymlinks(d)
	if err != nil {
		return normalizeNFC(d)
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return normalizeNFC(resolved)
	}
	return normalizeNFC(abs)
}

func findProjectDir(projectPath string) string {
	exact := getProjectDir(projectPath)
	if info, err := os.Stat(exact); err == nil && info.IsDir() {
		return exact
	}

	sanitized := sanitizePath(projectPath)
	if len(sanitized) <= maxSanitizedLength {
		return ""
	}

	prefix := sanitized[:maxSanitizedLength]
	projectsDir := getProjectsDir()
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix+"-") {
			return filepath.Join(projectsDir, entry.Name())
		}
	}
	return ""
}

type liteSessionFile struct {
	mtime int64
	size  int64
	head  string
	tail  string
}

func readSessionLite(filePath string) *liteSessionFile {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil
	}

	size := info.Size()
	mtime := info.ModTime().UnixMilli()

	headBuf := make([]byte, liteReadBufSize)
	n, err := f.Read(headBuf)
	if n == 0 || err != nil {
		return nil
	}
	head := string(headBuf[:n])

	var tail string
	tailOffset := size - int64(liteReadBufSize)
	if tailOffset <= 0 {
		tail = head
	} else {
		_, _ = f.Seek(tailOffset, 0)
		tailBuf := make([]byte, liteReadBufSize)
		n, _ := f.Read(tailBuf)
		tail = string(tailBuf[:n])
	}

	return &liteSessionFile{mtime: mtime, size: size, head: head, tail: tail}
}

func extractJSONStringField(text, key string) string {
	patterns := []string{
		fmt.Sprintf(`"%s":"`, key),
		fmt.Sprintf(`"%s": "`, key),
	}
	for _, pattern := range patterns {
		idx := strings.Index(text, pattern)
		if idx < 0 {
			continue
		}
		valueStart := idx + len(pattern)
		i := valueStart
		for i < len(text) {
			if text[i] == '\\' {
				i += 2
				continue
			}
			if text[i] == '"' {
				return unescapeJSONString(text[valueStart:i])
			}
			i++
		}
	}
	return ""
}

func extractLastJSONStringField(text, key string) string {
	patterns := []string{
		fmt.Sprintf(`"%s":"`, key),
		fmt.Sprintf(`"%s": "`, key),
	}
	var lastValue string
	for _, pattern := range patterns {
		searchFrom := 0
		for {
			idx := strings.Index(text[searchFrom:], pattern)
			if idx < 0 {
				break
			}
			idx += searchFrom
			valueStart := idx + len(pattern)
			i := valueStart
			for i < len(text) {
				if text[i] == '\\' {
					i += 2
					continue
				}
				if text[i] == '"' {
					lastValue = unescapeJSONString(text[valueStart:i])
					break
				}
				i++
			}
			searchFrom = i + 1
		}
	}
	return lastValue
}

func unescapeJSONString(raw string) string {
	if !strings.Contains(raw, `\`) {
		return raw
	}
	var result string
	if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, raw)), &result); err != nil {
		return raw
	}
	return result
}

func extractFirstPromptFromHead(head string) string {
	var commandFallback string
	lines := strings.Split(head, "\n")

	for _, line := range lines {
		if !strings.Contains(line, `"type":"user"`) && !strings.Contains(line, `"type": "user"`) {
			continue
		}
		if strings.Contains(line, `"tool_result"`) {
			continue
		}
		if strings.Contains(line, `"isMeta":true`) || strings.Contains(line, `"isMeta": true`) {
			continue
		}
		if strings.Contains(line, `"isCompactSummary":true`) || strings.Contains(line, `"isCompactSummary": true`) {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["type"] != "user" {
			continue
		}

		message, ok := entry["message"].(map[string]any)
		if !ok {
			continue
		}

		var texts []string
		switch content := message["content"].(type) {
		case string:
			texts = append(texts, content)
		case []any:
			for _, block := range content {
				if bm, ok := block.(map[string]any); ok {
					if bm["type"] == "text" {
						if t, ok := bm["text"].(string); ok {
							texts = append(texts, t)
						}
					}
				}
			}
		}

		for _, raw := range texts {
			result := strings.Map(func(r rune) rune {
				if r == '\n' {
					return ' '
				}
				return r
			}, raw)
			result = strings.TrimSpace(result)
			if result == "" {
				continue
			}

			if m := commandNameRE.FindStringSubmatch(result); m != nil {
				if commandFallback == "" {
					commandFallback = m[1]
				}
				continue
			}

			if skipFirstPromptPattern.MatchString(result) {
				continue
			}

			if len([]rune(result)) > 200 {
				result = string([]rune(result)[:200])
				result = strings.TrimRightFunc(result, unicode.IsSpace) + "\u2026"
			}
			return result
		}
	}

	return commandFallback
}

func readSessionsFromDir(projectDir string, projectPath string) []SDKSessionInfo {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil
	}

	var results []SDKSessionInfo
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		sessionID := name[:len(name)-6]
		if !isValidUUID(sessionID) {
			continue
		}

		lite := readSessionLite(filepath.Join(projectDir, name))
		if lite == nil {
			continue
		}

		// Check first line for sidechain
		firstNewline := strings.Index(lite.head, "\n")
		firstLine := lite.head
		if firstNewline >= 0 {
			firstLine = lite.head[:firstNewline]
		}
		if strings.Contains(firstLine, `"isSidechain":true`) || strings.Contains(firstLine, `"isSidechain": true`) {
			continue
		}

		customTitle := extractLastJSONStringField(lite.tail, "customTitle")
		firstPrompt := extractFirstPromptFromHead(lite.head)

		summary := customTitle
		if summary == "" {
			summary = extractLastJSONStringField(lite.tail, "summary")
		}
		if summary == "" {
			summary = firstPrompt
		}
		if summary == "" {
			continue
		}

		gitBranch := extractLastJSONStringField(lite.tail, "gitBranch")
		if gitBranch == "" {
			gitBranch = extractJSONStringField(lite.head, "gitBranch")
		}

		sessionCwd := extractJSONStringField(lite.head, "cwd")
		if sessionCwd == "" {
			sessionCwd = projectPath
		}

		tag := extractTagFromTranscript(lite.head, lite.tail)
		createdAt := extractCreatedAtFromHead(lite.head)
		fileSize := lite.size

		results = append(results, SDKSessionInfo{
			SessionID:    sessionID,
			Summary:      summary,
			LastModified: lite.mtime,
			FileSize:     &fileSize,
			CustomTitle:  customTitle,
			FirstPrompt:  firstPrompt,
			GitBranch:    gitBranch,
			Cwd:          sessionCwd,
			Tag:          tag,
			CreatedAt:    createdAt,
		})
	}

	return results
}

func getWorktreePaths(cwd string) []string {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			path := normalizeNFC(line[len("worktree "):])
			paths = append(paths, path)
		}
	}
	return paths
}

func deduplicateBySessionID(sessions []SDKSessionInfo) []SDKSessionInfo {
	byID := make(map[string]SDKSessionInfo)
	for _, s := range sessions {
		if existing, ok := byID[s.SessionID]; !ok || s.LastModified > existing.LastModified {
			byID[s.SessionID] = s
		}
	}
	result := make([]SDKSessionInfo, 0, len(byID))
	for _, s := range byID {
		result = append(result, s)
	}
	return result
}

func applySortAndLimit(sessions []SDKSessionInfo, offset int, limit *int) []SDKSessionInfo {
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastModified > sessions[j].LastModified
	})
	if offset > 0 {
		if offset >= len(sessions) {
			return nil
		}
		sessions = sessions[offset:]
	}
	if limit != nil && *limit > 0 && *limit < len(sessions) {
		sessions = sessions[:*limit]
	}
	return sessions
}

func listSessionsForProject(directory string, offset int, limit *int, includeWorktrees bool) []SDKSessionInfo {
	canonicalDir := canonicalizePath(directory)

	var worktreePaths []string
	if includeWorktrees {
		worktreePaths = getWorktreePaths(canonicalDir)
	}

	if len(worktreePaths) <= 1 {
		projectDir := findProjectDir(canonicalDir)
		if projectDir == "" {
			return nil
		}
		sessions := readSessionsFromDir(projectDir, canonicalDir)
		return applySortAndLimit(sessions, offset, limit)
	}

	// Worktree-aware scanning
	projectsDir := getProjectsDir()
	allDirents, err := os.ReadDir(projectsDir)
	if err != nil {
		projectDir := findProjectDir(canonicalDir)
		if projectDir == "" {
			return nil
		}
		sessions := readSessionsFromDir(projectDir, canonicalDir)
		return applySortAndLimit(sessions, offset, limit)
	}

	var allSessions []SDKSessionInfo
	seenDirs := make(map[string]bool)

	// Always include the user's actual directory
	canonicalProjectDir := findProjectDir(canonicalDir)
	if canonicalProjectDir != "" {
		dirBase := filepath.Base(canonicalProjectDir)
		seenDirs[dirBase] = true
		sessions := readSessionsFromDir(canonicalProjectDir, canonicalDir)
		allSessions = append(allSessions, sessions...)
	}

	type indexedWT struct {
		path   string
		prefix string
	}
	indexed := make([]indexedWT, 0, len(worktreePaths))
	for _, wt := range worktreePaths {
		indexed = append(indexed, indexedWT{path: wt, prefix: sanitizePath(wt)})
	}
	sort.Slice(indexed, func(i, j int) bool {
		return len(indexed[i].prefix) > len(indexed[j].prefix)
	})

	for _, entry := range allDirents {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		if seenDirs[dirName] {
			continue
		}

		for _, iwt := range indexed {
			isMatch := dirName == iwt.prefix ||
				(len(iwt.prefix) >= maxSanitizedLength && strings.HasPrefix(dirName, iwt.prefix[:maxSanitizedLength]+"-"))
			if isMatch {
				seenDirs[dirName] = true
				sessions := readSessionsFromDir(filepath.Join(projectsDir, dirName), iwt.path)
				allSessions = append(allSessions, sessions...)
				break
			}
		}
	}

	deduped := deduplicateBySessionID(allSessions)
	return applySortAndLimit(deduped, offset, limit)
}

func listAllSessions(offset int, limit *int) []SDKSessionInfo {
	projectsDir := getProjectsDir()
	projectDirs, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}

	var allSessions []SDKSessionInfo
	for _, entry := range projectDirs {
		if !entry.IsDir() {
			continue
		}
		sessions := readSessionsFromDir(filepath.Join(projectsDir, entry.Name()), "")
		allSessions = append(allSessions, sessions...)
	}

	deduped := deduplicateBySessionID(allSessions)
	return applySortAndLimit(deduped, offset, limit)
}

func readSessionFile(sessionID string, directory string) string {
	fileName := sessionID + ".jsonl"

	if directory != "" {
		canonicalDir := canonicalizePath(directory)

		projectDir := findProjectDir(canonicalDir)
		if projectDir != "" {
			content := tryReadSessionFile(projectDir, fileName)
			if content != "" {
				return content
			}
		}

		for _, wt := range getWorktreePaths(canonicalDir) {
			if wt == canonicalDir {
				continue
			}
			wtProjectDir := findProjectDir(wt)
			if wtProjectDir != "" {
				content := tryReadSessionFile(wtProjectDir, fileName)
				if content != "" {
					return content
				}
			}
		}
		return ""
	}

	// Search all project directories
	projectsDir := getProjectsDir()
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		content := tryReadSessionFile(filepath.Join(projectsDir, entry.Name()), fileName)
		if content != "" {
			return content
		}
	}
	return ""
}

func tryReadSessionFile(projectDir, fileName string) string {
	data, err := os.ReadFile(filepath.Join(projectDir, fileName))
	if err != nil {
		return ""
	}
	return string(data)
}

var transcriptEntryTypes = map[string]bool{
	"user": true, "assistant": true, "progress": true, "system": true, "attachment": true,
}

func parseTranscriptEntries(content string) []transcriptEntry {
	var entries []transcriptEntry
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entryType, _ := entry["type"].(string)
		uuid, _ := entry["uuid"].(string)
		if transcriptEntryTypes[entryType] && uuid != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

// filterTranscriptEntries is the already-parsed-object counterpart of
// parseTranscriptEntries. It keeps only entries whose "type" is in
// transcriptEntryTypes and whose "uuid" is a non-empty string, so
// chain-building never sees metadata-only entries (custom-title, tag,
// agent_metadata, etc.). Shared by the filesystem path (which parses JSONL
// first) and the [SessionStore] path (which gets objects directly).
func filterTranscriptEntries(entries []SessionStoreEntry) []transcriptEntry {
	result := make([]transcriptEntry, 0, len(entries))
	for _, e := range entries {
		if e == nil {
			continue
		}
		entryType, _ := e["type"].(string)
		uuid, _ := e["uuid"].(string)
		if transcriptEntryTypes[entryType] && uuid != "" {
			result = append(result, e)
		}
	}
	return result
}

// entriesToSessionMessages runs the shared pipeline used by both the
// filesystem and [SessionStore]-backed session message readers: build the
// conversation chain, filter to visible user/assistant (and optional
// system) messages, and apply offset/limit.
func entriesToSessionMessages(entries []transcriptEntry, includeSystem bool, limit *int, offset int) []SessionMessage {
	chain := buildConversationChain(entries)

	var visible []transcriptEntry
	for _, e := range chain {
		if isVisibleMessage(e) {
			visible = append(visible, e)
			continue
		}
		if includeSystem && isVisibleSystemMessage(e) {
			visible = append(visible, e)
		}
	}

	messages := make([]SessionMessage, len(visible))
	for i, e := range visible {
		messages[i] = toSessionMessage(e)
	}

	if offset > 0 {
		if offset >= len(messages) {
			return nil
		}
		messages = messages[offset:]
	}
	if limit != nil && *limit > 0 && *limit < len(messages) {
		messages = messages[:*limit]
	}
	return messages
}

func buildConversationChain(entries []transcriptEntry) []transcriptEntry {
	if len(entries) == 0 {
		return nil
	}

	byUUID := make(map[string]transcriptEntry, len(entries))
	entryIndex := make(map[string]int, len(entries))
	for i, e := range entries {
		uuid, _ := e["uuid"].(string)
		byUUID[uuid] = e
		entryIndex[uuid] = i
	}

	// Find terminal messages
	parentUUIDs := make(map[string]bool)
	for _, e := range entries {
		if p, ok := e["parentUuid"].(string); ok && p != "" {
			parentUUIDs[p] = true
		}
	}

	var terminals []transcriptEntry
	for _, e := range entries {
		uuid, _ := e["uuid"].(string)
		if !parentUUIDs[uuid] {
			terminals = append(terminals, e)
		}
	}

	// Find leaves (walk back from terminals to user/assistant)
	var leaves []transcriptEntry
	for _, terminal := range terminals {
		seen := make(map[string]bool)
		cur := terminal
		for {
			uuid, _ := cur["uuid"].(string)
			if seen[uuid] {
				break
			}
			seen[uuid] = true
			entryType, _ := cur["type"].(string)
			if entryType == "user" || entryType == "assistant" {
				leaves = append(leaves, cur)
				break
			}
			parent, _ := cur["parentUuid"].(string)
			if parent == "" {
				break
			}
			next, ok := byUUID[parent]
			if !ok {
				break
			}
			cur = next
		}
	}

	if len(leaves) == 0 {
		return nil
	}

	// Pick best leaf (not sidechain/team/meta, highest index)
	var mainLeaves []transcriptEntry
	for _, leaf := range leaves {
		isSidechain, _ := leaf["isSidechain"].(bool)
		_, hasTeam := leaf["teamName"].(string)
		isMeta, _ := leaf["isMeta"].(bool)
		if !isSidechain && !hasTeam && !isMeta {
			mainLeaves = append(mainLeaves, leaf)
		}
	}

	pickBest := func(candidates []transcriptEntry) transcriptEntry {
		best := candidates[0]
		bestUUID, _ := best["uuid"].(string)
		bestIdx := entryIndex[bestUUID]
		for _, c := range candidates[1:] {
			cUUID, _ := c["uuid"].(string)
			cIdx := entryIndex[cUUID]
			if cIdx > bestIdx {
				best = c
				bestIdx = cIdx
			}
		}
		return best
	}

	var leaf transcriptEntry
	if len(mainLeaves) > 0 {
		leaf = pickBest(mainLeaves)
	} else {
		leaf = pickBest(leaves)
	}

	// Walk from leaf to root
	var chain []transcriptEntry
	seen := make(map[string]bool)
	cur := leaf
	for {
		uuid, _ := cur["uuid"].(string)
		if seen[uuid] {
			break
		}
		seen[uuid] = true
		chain = append(chain, cur)
		parent, _ := cur["parentUuid"].(string)
		if parent == "" {
			break
		}
		next, ok := byUUID[parent]
		if !ok {
			break
		}
		cur = next
	}

	// Reverse to chronological order
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func isVisibleMessage(entry transcriptEntry) bool {
	entryType, _ := entry["type"].(string)
	if entryType != "user" && entryType != "assistant" {
		return false
	}
	if isMeta, _ := entry["isMeta"].(bool); isMeta {
		return false
	}
	if isSidechain, _ := entry["isSidechain"].(bool); isSidechain {
		return false
	}
	_, hasTeam := entry["teamName"].(string)
	return !hasTeam
}

func isVisibleSystemMessage(entry transcriptEntry) bool {
	entryType, _ := entry["type"].(string)
	if entryType != "system" {
		return false
	}
	if isMeta, _ := entry["isMeta"].(bool); isMeta {
		return false
	}
	if isSidechain, _ := entry["isSidechain"].(bool); isSidechain {
		return false
	}
	return true
}

func toSessionMessage(entry transcriptEntry) SessionMessage {
	entryType, _ := entry["type"].(string)
	msgType := "user"
	switch entryType {
	case "assistant":
		msgType = "assistant"
	case "system":
		msgType = "system"
	}
	uuid, _ := entry["uuid"].(string)
	sessionID, _ := entry["sessionId"].(string)
	return SessionMessage{
		Type:      msgType,
		UUID:      uuid,
		SessionID: sessionID,
		Message:   entry["message"],
	}
}

// extractTagFromTranscript scans transcript head and tail for the last {"type":"tag"} entry
// and returns the tag value. Returns nil if no tag entry is found.
func extractTagFromTranscript(head, tail string) *string {
	// Search tail first (more recent), then head
	for _, section := range []string{tail, head} {
		var lastTag *string
		for _, line := range strings.Split(section, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Quick check: must contain "type" and "tag" to be a tag entry
			if !strings.Contains(line, `"type"`) {
				continue
			}
			if !strings.Contains(line, `"tag"`) {
				continue
			}
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			if entry["type"] != "tag" {
				continue
			}
			if tagVal, ok := entry["tag"].(string); ok {
				lastTag = &tagVal
			}
		}
		if lastTag != nil {
			return lastTag
		}
	}
	return nil
}

// extractCreatedAtFromHead extracts the timestamp from the first transcript entry.
// Returns the timestamp in Unix milliseconds, or nil if not found.
func extractCreatedAtFromHead(head string) *int64 {
	for _, line := range strings.Split(head, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		// Look for a timestamp field
		if ts, ok := entry["timestamp"].(string); ok && ts != "" {
			// Parse ISO 8601 timestamp to Unix milliseconds
			for _, layout := range []string{
				"2006-01-02T15:04:05.000Z",
				"2006-01-02T15:04:05Z",
				"2006-01-02T15:04:05.000-07:00",
				"2006-01-02T15:04:05-07:00",
			} {
				if t, err := time.Parse(layout, ts); err == nil {
					ms := t.UnixMilli()
					return &ms
				}
			}
		}
		// Also check for numeric timestamp
		if ts, ok := entry["timestamp"].(float64); ok {
			ms := int64(ts)
			return &ms
		}
		// Only check the first valid JSON line
		return nil
	}
	return nil
}

// GetSessionInfo retrieves session metadata for a single session by its ID.
// It locates the JSONL file and parses it to extract metadata including
// tag, created_at, summary, and other fields.
// Returns nil and an error if the session is not found.
func GetSessionInfo(sessionID string, directory ...string) (*SDKSessionInfo, error) {
	if !isValidUUID(sessionID) {
		return nil, fmt.Errorf("invalid session ID: %s", sessionID)
	}

	dir := ""
	if len(directory) > 0 {
		dir = directory[0]
	}

	filePath := findSessionFilePath(sessionID, dir)
	if filePath == "" {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	lite := readSessionLite(filePath)
	if lite == nil {
		return nil, fmt.Errorf("failed to read session file: %s", sessionID)
	}

	// Check for sidechain
	firstNewline := strings.Index(lite.head, "\n")
	firstLine := lite.head
	if firstNewline >= 0 {
		firstLine = lite.head[:firstNewline]
	}
	if strings.Contains(firstLine, `"isSidechain":true`) || strings.Contains(firstLine, `"isSidechain": true`) {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	customTitle := extractLastJSONStringField(lite.tail, "customTitle")
	firstPrompt := extractFirstPromptFromHead(lite.head)

	summary := customTitle
	if summary == "" {
		summary = extractLastJSONStringField(lite.tail, "summary")
	}
	if summary == "" {
		summary = firstPrompt
	}

	gitBranch := extractLastJSONStringField(lite.tail, "gitBranch")
	if gitBranch == "" {
		gitBranch = extractJSONStringField(lite.head, "gitBranch")
	}

	sessionCwd := extractJSONStringField(lite.head, "cwd")

	tag := extractTagFromTranscript(lite.head, lite.tail)
	createdAt := extractCreatedAtFromHead(lite.head)
	fileSize := lite.size

	return &SDKSessionInfo{
		SessionID:    sessionID,
		Summary:      summary,
		LastModified: lite.mtime,
		FileSize:     &fileSize,
		CustomTitle:  customTitle,
		FirstPrompt:  firstPrompt,
		GitBranch:    gitBranch,
		Cwd:          sessionCwd,
		Tag:          tag,
		CreatedAt:    createdAt,
	}, nil
}

// findSessionFilePath locates the JSONL file for a given session ID.
func findSessionFilePath(sessionID string, directory string) string {
	fileName := sessionID + ".jsonl"

	if directory != "" {
		canonicalDir := canonicalizePath(directory)

		projectDir := findProjectDir(canonicalDir)
		if projectDir != "" {
			path := filepath.Join(projectDir, fileName)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}

		for _, wt := range getWorktreePaths(canonicalDir) {
			if wt == canonicalDir {
				continue
			}
			wtProjectDir := findProjectDir(wt)
			if wtProjectDir != "" {
				path := filepath.Join(wtProjectDir, fileName)
				if _, err := os.Stat(path); err == nil {
					return path
				}
			}
		}
		return ""
	}

	// Search all project directories
	projectsDir := getProjectsDir()
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		path := filepath.Join(projectsDir, entry.Name(), fileName)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// appendJSONLEntry appends a JSON object as a new line to a JSONL file.
func appendJSONLEntry(filePath string, entry map[string]any) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open session file: %w", err)
	}
	defer func() { _ = f.Close() }()

	_, err = f.Write(append([]byte("\n"), append(data, '\n')...))
	if err != nil {
		return fmt.Errorf("failed to write to session file: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// SessionStore-backed read helpers
// ---------------------------------------------------------------------------

// ListSessionsFromStore lists sessions from a [SessionStore]. Store-backed
// counterpart to [ListSessions].
//
// When store implements [SessionStoreSummarizer], this takes the fast path:
// one batched summaries call converts each [SessionSummaryEntry] directly
// into an [SDKSessionInfo] without per-session [SessionStore.Load]. When the
// store also implements [SessionStoreLister], the slow-path gap-fill covers
// sessions whose sidecar is absent or stale (summary.Mtime < list.Mtime) so
// the combined result matches a full Load-per-session view.
//
// When store implements only [SessionStoreLister] (no summarizer), every
// listed session is loaded via [SessionStore.Load] and parsed through the
// same lite-parse used by the filesystem path.
//
// opts.Directory is resolved to a project_key via [ProjectKeyForDirectory]
// (defaults to the current working directory when empty). opts.IncludeWorktrees
// is a filesystem concept and is not honored on the store path — the store
// operates on a single project_key.
//
// Results are sorted by LastModified descending and then offset/limit are
// applied. Returns a non-nil zero-length slice on an empty store, matching
// the filesystem helper.
//
// Returns an error when store implements neither [SessionStoreSummarizer]
// nor [SessionStoreLister] — without at least one of those methods the SDK
// cannot enumerate sessions.
func ListSessionsFromStore(ctx context.Context, store SessionStore, opts ListSessionsOptions) ([]SDKSessionInfo, error) {
	if store == nil {
		return nil, fmt.Errorf("session store is nil")
	}
	dir := opts.Directory
	if dir == "" {
		dir = "."
	}
	projectPath := canonicalizePath(dir)
	projectKey := sanitizePath(projectPath)

	summarizer, hasSummarizer := store.(SessionStoreSummarizer)
	lister, hasLister := store.(SessionStoreLister)

	if hasSummarizer {
		summaries, err := summarizer.ListSessionSummaries(ctx, projectKey)
		if err != nil {
			return nil, err
		}

		// Gap-fill requires the lister. Without it, sessions missing a
		// sidecar can't be discovered.
		var knownMtimes map[string]int64
		var listing []SessionStoreListEntry
		if hasLister {
			listing, err = lister.ListSessions(ctx, projectKey)
			if err != nil {
				return nil, err
			}
			knownMtimes = make(map[string]int64, len(listing))
			for _, e := range listing {
				knownMtimes[e.SessionID] = e.Mtime
			}
		}

		type slot struct {
			mtime     int64
			sessionID string
			info      *SDKSessionInfo // nil means gap-fill via Load
		}

		var slots []slot
		fresh := make(map[string]bool)
		for _, s := range summaries {
			if hasLister {
				known, ok := knownMtimes[s.SessionID]
				if !ok {
					// Summary for a session ListSessions no longer
					// reports — drop it.
					continue
				}
				if s.Mtime < known {
					// Stale sidecar — let gap-fill re-fold from source.
					continue
				}
			}
			info := summaryEntryToSDKInfo(s, projectPath)
			fresh[s.SessionID] = true
			if info == nil {
				// Sidechain or no-summary summary: pre-filter (free —
				// we already know) so it doesn't consume an
				// offset/limit position.
				continue
			}
			slots = append(slots, slot{mtime: s.Mtime, sessionID: s.SessionID, info: info})
		}
		if hasLister {
			for _, e := range listing {
				if !fresh[e.SessionID] {
					slots = append(slots, slot{mtime: e.Mtime, sessionID: e.SessionID, info: nil})
				}
			}
		}

		// Paginate BEFORE per-session load so gap-fill Load count is
		// bounded by page size, not total missing.
		sort.Slice(slots, func(i, j int) bool { return slots[i].mtime > slots[j].mtime })
		page := slots
		if opts.Offset > 0 {
			if opts.Offset >= len(page) {
				return []SDKSessionInfo{}, nil
			}
			page = page[opts.Offset:]
		}
		if opts.Limit != nil && *opts.Limit > 0 && *opts.Limit < len(page) {
			page = page[:*opts.Limit]
		}

		// Fill placeholders via per-session Load.
		for i := range page {
			if page[i].info != nil {
				continue
			}
			info, err := loadAndDeriveSessionInfo(ctx, store, projectKey, page[i].sessionID, page[i].mtime, projectPath)
			if err != nil {
				return nil, err
			}
			page[i].info = info
		}

		// Drop placeholders whose Load yielded no extractable summary
		// (sidechain / empty). This can short-page, mirroring the disk
		// path's filter-then-drop behavior.
		out := make([]SDKSessionInfo, 0, len(page))
		for _, sl := range page {
			if sl.info != nil {
				out = append(out, *sl.info)
			}
		}
		return out, nil
	}

	if !hasLister {
		return nil, fmt.Errorf("session store implements neither SessionStoreSummarizer nor SessionStoreLister -- cannot list sessions")
	}

	listing, err := lister.ListSessions(ctx, projectKey)
	if err != nil {
		return nil, err
	}

	results := make([]SDKSessionInfo, 0, len(listing))
	for _, e := range listing {
		info, err := loadAndDeriveSessionInfo(ctx, store, projectKey, e.SessionID, e.Mtime, projectPath)
		if err != nil {
			return nil, err
		}
		if info != nil {
			results = append(results, *info)
		}
	}

	return applySortAndLimit(results, opts.Offset, opts.Limit), nil
}

// summaryEntryToSDKInfo converts a [SessionSummaryEntry] to an
// [SDKSessionInfo] by reading the opaque Data map produced by
// [FoldSessionSummary]. Returns nil for sidechain sessions or sessions with
// no extractable summary, matching readSessionsFromDir's filtering.
func summaryEntryToSDKInfo(entry SessionSummaryEntry, projectPath string) *SDKSessionInfo {
	data := entry.Data
	if data == nil {
		return nil
	}
	if sc, _ := data["is_sidechain"].(bool); sc {
		return nil
	}

	locked, _ := data["first_prompt_locked"].(bool)
	var firstPrompt string
	if locked {
		firstPrompt, _ = data["first_prompt"].(string)
	} else {
		firstPrompt, _ = data["command_fallback"].(string)
	}

	customTitle, _ := data["custom_title"].(string)
	if customTitle == "" {
		customTitle, _ = data["ai_title"].(string)
	}

	summary := customTitle
	if summary == "" {
		summary, _ = data["last_prompt"].(string)
	}
	if summary == "" {
		summary, _ = data["summary_hint"].(string)
	}
	if summary == "" {
		summary = firstPrompt
	}
	if summary == "" {
		return nil
	}

	gitBranch, _ := data["git_branch"].(string)

	cwd, _ := data["cwd"].(string)
	if cwd == "" {
		cwd = projectPath
	}

	var tagPtr *string
	if tagStr, ok := data["tag"].(string); ok && tagStr != "" {
		tagCopy := tagStr
		tagPtr = &tagCopy
	}

	var createdAtPtr *int64
	switch v := data["created_at"].(type) {
	case int64:
		cp := v
		createdAtPtr = &cp
	case float64:
		cp := int64(v)
		createdAtPtr = &cp
	}

	return &SDKSessionInfo{
		SessionID:    entry.SessionID,
		Summary:      summary,
		LastModified: entry.Mtime,
		// FileSize is a JSONL byte count meaningful only for the
		// local-disk path — stores have no equivalent.
		FileSize:    nil,
		CustomTitle: customTitle,
		FirstPrompt: firstPrompt,
		GitBranch:   gitBranch,
		Cwd:         cwd,
		Tag:         tagPtr,
		CreatedAt:   createdAtPtr,
	}
}

// loadAndDeriveSessionInfo loads a session's entries via store.Load and
// derives its SDKSessionInfo by running the entries through the same
// lite-parse the filesystem path uses. Returns (nil, nil) when the session
// has no entries or yields no extractable summary (sidechain / empty),
// matching readSessionsFromDir's drop semantics.
func loadAndDeriveSessionInfo(ctx context.Context, store SessionStore, projectKey, sessionID string, mtime int64, projectPath string) (*SDKSessionInfo, error) {
	entries, err := store.Load(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	jsonl := entriesToJSONL(entries)
	lite := jsonlToLite(jsonl, mtime)
	return parseSessionInfoFromLite(sessionID, lite, projectPath), nil
}

// entriesToJSONL serializes store entries to a JSONL string. It hoists
// "type" to the front of each object so byte-level scanners like
// extractTagFromTranscript (which looks for `"type":"tag"` as a substring
// anchored near the start of a line) see the same byte shape the on-disk
// path produces, even if an adapter (e.g. Postgres JSONB) reordered object
// keys.
func entriesToJSONL(entries []SessionStoreEntry) string {
	var buf strings.Builder
	for _, e := range entries {
		if e == nil {
			continue
		}
		// If "type" present, emit it first.
		if typeVal, ok := e["type"]; ok {
			reordered := make(map[string]any, len(e))
			reordered["type"] = typeVal
			for k, v := range e {
				if k == "type" {
					continue
				}
				reordered[k] = v
			}
			if data, err := json.Marshal(reordered); err == nil {
				buf.Write(data)
				buf.WriteByte('\n')
				continue
			}
		}
		if data, err := json.Marshal(e); err == nil {
			buf.Write(data)
			buf.WriteByte('\n')
		}
	}
	return buf.String()
}

// jsonlToLite builds a liteSessionFile from an in-memory JSONL string,
// matching readSessionLite's byte semantics so the store path exposes the
// same head/tail slice to parseSessionInfoFromLite as the disk path would
// for the same transcript.
func jsonlToLite(jsonl string, mtime int64) *liteSessionFile {
	buf := []byte(jsonl)
	size := int64(len(buf))
	headEnd := liteReadBufSize
	if int64(headEnd) > size {
		headEnd = int(size)
	}
	head := string(buf[:headEnd])
	var tail string
	if size > liteReadBufSize {
		tail = string(buf[size-liteReadBufSize:])
	} else {
		tail = head
	}
	return &liteSessionFile{mtime: mtime, size: size, head: head, tail: tail}
}

// parseSessionInfoFromLite runs the same lite-parse as readSessionsFromDir
// on a single liteSessionFile and returns the resulting SDKSessionInfo, or
// nil for sidechain / no-summary sessions.
func parseSessionInfoFromLite(sessionID string, lite *liteSessionFile, projectPath string) *SDKSessionInfo {
	firstNewline := strings.Index(lite.head, "\n")
	firstLine := lite.head
	if firstNewline >= 0 {
		firstLine = lite.head[:firstNewline]
	}
	if strings.Contains(firstLine, `"isSidechain":true`) || strings.Contains(firstLine, `"isSidechain": true`) {
		return nil
	}

	customTitle := extractLastJSONStringField(lite.tail, "customTitle")
	firstPrompt := extractFirstPromptFromHead(lite.head)

	summary := customTitle
	if summary == "" {
		summary = extractLastJSONStringField(lite.tail, "summary")
	}
	if summary == "" {
		summary = firstPrompt
	}
	if summary == "" {
		return nil
	}

	gitBranch := extractLastJSONStringField(lite.tail, "gitBranch")
	if gitBranch == "" {
		gitBranch = extractJSONStringField(lite.head, "gitBranch")
	}

	sessionCwd := extractJSONStringField(lite.head, "cwd")
	if sessionCwd == "" {
		sessionCwd = projectPath
	}

	tag := extractTagFromTranscript(lite.head, lite.tail)
	createdAt := extractCreatedAtFromHead(lite.head)
	fileSize := lite.size

	return &SDKSessionInfo{
		SessionID:    sessionID,
		Summary:      summary,
		LastModified: lite.mtime,
		FileSize:     &fileSize,
		CustomTitle:  customTitle,
		FirstPrompt:  firstPrompt,
		GitBranch:    gitBranch,
		Cwd:          sessionCwd,
		Tag:          tag,
		CreatedAt:    createdAt,
	}
}

// GetSessionMessagesFromStore reads a session's conversation messages from
// a [SessionStore]. Store-backed counterpart to [GetSessionMessages].
//
// Feeds [SessionStore.Load] results directly into the shared chain builder
// — no JSONL round-trip. opts.Directory is resolved to a project_key via
// [ProjectKeyForDirectory] (defaults to the current working directory when
// empty).
//
// Returns (nil, nil) when sessionID is not a valid UUID or the session has
// no entries. Errors come only from [SessionStore.Load].
func GetSessionMessagesFromStore(ctx context.Context, store SessionStore, sessionID string, opts GetSessionMessagesOptions) ([]SessionMessage, error) {
	if store == nil {
		return nil, fmt.Errorf("session store is nil")
	}
	if !isValidUUID(sessionID) {
		return nil, nil
	}
	dir := opts.Directory
	if dir == "" {
		dir = "."
	}
	projectKey := sanitizePath(canonicalizePath(dir))

	entries, err := store.Load(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	filtered := filterTranscriptEntries(entries)
	return entriesToSessionMessages(filtered, opts.IncludeSystemMessages, opts.Limit, opts.Offset), nil
}

// ListSubagentsFromStore lists subagent IDs for a session from a
// [SessionStore]. Store-backed counterpart to [ListSubagents].
//
// Requires store to implement [SessionStoreSubkeys]; returns an error
// otherwise. Subkeys of the shape "subagents/agent-<id>" or nested
// equivalents such as "subagents/workflows/<runId>/agent-<id>" are
// recognised and their agent IDs are extracted. Duplicate IDs across
// nested layouts are deduplicated.
//
// opts.Directory is resolved to a project_key via [ProjectKeyForDirectory]
// (defaults to the current working directory when empty).
//
// Returns (nil, nil) when sessionID is not a valid UUID. An empty slice (or
// nil) is returned when the session has no subagents.
func ListSubagentsFromStore(ctx context.Context, store SessionStore, sessionID string, opts ListSubagentsOptions) ([]string, error) {
	if store == nil {
		return nil, fmt.Errorf("session store is nil")
	}
	if !isValidUUID(sessionID) {
		return nil, nil
	}
	sub, ok := store.(SessionStoreSubkeys)
	if !ok {
		return nil, fmt.Errorf("session store does not implement SessionStoreSubkeys -- cannot list subagents")
	}
	dir := opts.Directory
	if dir == "" {
		dir = "."
	}
	projectKey := sanitizePath(canonicalizePath(dir))
	subkeys, err := sub.ListSubkeys(ctx, SessionListSubkeysKey{ProjectKey: projectKey, SessionID: sessionID})
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var ids []string
	for _, subpath := range subkeys {
		if !strings.HasPrefix(subpath, "subagents/") {
			continue
		}
		last := subpath
		if idx := strings.LastIndex(subpath, "/"); idx >= 0 {
			last = subpath[idx+1:]
		}
		if !strings.HasPrefix(last, "agent-") {
			continue
		}
		agentID := strings.TrimPrefix(last, "agent-")
		if agentID == "" || seen[agentID] {
			continue
		}
		seen[agentID] = true
		ids = append(ids, agentID)
	}
	return ids, nil
}

// GetSubagentMessagesFromStore reads a subagent's conversation messages
// from a [SessionStore]. Store-backed counterpart to [GetSubagentMessages].
//
// Subagent transcripts may live at "subagents/agent-<id>" or nested under
// "subagents/workflows/<runId>/agent-<id>". When store implements
// [SessionStoreSubkeys], ListSubkeys is used to find the exact subpath;
// otherwise the direct "subagents/agent-<id>" path is tried.
//
// Synthetic agent_metadata entries injected by the mirror write path (they
// describe the .meta.json sidecar rather than transcript lines) are
// dropped before the chain builder runs.
//
// Returns (nil, nil) when sessionID is not a valid UUID, agentID is empty,
// or the subagent has no entries. Errors come from [SessionStore.Load] or
// [SessionStoreSubkeys.ListSubkeys].
func GetSubagentMessagesFromStore(ctx context.Context, store SessionStore, sessionID, agentID string, opts GetSubagentMessagesOptions) ([]SessionMessage, error) {
	if store == nil {
		return nil, fmt.Errorf("session store is nil")
	}
	if !isValidUUID(sessionID) {
		return nil, nil
	}
	if agentID == "" {
		return nil, nil
	}
	dir := opts.Directory
	if dir == "" {
		dir = "."
	}
	projectKey := sanitizePath(canonicalizePath(dir))

	subpath := "subagents/agent-" + agentID
	if sub, ok := store.(SessionStoreSubkeys); ok {
		subkeys, err := sub.ListSubkeys(ctx, SessionListSubkeysKey{ProjectKey: projectKey, SessionID: sessionID})
		if err != nil {
			return nil, err
		}
		target := "agent-" + agentID
		var match string
		for _, sk := range subkeys {
			if !strings.HasPrefix(sk, "subagents/") {
				continue
			}
			last := sk
			if idx := strings.LastIndex(sk, "/"); idx >= 0 {
				last = sk[idx+1:]
			}
			if last == target {
				match = sk
				break
			}
		}
		if match == "" {
			return nil, nil
		}
		subpath = match
	}

	entries, err := store.Load(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID, Subpath: subpath})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	// Drop synthetic agent_metadata entries — they describe the
	// .meta.json sidecar, not transcript lines.
	transcript := make([]SessionStoreEntry, 0, len(entries))
	for _, e := range entries {
		if e == nil {
			continue
		}
		if t, _ := e["type"].(string); t == "agent_metadata" {
			continue
		}
		transcript = append(transcript, e)
	}
	if len(transcript) == 0 {
		return nil, nil
	}

	filtered := filterTranscriptEntries(transcript)
	return entriesToSessionMessages(filtered, opts.IncludeSystemMessages, opts.Limit, opts.Offset), nil
}

// ---------------------------------------------------------------------------
// SessionStore-backed mutation helpers
// ---------------------------------------------------------------------------

// StoreMutationOptions configures the project scope for store-backed
// session mutators ([RenameSessionViaStore], [TagSessionViaStore],
// [DeleteSessionViaStore], [ForkSessionViaStore]).
type StoreMutationOptions struct {
	// Directory selects the project the session lives under. The
	// project_key is computed via [ProjectKeyForDirectory]. When empty,
	// the current working directory is used.
	Directory string
}

// projectKey returns the resolved project_key for these options.
func (o StoreMutationOptions) projectKey() string {
	return ProjectKeyForDirectory(o.Directory)
}

// RenameSessionViaStore renames a session by appending a custom-title entry
// to the target [SessionStore]. Store-backed counterpart to [RenameSession].
//
// The entry shape is identical to the filesystem mutator so a disk-backed
// session and a store-backed session are interchangeable. Because the entry
// is append-only, repeated calls are safe; readers pick up the most recent
// custom-title from the tail.
//
// title is stripped of leading/trailing whitespace and must be non-empty
// after stripping. sessionID must be a valid UUID.
//
// opts selects the project scope; pass StoreMutationOptions{} to default
// to the current working directory.
func RenameSessionViaStore(ctx context.Context, store SessionStore, sessionID, title string, opts StoreMutationOptions) error {
	if store == nil {
		return fmt.Errorf("session store is nil")
	}
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		return fmt.Errorf("title cannot be empty or whitespace-only")
	}
	projectKey := opts.projectKey()
	entry := SessionStoreEntry{
		"type":        "custom-title",
		"customTitle": trimmedTitle,
		"sessionId":   sessionID,
	}
	return store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID}, []SessionStoreEntry{entry})
}

// TagSessionViaStore tags a session by appending a tag entry to the target
// [SessionStore]. Store-backed counterpart to [TagSession].
//
// Pass a nil tag to clear the tag (appends an empty-string tag entry which
// the list helpers treat as cleared). When non-nil, the tag is Unicode
// sanitized via the same helper used by [TagSession] and must remain
// non-empty after sanitization and whitespace trimming.
//
// The entry shape mirrors [TagSession] exactly so disk and store paths are
// interchangeable. sessionID must be a valid UUID.
//
// opts selects the project scope; pass StoreMutationOptions{} to default
// to the current working directory.
func TagSessionViaStore(ctx context.Context, store SessionStore, sessionID string, tag *string, opts StoreMutationOptions) error {
	if store == nil {
		return fmt.Errorf("session store is nil")
	}
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}
	sanitized := ""
	if tag != nil {
		sanitized = sanitizeTag(*tag)
		if sanitized == "" {
			return fmt.Errorf("tag cannot be empty after sanitization (use nil to clear)")
		}
	}
	projectKey := opts.projectKey()
	entry := SessionStoreEntry{
		"type":      "tag",
		"tag":       sanitized,
		"sessionId": sessionID,
	}
	return store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID}, []SessionStoreEntry{entry})
}

// DeleteSessionViaStore deletes a session from a [SessionStore]. Store-backed
// counterpart to [DeleteSession].
//
// Requires store to implement [SessionStoreDeleter]; returns an error
// otherwise. When store also implements [SessionStoreSubkeys], every listed
// subkey (subagent transcript, etc.) is deleted in addition to the main
// transcript. Adapters whose [SessionStoreDeleter.Delete] already cascades
// (e.g. [InMemorySessionStore]) need not also implement
// [SessionStoreSubkeys] for cascade to work — the main delete alone will
// wipe them.
//
// If the main delete succeeds but one or more subkey deletes fail, the
// returned error aggregates the subkey failures; the session is left in
// whatever partial state the adapter produced. Callers that require
// transactional semantics should implement them in the adapter.
//
// sessionID must be a valid UUID. opts selects the project scope; pass
// StoreMutationOptions{} to default to the current working directory.
func DeleteSessionViaStore(ctx context.Context, store SessionStore, sessionID string, opts StoreMutationOptions) error {
	if store == nil {
		return fmt.Errorf("session store is nil")
	}
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}
	deleter, ok := store.(SessionStoreDeleter)
	if !ok {
		return fmt.Errorf("session store does not implement SessionStoreDeleter -- cannot delete sessions")
	}
	projectKey := opts.projectKey()

	// Discover subkeys up front (before main delete) so adapters that wipe
	// subkey indexes during the main delete still expose the list.
	var subpaths []string
	if sub, ok := store.(SessionStoreSubkeys); ok {
		discovered, err := sub.ListSubkeys(ctx, SessionListSubkeysKey{ProjectKey: projectKey, SessionID: sessionID})
		if err != nil {
			return fmt.Errorf("failed to list subkeys for cascade delete: %w", err)
		}
		subpaths = discovered
	}

	if err := deleter.Delete(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID}); err != nil {
		return fmt.Errorf("failed to delete session %s: %w", sessionID, err)
	}

	// Best-effort subkey cascade — collect individual failures and return
	// a combined error at the end so a single bad subpath does not mask
	// the others (and does not roll back the main delete).
	var subErrs []string
	for _, subpath := range subpaths {
		if subpath == "" {
			continue
		}
		if err := deleter.Delete(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID, Subpath: subpath}); err != nil {
			subErrs = append(subErrs, fmt.Sprintf("%s: %v", subpath, err))
		}
	}
	if len(subErrs) > 0 {
		return fmt.Errorf("session %s main-transcript deleted but %d subkey delete(s) failed: %s",
			sessionID, len(subErrs), strings.Join(subErrs, "; "))
	}
	return nil
}

// ForkSessionViaStore creates a copy of a session's transcript under a new
// session ID in a [SessionStore]. Store-backed counterpart to [ForkSession].
//
// Every entry from the source session is loaded, deep-copied, and its
// "sessionId" field is rewritten to newSessionID. The rewritten entries are
// then appended under a fresh [SessionKey] that shares the source's
// project_key. Source entries are not mutated.
//
// Both sessionID and newSessionID must be valid UUIDs. Returns an error if
// the source session has no entries. opts selects the project scope for
// both sessions; pass StoreMutationOptions{} to default to the current
// working directory.
//
// Only the main transcript is copied — subagent and other sub-streams
// under the source [SessionKey] are not cloned. Callers that need those
// cloned should iterate via [SessionStoreSubkeys] and copy each subkey
// explicitly.
func ForkSessionViaStore(ctx context.Context, store SessionStore, sessionID, newSessionID string, opts StoreMutationOptions) error {
	if store == nil {
		return fmt.Errorf("session store is nil")
	}
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}
	if !isValidUUID(newSessionID) {
		return fmt.Errorf("invalid new session ID: %s", newSessionID)
	}
	projectKey := opts.projectKey()

	srcEntries, err := store.Load(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID})
	if err != nil {
		return fmt.Errorf("failed to load source session %s: %w", sessionID, err)
	}
	if len(srcEntries) == 0 {
		return fmt.Errorf("source session %s has no entries to fork", sessionID)
	}

	forked, err := deepCopyEntriesRewritingSessionID(srcEntries, newSessionID)
	if err != nil {
		return fmt.Errorf("failed to copy source entries: %w", err)
	}

	return store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: newSessionID}, forked)
}

// deepCopyEntriesRewritingSessionID returns a deep copy of entries with
// every "sessionId" field rewritten to newSessionID. Uses a JSON round-trip
// to ensure no map or slice aliasing between source and destination,
// matching the spec's "don't mutate source entries" requirement for both
// top-level and nested values.
func deepCopyEntriesRewritingSessionID(entries []SessionStoreEntry, newSessionID string) ([]SessionStoreEntry, error) {
	out := make([]SessionStoreEntry, 0, len(entries))
	for _, e := range entries {
		if e == nil {
			continue
		}
		data, err := json.Marshal(e)
		if err != nil {
			return nil, fmt.Errorf("marshal entry: %w", err)
		}
		var clone SessionStoreEntry
		if err := json.Unmarshal(data, &clone); err != nil {
			return nil, fmt.Errorf("unmarshal entry: %w", err)
		}
		// Only rewrite when the source entry had a sessionId at all —
		// avoids synthesizing the field onto metadata entries that did
		// not carry it (e.g. type=summary without sessionId).
		if _, ok := clone["sessionId"]; ok {
			clone["sessionId"] = newSessionID
		}
		out = append(out, clone)
	}
	return out, nil
}
