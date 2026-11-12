package session

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxArtifactPreviewLines = 120
	maxArtifactPreviewBytes = 12 * 1024
)

// SessionArtifacts holds bounded markdown excerpts from a session's brain directory for preview.
type SessionArtifacts struct {
	TaskMarkdown string
	TaskDone     int
	TaskTotal    int
	PlanMarkdown string
	Walkthrough  string
}

// LoadArtifacts reads task.md, implementation_plan.md, and walkthrough.md for preview display.
func LoadArtifacts(sessionDir string) SessionArtifacts {
	if strings.TrimSpace(sessionDir) == "" {
		return SessionArtifacts{}
	}

	taskText, done, total := readMarkdownBounded(filepath.Join(sessionDir, "task.md"), true)
	planText, _, _ := readMarkdownBounded(filepath.Join(sessionDir, "implementation_plan.md"), false)
	walkText, _, _ := readMarkdownBounded(filepath.Join(sessionDir, "walkthrough.md"), false)

	return SessionArtifacts{
		TaskMarkdown: taskText,
		TaskDone:     done,
		TaskTotal:    total,
		PlanMarkdown: planText,
		Walkthrough:  walkText,
	}
}

func readMarkdownBounded(path string, countTasks bool) (excerpt string, done int, total int) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, 0
	}
	defer f.Close()

	var lines []string
	totalBytes := 0
	truncated := false

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 256*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if countTasks {
			trimmed := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(trimmed, "- [x] "), strings.HasPrefix(trimmed, "- [X] "):
				done++
				total++
			case strings.HasPrefix(trimmed, "- [ ] "), strings.HasPrefix(trimmed, "- [/] "):
				total++
			}
		}

		if !truncated {
			if len(lines) >= maxArtifactPreviewLines || totalBytes+len(line) > maxArtifactPreviewBytes {
				truncated = true
				if !countTasks {
					break
				}
			} else {
				lines = append(lines, line)
				totalBytes += len(line) + 1
			}
		}
	}

	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		return "", done, total
	}
	if truncated {
		text += "\n... (truncated)"
	}
	return text, done, total
}
