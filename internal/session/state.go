package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StateOverride stores explicit pin/archive overrides persisted by agys.
type StateOverride struct {
	Pinned   *bool `json:"pinned,omitempty"`
	Archived *bool `json:"archived,omitempty"`
}

// UserState holds user-managed pin and archive overrides keyed by session UUID.
type UserState struct {
	Sessions map[string]StateOverride `json:"sessions"`
}

// LoadUserState reads user pin/archive overrides from path.
func LoadUserState(path string) (UserState, error) {
	st := UserState{
		Sessions: make(map[string]StateOverride),
	}
	if strings.TrimSpace(path) == "" {
		return st, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return st, nil
		}
		return st, fmt.Errorf("read state file %q: %w", path, err)
	}

	if strings.TrimSpace(string(raw)) == "" {
		return st, nil
	}

	if err := json.Unmarshal(raw, &st); err != nil {
		return UserState{Sessions: make(map[string]StateOverride)}, fmt.Errorf("parse state file %q: %w", path, err)
	}
	if st.Sessions == nil {
		st.Sessions = make(map[string]StateOverride)
	}
	return st, nil
}

// SaveUserState atomically persists user pin/archive overrides to path.
func SaveUserState(path string, st UserState) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	cleaned := UserState{
		Sessions: make(map[string]StateOverride, len(st.Sessions)),
	}
	for id, ov := range st.Sessions {
		if ov.Pinned == nil && ov.Archived == nil {
			continue
		}
		cleaned.Sessions[strings.ToLower(strings.TrimSpace(id))] = ov
	}

	payload, err := json.MarshalIndent(cleaned, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	payload = append(payload, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state directory %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".agys-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp state file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp state file: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp state file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("commit state file %q: %w", path, err)
	}
	return nil
}

// SetPinned records an explicit pinned override for session id.
func (s *UserState) SetPinned(id string, pinned bool) {
	if s.Sessions == nil {
		s.Sessions = make(map[string]StateOverride)
	}
	key := strings.ToLower(strings.TrimSpace(id))
	ov := s.Sessions[key]
	ov.Pinned = boolPtr(pinned)
	s.Sessions[key] = ov
}

// SetArchived records an explicit archived override for session id.
func (s *UserState) SetArchived(id string, archived bool) {
	if s.Sessions == nil {
		s.Sessions = make(map[string]StateOverride)
	}
	key := strings.ToLower(strings.TrimSpace(id))
	ov := s.Sessions[key]
	ov.Archived = boolPtr(archived)
	s.Sessions[key] = ov
}

// ApplyUserState merges persisted user overrides onto discovered sessions.
func ApplyUserState(sessions map[string]Session, st UserState) {
	for id, ov := range st.Sessions {
		s, ok := sessions[id]
		if !ok {
			continue
		}
		if ov.Pinned != nil {
			s.Pinned = *ov.Pinned
		}
		if ov.Archived != nil {
			s.Archived = *ov.Archived
		}
		sessions[id] = s
	}
}

func boolPtr(v bool) *bool {
	return &v
}
