package session

import (
	"testing"
	"time"
)

func TestSearchRankingAndSubsessions(t *testing.T) {
	now := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)

	sessions := map[string]Session{
		"11111111-1111-4111-8111-111111111111": {
			ID:          "11111111-1111-4111-8111-111111111111",
			Title:       "Linux Daemon With Unix Domain Sockets",
			WorkDir:     "/home/nox/IT-projects/Synapsed",
			FirstPrompt: "Explain how to build a daemon on Linux",
			LastPrompt:  "Add socket framing support",
			UpdatedAt:   now,
		},
		"22222222-2222-4222-8222-222222222222": {
			ID:          "22222222-2222-4222-8222-222222222222",
			Title:       "Project Technical Analysis And Stability",
			WorkDir:     "/home/nox/Competitions/Debris/app",
			FirstPrompt: "Analyze the entire project architecture",
			LastPrompt:  "Generate fix_plan checklist",
			UpdatedAt:   now.Add(-24 * time.Hour),
			SubSessions: []Session{
				{
					ID:           "33333333-3333-4333-8333-333333333333",
					ParentID:     "22222222-2222-4222-8222-222222222222",
					Title:        "WifiDirect and misc fixes",
					SubagentRole: "WifiDirect and misc fixes",
					WorkDir:      "/home/nox/Competitions/Debris/app",
					FirstPrompt:  "Apply fixes to WifiDirectBeacon",
					UpdatedAt:    now.Add(-25 * time.Hour),
				},
			},
			KnowledgeItems: []KnowledgeItem{
				{Title: "Mesh Recovery Notes", Summary: "BLE and WifiDirect reconnection patterns"},
			},
		},
		"33333333-3333-4333-8333-333333333333": {
			ID:       "33333333-3333-4333-8333-333333333333",
			ParentID: "22222222-2222-4222-8222-222222222222",
			Title:    "WifiDirect and misc fixes",
		},
	}

	t.Run("empty query returns all roots", func(t *testing.T) {
		res := Search(sessions, "")
		if len(res) != 2 {
			t.Fatalf("got %d results, want 2 roots", len(res))
		}
	})

	t.Run("multi-token across dir and last prompt", func(t *testing.T) {
		res := Search(sessions, "synapsed framing")
		if len(res) != 1 || res[0].Session.ID != "11111111-1111-4111-8111-111111111111" {
			t.Fatalf("unexpected search results: %+v", res)
		}
	})

	t.Run("fuzzy title match", func(t *testing.T) {
		res := Search(sessions, "lduds")
		if len(res) != 1 || res[0].Session.ID != "11111111-1111-4111-8111-111111111111" {
			t.Fatalf("expected fuzzy match on title, got %+v", res)
		}
	})

	t.Run("child subsession match surfaces parent and marks child", func(t *testing.T) {
		res := Search(sessions, "wifidirectbeacon")
		if len(res) != 1 || res[0].Session.ID != "22222222-2222-4222-8222-222222222222" {
			t.Fatalf("expected parent surfaced from child prompt, got %+v", res)
		}
		if !res[0].MatchedSubIDs["33333333-3333-4333-8333-333333333333"] {
			t.Fatalf("expected child subsession marked as matched: %+v", res[0].MatchedSubIDs)
		}
	})

	t.Run("knowledge item match", func(t *testing.T) {
		res := Search(sessions, "reconnection")
		if len(res) != 1 || res[0].Session.ID != "22222222-2222-4222-8222-222222222222" {
			t.Fatalf("expected KI summary match, got %+v", res)
		}
	})

	t.Run("prompt does not fuzzy false positive", func(t *testing.T) {
		res := Search(sessions, "qxz")
		if len(res) != 0 {
			t.Fatalf("expected 0 results for non-matching query, got %+v", res)
		}
	})
}
