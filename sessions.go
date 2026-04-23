package claude

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
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
	chain := buildConversationChain(entries)

	var visible []transcriptEntry
	for _, e := range chain {
		if isVisibleMessage(e) {
			visible = append(visible, e)
			continue
		}
		if opts.IncludeSystemMessages && isVisibleSystemMessage(e) {
			visible = append(visible, e)
		}
	}

	messages := make([]SessionMessage, len(visible))
	for i, e := range visible {
		messages[i] = toSessionMessage(e)
	}

	// Apply offset and limit
	if opts.Offset > 0 {
		if opts.Offset >= len(messages) {
			return nil, nil
		}
		messages = messages[opts.Offset:]
	}
	if opts.Limit != nil && *opts.Limit > 0 && *opts.Limit < len(messages) {
		messages = messages[:*opts.Limit]
	}

	return messages, nil
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
