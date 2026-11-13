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

func TestSearchIndexMatchesSearchWithOptions(t *testing.T) {
	sessions := map[string]Session{
		"11111111-1111-4111-8111-111111111111": {
			ID:          "11111111-1111-4111-8111-111111111111",
			Title:       "Deploy API Gateway",
			WorkDir:     "/home/nox/work/gateway",
			FirstPrompt: "Plan canary deployment rollout",
			LastPrompt:  "Verify rollback thresholds",
			Pinned:      true,
			SubSessions: []Session{
				{
					ID:           "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
					Title:        "Token Inspector",
					SubagentRole: "Security Auditor",
					ParentID:     "11111111-1111-4111-8111-111111111111",
				},
			},
			KnowledgeItems: []KnowledgeItem{
				{
					Title:   "Circuit Breaker",
					Summary: "Exponential backoff prevents upstream overload",
				},
			},
		},
		"22222222-2222-4222-8222-222222222222": {
			ID:          "22222222-2222-4222-8222-222222222222",
			Title:       "Cache Optimization",
			WorkDir:     "/home/nox/work/agys",
			FirstPrompt: "Benchmark gob snapshot loading",
			LastPrompt:  "Tune worker concurrency",
		},
		"33333333-3333-4333-8333-333333333333": {
			ID:          "33333333-3333-4333-8333-333333333333",
			Title:       "Legacy Migration",
			WorkDir:     "/home/nox/archive/legacy",
			FirstPrompt: "Archive old deployment scripts",
			Archived:    true,
			SubSessions: []Session{
				{
					ID:           "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
					Title:        "Legacy Verifier",
					SubagentRole: "QA Helper",
					ParentID:     "33333333-3333-4333-8333-333333333333",
				},
			},
		},
	}

	idx := NewSearchIndex(sessions)

	cases := []struct {
		name  string
		query string
		opts  SearchOptions
	}{
		{name: "empty active", query: "", opts: SearchOptions{IncludeArchived: false}},
		{name: "empty with archived", query: "", opts: SearchOptions{IncludeArchived: true}},
		{name: "multi token match", query: "deploy canary", opts: SearchOptions{IncludeArchived: false}},
		{name: "fuzzy title match", query: "cchopt", opts: SearchOptions{IncludeArchived: false}},
		{name: "subsession match", query: "auditor", opts: SearchOptions{IncludeArchived: false}},
		{name: "knowledge match", query: "upstream", opts: SearchOptions{IncludeArchived: false}},
		{name: "archived hidden", query: "legacy", opts: SearchOptions{IncludeArchived: false}},
		{name: "archived shown", query: "legacy", opts: SearchOptions{IncludeArchived: true}},
		{name: "archived subsession hidden", query: "verifier", opts: SearchOptions{IncludeArchived: false}},
		{name: "archived subsession shown", query: "verifier", opts: SearchOptions{IncludeArchived: true}},
		{name: "no match", query: "zzzzqxz", opts: SearchOptions{IncludeArchived: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := idx.Search(tc.query, tc.opts)
			want := SearchWithOptions(sessions, tc.query, tc.opts)
			if len(got) != len(want) {
				t.Fatalf("len mismatch for %q (%+v): got %d, want %d", tc.query, tc.opts, len(got), len(want))
			}
			for i := range want {
				if got[i].Session.ID != want[i].Session.ID {
					t.Fatalf("result[%d] ID mismatch: got %s, want %s", i, got[i].Session.ID, want[i].Session.ID)
				}
				if got[i].Score != want[i].Score {
					t.Fatalf("result[%d] score mismatch: got %d, want %d", i, got[i].Score, want[i].Score)
				}
				if len(got[i].MatchedSubIDs) != len(want[i].MatchedSubIDs) {
					t.Fatalf("result[%d] MatchedSubIDs len mismatch: got %v, want %v", i, got[i].MatchedSubIDs, want[i].MatchedSubIDs)
				}
				for subID, matched := range want[i].MatchedSubIDs {
					if got[i].MatchedSubIDs[subID] != matched {
						t.Fatalf("result[%d] MatchedSubIDs[%s] mismatch: got %v, want %v", i, subID, got[i].MatchedSubIDs[subID], matched)
					}
				}
			}
		})
	}
}
