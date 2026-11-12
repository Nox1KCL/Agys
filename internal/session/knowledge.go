package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Nox1KCL/Agys/internal/utils"
)

var kiReferenceKeys = []string{
	"references",
	"sourceConversations",
	"source_conversations",
	"conversationIds",
	"conversation_ids",
	"sourceSessions",
	"source_sessions",
	"sessions",
	"sources",
}

// LoadKnowledgeItems scans knowledgeDir and returns Knowledge Items indexed by source session UUID.
func LoadKnowledgeItems(knowledgeDir string) (map[string][]KnowledgeItem, error) {
	bySession := make(map[string][]KnowledgeItem)

	entries, err := os.ReadDir(knowledgeDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return bySession, nil
		}
		return nil, fmt.Errorf("read knowledge directory %q: %w", knowledgeDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		kiDir := filepath.Join(knowledgeDir, entry.Name())
		item, ok, err := parseKnowledgeItem(entry.Name(), kiDir)
		if err != nil || !ok {
			continue
		}

		for _, sessionID := range item.SourceSessionIDs {
			bySession[sessionID] = append(bySession[sessionID], item)
		}
	}

	return bySession, nil
}

func parseKnowledgeItem(dirName, kiDir string) (KnowledgeItem, bool, error) {
	metaPath := filepath.Join(kiDir, "metadata.json")
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return KnowledgeItem{}, false, nil
		}
		return KnowledgeItem{}, false, fmt.Errorf("read %q: %w", metaPath, err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return KnowledgeItem{}, false, fmt.Errorf("decode %q: %w", metaPath, err)
	}

	title := firstNonEmptyString(payload, "title", "name")
	if title == "" {
		title = dirName
	}

	item := KnowledgeItem{
		ID:               dirName,
		Title:            title,
		Summary:          firstNonEmptyString(payload, "summary", "description"),
		Path:             kiDir,
		UpdatedAt:        firstValidTime(payload, "updatedAt", "updated_at", "createdAt", "created_at"),
		SourceSessionIDs: extractSessionUUIDs(payload),
	}

	return item, true, nil
}

func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if val, ok := m[k].(string); ok {
			if trimmed := strings.TrimSpace(val); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func firstValidTime(m map[string]any, keys ...string) time.Time {
	for _, k := range keys {
		raw, ok := m[k].(string)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
			if t, err := time.Parse(layout, raw); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func extractSessionUUIDs(payload map[string]any) []string {
	unique := make(map[string]struct{})
	for _, key := range kiReferenceKeys {
		if val, ok := payload[key]; ok {
			collectUUIDs(val, unique)
		}
	}

	if len(unique) == 0 {
		return nil
	}

	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func collectUUIDs(val any, out map[string]struct{}) {
	switch v := val.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if utils.IsUUIDv4(trimmed) {
			out[strings.ToLower(trimmed)] = struct{}{}
		}
	case []any:
		for _, elem := range v {
			collectUUIDs(elem, out)
		}
	case map[string]any:
		for _, elem := range v {
			collectUUIDs(elem, out)
		}
	}
}
