// Package session discovers, links, searches, and resumes Google Antigravity CLI sessions and Knowledge Items.
package session

import "time"

// TitleSource describes where a session title was resolved from.
type TitleSource string

const (
	TitleSourceAnnotation TitleSource = "annotation"
	TitleSourceSubagent   TitleSource = "subagent"
	TitleSourceKnowledge  TitleSource = "knowledge"
	TitleSourceMarkdown   TitleSource = "markdown"
	TitleSourceTranscript TitleSource = "transcript"
	TitleSourceNone       TitleSource = "none"
)

// KnowledgeItem represents an entry stored under ~/.gemini/antigravity-cli/knowledge/.
type KnowledgeItem struct {
	ID               string
	Title            string
	Summary          string
	Path             string
	UpdatedAt        time.Time
	SourceSessionIDs []string
}

// Session represents a single Antigravity CLI conversation stored under brain/.
type Session struct {
	ID             string
	Title          string
	TitleSource    TitleSource
	Path           string
	WorkDir        string
	FirstPrompt    string
	LastPrompt     string
	UpdatedAt      time.Time
	LastViewedAt   time.Time
	Pinned         bool
	Archived       bool
	ParentID       string
	SubagentRole   string
	SubagentType   string
	WorkspaceURIs  []string
	SubSessions    []Session
	KnowledgeItems []KnowledgeItem
}

// TitlesMap converts a session dictionary into a plain uuid -> title map.
func TitlesMap(sessions map[string]Session) map[string]string {
	titles := make(map[string]string, len(sessions))
	for id, s := range sessions {
		titles[id] = s.Title
	}
	return titles
}
