package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Nox1KCL/Agys/internal/utils"
)

const (
	maxMarkdownScanLines   = 30
	maxTranscriptTitleRune = 72
	maxStoredPromptRunes   = 2000
	maxPlannerScanForDir   = 12
)

var preferredMarkdownFiles = []string{
	"task.md",
	"implementation_plan.md",
	"walkthrough.md",
}

var projectMarkers = []string{
	".git",
	"go.mod",
	"package.json",
	"Cargo.toml",
	"pyproject.toml",
}

type transcriptEntry struct {
	Type      string `json:"type"`
	CreatedAt string `json:"created_at"`
	Content   string `json:"content"`
	ToolCalls []struct {
		Args map[string]any `json:"args"`
	} `json:"tool_calls"`
}

type transcriptDetails struct {
	FirstPrompt      string
	LastPrompt       string
	FirstPromptTitle string
	FirstPromptTime  time.Time
	SenderParentID   string
	WorkDir          string
}

func resolveMarkdownTitle(sessionDir string) string {
	seen := make(map[string]struct{}, len(preferredMarkdownFiles))
	for _, name := range preferredMarkdownFiles {
		seen[name] = struct{}{}
		if title := extractMarkdownHeading(filepath.Join(sessionDir, name)); title != "" {
			return title
		}
	}

	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, checked := seen[name]; checked || !strings.HasSuffix(name, ".md") {
			continue
		}
		if title := extractMarkdownHeading(filepath.Join(sessionDir, name)); title != "" {
			return title
		}
	}

	return ""
}

func extractMarkdownHeading(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for i := 0; i < maxMarkdownScanLines; i++ {
		line, err := reader.ReadString('\n')
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			heading := strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			if heading != "" {
				return heading
			}
		}
		if err != nil {
			break
		}
	}

	return ""
}

func extractTranscriptDetails(sessionDir string) transcriptDetails {
	path := filepath.Join(sessionDir, ".system_generated", "logs", "transcript.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return transcriptDetails{}
	}
	defer f.Close()

	var details transcriptDetails
	var dirCandidate string
	plannerChecked := 0
	lineIdx := 0

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			hasUser := strings.Contains(trimmed, `"USER_INPUT"`)
			hasSys := lineIdx < 5 && strings.Contains(trimmed, `"SYSTEM_MESSAGE"`)
			needPlanner := details.WorkDir == "" && plannerChecked < maxPlannerScanForDir && strings.Contains(trimmed, `"PLANNER_RESPONSE"`)

			if hasUser || hasSys || needPlanner {
				var entry transcriptEntry
				if json.Unmarshal([]byte(trimmed), &entry) == nil {
					switch entry.Type {
					case "USER_INPUT":
						prompt := extractUserPromptBody(entry.Content)
						if prompt != "" {
							truncatedPrompt := utils.TruncateRunes(prompt, maxStoredPromptRunes)
							if details.FirstPrompt == "" {
								details.FirstPrompt = truncatedPrompt
								details.FirstPromptTitle = firstPromptLine(prompt, maxTranscriptTitleRune)
								details.FirstPromptTime = parseTranscriptTime(entry.CreatedAt)
							}
							details.LastPrompt = truncatedPrompt
						}
						if details.WorkDir == "" && dirCandidate == "" {
							if p := extractActiveDocPath(entry.Content); p != "" {
								dirCandidate = resolveProjectRoot(p)
							}
						}
					case "SYSTEM_MESSAGE":
						if details.SenderParentID == "" {
							details.SenderParentID = parseSenderFromContent(entry.Content)
						}
					case "PLANNER_RESPONSE":
						if len(entry.ToolCalls) > 0 {
							plannerChecked++
							if cwd, filePath := extractToolCallPaths(entry); cwd != "" {
								details.WorkDir = cwd
							} else if dirCandidate == "" && filePath != "" {
								dirCandidate = resolveProjectRoot(filePath)
							}
						}
					}
				}
			}
			lineIdx++
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			break
		}
	}

	if details.WorkDir == "" && dirCandidate != "" {
		details.WorkDir = dirCandidate
	}
	return details
}

