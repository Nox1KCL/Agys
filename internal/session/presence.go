package session

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/Nox1KCL/Agys/internal/config"
	"github.com/Nox1KCL/Agys/internal/utils"
)

// PresenceState describes live runtime status for an active Antigravity session.
type PresenceState struct {
	Active  bool
	Working bool
}

// DetectActiveSessions inspects Antigravity presence lockfiles to identify
// currently running sessions and whether any of them appear to be actively executing.
func DetectActiveSessions(cfg config.Config) map[string]PresenceState {
	result := make(map[string]PresenceState)
	if strings.TrimSpace(cfg.PresenceDir) == "" {
		return result
	}

	entries, err := os.ReadDir(cfg.PresenceDir)
	if err != nil {
		return result
	}

	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".lock") {
			continue
		}
		id := strings.ToLower(strings.TrimSuffix(name, ".lock"))
		if !utils.IsUUIDv4(id) {
			continue
		}
		lockPath := filepath.Join(cfg.PresenceDir, name)
		if isLockHeldByActiveProcess(lockPath) {
			ids = append(ids, id)
		}
	}

	sort.Strings(ids)
	for _, id := range ids {
		result[id] = PresenceState{
			Active:  true,
			Working: isSessionTranscriptWorking(filepath.Join(cfg.BrainDir, id)),
		}
	}
	return result
}

// isLockHeldByActiveProcess returns true only when another open file description
// currently holds an exclusive flock on lockPath.
func isLockHeldByActiveProcess(lockPath string) bool {
	f, err := os.Open(lockPath)
	if err != nil {
		return false
	}
	defer f.Close()

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	if err == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return false
	}
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

func isSessionTranscriptWorking(sessionDir string) bool {
	if strings.TrimSpace(sessionDir) == "" {
		return false
	}
	tPath := filepath.Join(sessionDir, ".system_generated", "logs", "transcript.jsonl")
	f, err := os.Open(tPath)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return false
	}

	const tailBytes = 4096
	offset := int64(0)
	if info.Size() > tailBytes {
		offset = info.Size() - tailBytes
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return false
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return false
	}

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 0 {
		return false
	}
	lastLine := strings.TrimSpace(lines[len(lines)-1])
	if lastLine == "" && len(lines) > 1 {
		lastLine = strings.TrimSpace(lines[len(lines)-2])
	}
	if lastLine == "" {
		return false
	}

	var step struct {
		Type      string            `json:"type"`
		ToolCalls []json.RawMessage `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(lastLine), &step); err != nil {
		return false
	}
	if step.Type == "USER_INPUT" || step.Type == "GENERIC" {
		return true
	}
	return step.Type == "PLANNER_RESPONSE" && len(step.ToolCalls) > 0
}
