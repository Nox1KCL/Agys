package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
	"github.com/Nox1KCL/Agys/internal/utils"
)

const untitledSessionLabel = "(untitled)"

// Collect scans Antigravity data directories, indexes prompts/workdirs, links
// Knowledge Items and subagent child sessions, and returns all sessions by UUID.
func Collect(cfg config.Config) (map[string]Session, error) {
	kiBySession, err := LoadKnowledgeItems(cfg.KnowledgeDir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(cfg.BrainDir)
	if err != nil {
		return nil, fmt.Errorf("read brain directory %q: %w", cfg.BrainDir, err)
	}

	sessions := make(map[string]Session)
	detailsByID := make(map[string]transcriptDetails)
	var subagentLinks []subagentLink

	for _, entry := range entries {
		if !entry.IsDir() || !utils.IsUUIDv4(entry.Name()) {
			continue
		}

		id := strings.ToLower(entry.Name())
		sessionDir := filepath.Join(cfg.BrainDir, entry.Name())

		var dirModTime time.Time
		if info, infoErr := entry.Info(); infoErr == nil {
			dirModTime = info.ModTime()
		}

		td := extractTranscriptDetails(sessionDir)
		detailsByID[id] = td

		s := Session{
			ID:             id,
			Path:           sessionDir,
			WorkDir:        td.WorkDir,
			FirstPrompt:    td.FirstPrompt,
			LastPrompt:     td.LastPrompt,
			UpdatedAt:      latestFileMtime(sessionDir, dirModTime),
			KnowledgeItems: kiBySession[id],
		}

		if cfg.AnnotationsDir != "" {
			annPath := filepath.Join(cfg.AnnotationsDir, id+".pbtxt")
			if meta, ok, annErr := loadAnnotation(annPath); annErr == nil && ok {
				s.Pinned = meta.Pinned
				s.Archived = meta.Archived
				s.LastViewedAt = meta.UpdatedAt
				if meta.Title != "" {
					s.Title = meta.Title
					s.TitleSource = TitleSourceAnnotation
				}
			}
		}

		subagentLinks = append(subagentLinks, loadSubagentDescriptors(sessionDir, id)...)
		sessions[id] = s
	}

	attachSubagentMetadata(sessions, detailsByID, subagentLinks)

	for id, s := range sessions {
		resolveTitleFallback(&s, detailsByID[id])
		s.DefaultTitle = s.Title
		s.DefaultTitleSource = s.TitleSource
		sessions[id] = s
	}

	if cfg.StateFile != "" {
		st, err := LoadUserState(cfg.StateFile)
		if err != nil {
			return nil, err
		}
		ApplyUserState(sessions, st)
	}

	bindSubSessions(sessions)
	return sessions, nil
}

// CollectTitles scans Antigravity data directories and returns a uuid -> title dictionary.
func CollectTitles(cfg config.Config) (map[string]string, error) {
	sessions, err := Collect(cfg)
	if err != nil {
		return nil, err
	}
	return TitlesMap(sessions), nil
}

// Sorted returns all sessions ordered by freshness (max inner file mtime descending, then ID).
func Sorted(sessions map[string]Session) []Session {
	ordered := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		ordered = append(ordered, s)
	}
	sortSessionSlice(ordered)
	return ordered
}

// SortedRoots returns top-level sessions (excluding child subagent sessions attached
// to a parent) ordered by freshness descending.
func SortedRoots(sessions map[string]Session) []Session {
	ordered := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		if s.ParentID != "" {
			if _, hasParent := sessions[s.ParentID]; hasParent {
				continue
			}
		}
		ordered = append(ordered, s)
	}
	sortSessionSlice(ordered)
	return ordered
}

func attachSubagentMetadata(sessions map[string]Session, details map[string]transcriptDetails, links []subagentLink) {
	for _, link := range links {
		child, ok := sessions[link.ChildID]
		if !ok || link.ChildID == link.ParentID {
			continue
		}
		child.ParentID = link.ParentID
		if link.Role != "" {
			child.SubagentRole = link.Role
		}
		if link.TypeName != "" {
			child.SubagentType = link.TypeName
		}
		if len(link.WorkspaceURIs) > 0 {
			child.WorkspaceURIs = link.WorkspaceURIs
			if wsDir := primaryWorkspaceDir(link.WorkspaceURIs); wsDir != "" {
				child.WorkDir = wsDir
			}
		}
		sessions[link.ChildID] = child
	}

	for id, s := range sessions {
		if s.ParentID != "" || s.TitleSource == TitleSourceAnnotation {
			continue
		}
		if senderID := details[id].SenderParentID; senderID != "" && senderID != id {
			if _, hasParent := sessions[senderID]; hasParent {
				s.ParentID = senderID
				sessions[id] = s
			}
		}
	}
}

func bindSubSessions(sessions map[string]Session) {
	childrenByParent := make(map[string][]Session)
	for _, s := range sessions {
		if s.ParentID == "" {
			continue
		}
		if _, hasParent := sessions[s.ParentID]; !hasParent {
			continue
		}
		childrenByParent[s.ParentID] = append(childrenByParent[s.ParentID], s)
	}

	for parentID, children := range childrenByParent {
		sortSessionSlice(children)
		parent := sessions[parentID]
		parent.SubSessions = children
		if parent.WorkDir == "" {
			for _, child := range children {
				if child.WorkDir != "" {
					parent.WorkDir = child.WorkDir
					break
				}
			}
		}
		sessions[parentID] = parent
	}
}

func sortSessionSlice(items []Session) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Pinned != items[j].Pinned {
			return items[i].Pinned
		}
		if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		return items[i].ID < items[j].ID
	})
}

func resolveTitleFallback(s *Session, td transcriptDetails) {
	if s.Title != "" {
		return
	}

	if s.SubagentRole != "" {
		s.Title = s.SubagentRole
		s.TitleSource = TitleSourceSubagent
		return
	}

	for _, ki := range s.KnowledgeItems {
		if ki.Title != "" {
			s.Title = ki.Title
			s.TitleSource = TitleSourceKnowledge
			if s.UpdatedAt.IsZero() && !ki.UpdatedAt.IsZero() {
				s.UpdatedAt = ki.UpdatedAt
			}
			return
		}
	}

	if title := resolveMarkdownTitle(s.Path); title != "" {
		s.Title = title
		s.TitleSource = TitleSourceMarkdown
		return
	}

	if td.FirstPromptTitle != "" {
		s.Title = td.FirstPromptTitle
		s.TitleSource = TitleSourceTranscript
		if s.UpdatedAt.IsZero() && !td.FirstPromptTime.IsZero() {
			s.UpdatedAt = td.FirstPromptTime
		}
		return
	}

	s.Title = untitledSessionLabel
	s.TitleSource = TitleSourceNone
}