func extractUserPromptBody(content string) string {
	const openTag = "<USER_REQUEST>"
	const closeTag = "</USER_REQUEST>"

	if start := strings.Index(content, openTag); start >= 0 {
		rest := content[start+len(openTag):]
		if end := strings.Index(rest, closeTag); end >= 0 {
			return normalizePromptText(rest[:end])
		}
		return normalizePromptText(rest)
	}

	body := content
	for _, marker := range []string{"<ADDITIONAL_METADATA>", "<USER_SETTINGS_CHANGE>"} {
		if idx := strings.Index(body, marker); idx >= 0 {
			body = body[:idx]
		}
	}
	return normalizePromptText(body)
}

func normalizePromptText(raw string) string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, " ")
}

func firstPromptLine(prompt string, maxRunes int) string {
	return utils.TruncateRunes(strings.TrimSpace(prompt), maxRunes)
}

func extractActiveDocPath(content string) string {
	for _, prefix := range []string{"Active Document:", "- /"} {
		idx := strings.Index(content, prefix)
		if idx < 0 {
			continue
		}
		rest := content[idx:]
		if prefix == "Active Document:" {
			rest = strings.TrimSpace(strings.TrimPrefix(rest, prefix))
		} else {
			rest = strings.TrimPrefix(rest, "- ")
		}
		if newline := strings.IndexByte(rest, '\n'); newline >= 0 {
			rest = rest[:newline]
		}
		if paren := strings.Index(rest, " ("); paren >= 0 {
			rest = rest[:paren]
		}
		candidate := strings.TrimSpace(rest)
		if isUsableWorkPath(candidate) {
			return candidate
		}
	}
	return ""
}

func extractToolCallPaths(entry transcriptEntry) (cwd string, filePath string) {
	for _, tc := range entry.ToolCalls {
		if rawCwd := unquoteArg(argString(tc.Args["Cwd"])); isUsableWorkPath(rawCwd) {
			return filepath.Clean(rawCwd), ""
		}
		for _, key := range []string{"Path", "AbsolutePath", "TargetFile", "DirectoryPath", "SearchPath"} {
			if p := unquoteArg(argString(tc.Args[key])); isUsableWorkPath(p) && filePath == "" {
				filePath = filepath.Clean(p)
			}
		}
	}
	return "", filePath
}

func argString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func unquoteArg(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) {
		if unquoted, err := strconv.Unquote(trimmed); err == nil {
			trimmed = unquoted
		} else {
			trimmed = strings.Trim(trimmed, `"`)
		}
	}
	return strings.TrimSpace(trimmed)
}

func isUsableWorkPath(p string) bool {
	if p == "" || !filepath.IsAbs(p) {
		return false
	}
	clean := filepath.Clean(p)
	if clean == "/" || strings.HasPrefix(clean, "/tmp/") || clean == "/tmp" {
		return false
	}
	geminiPart := string(filepath.Separator) + ".gemini"
	if strings.Contains(clean, geminiPart+string(filepath.Separator)) || strings.HasSuffix(clean, geminiPart) {
		return false
	}
	return true
}

func resolveProjectRoot(filePath string) string {
	clean := filepath.Clean(filePath)
	home, _ := os.UserHomeDir()

	dir := clean
	if info, err := os.Stat(clean); err != nil || !info.IsDir() {
		dir = filepath.Dir(clean)
	}

	cur := dir
	for depth := 0; depth < 6; depth++ {
		if cur == "/" || cur == "." || (home != "" && cur == home) {
			break
		}
		for _, marker := range projectMarkers {
			if _, err := os.Stat(filepath.Join(cur, marker)); err == nil {
				return cur
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	return dir
}

func parseTranscriptTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

// latestFileMtime walks sessionDir and returns the maximum mtime across all inner files.
// On POSIX filesystems, modifying file contents does not update parent directory mtime,
// so directory ModTime is only used as a fallback when no inner files exist.
func latestFileMtime(sessionDir string, fallback time.Time) time.Time {
	var maxTime time.Time
	foundFile := false

	_ = filepath.WalkDir(sessionDir, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		modTime := info.ModTime()
		if !foundFile || modTime.After(maxTime) {
			maxTime = modTime
			foundFile = true
		}
		return nil
	})

	if foundFile {
		return maxTime
	}
	return fallback
}
