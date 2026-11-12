package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nox1KCL/Agys/internal/utils"
)

type subagentLink struct {
	ParentID      string
	ChildID       string
	Role          string
	TypeName      string
	WorkspaceURIs []string
}

type subagentDescriptorFile struct {
	ConversationID     string   `json:"conversationId"`
	WorkspaceURIs      []string `json:"workspaceUris"`
	SubagentDescriptor struct {
		TypeName string `json:"typeName"`
		Role     string `json:"role"`
	} `json:"subagentDescriptor"`
}

func loadSubagentDescriptors(parentDir, parentID string) []subagentLink {
	subagentsDir := filepath.Join(parentDir, ".system_generated", "subagents")
	entries, err := os.ReadDir(subagentsDir)
	if err != nil {
		return nil
	}

	var links []subagentLink
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(subagentsDir, entry.Name()))
		if err != nil {
			continue
		}

		var doc subagentDescriptorFile
		if err := json.Unmarshal(raw, &doc); err != nil {
			continue
		}

		childID := strings.TrimSpace(doc.ConversationID)
		if childID == "" {
			childID = strings.TrimSuffix(entry.Name(), ".json")
		}
		if !utils.IsUUIDv4(childID) {
			continue
		}

		links = append(links, subagentLink{
			ParentID:      parentID,
			ChildID:       strings.ToLower(childID),
			Role:          strings.TrimSpace(doc.SubagentDescriptor.Role),
			TypeName:      strings.TrimSpace(doc.SubagentDescriptor.TypeName),
			WorkspaceURIs: doc.WorkspaceURIs,
		})
	}

	return links
}

func parseSenderFromContent(content string) string {
	const prefix = "sender="
	idx := strings.Index(content, prefix)
	if idx < 0 {
		return ""
	}

	rest := content[idx+len(prefix):]
	if len(rest) < 36 {
		return ""
	}

	candidate := rest[:36]
	if !utils.IsUUIDv4(candidate) {
		return ""
	}
	return strings.ToLower(candidate)
}

func primaryWorkspaceDir(uris []string) string {
	var fallback string
	for _, uri := range uris {
		cleaned := strings.TrimSpace(strings.TrimPrefix(uri, "file://"))
		if cleaned == "" {
			continue
		}
		cleaned = filepath.Clean(cleaned)
		if fallback == "" {
			fallback = cleaned
		}
		if !strings.Contains(strings.ToLower(cleaned), "obsidian") {
			return cleaned
		}
	}
	return fallback
}
