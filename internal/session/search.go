package session

import (
	"sort"
	"strings"

	"github.com/Nox1KCL/Agys/internal/utils"
)

// SearchResult represents a root session matching a search query along with
// ranking score and any matched child subsession IDs to expand in the tree.
type SearchResult struct {
	Session       Session
	Score         int
	MatchedSubIDs map[string]bool
}

// SearchOptions controls filtering behavior during session search.
type SearchOptions struct {
	IncludeArchived bool
}

// Search ranks non-archived root sessions against query using fzf-style multi-token matching.
// An empty query returns all non-archived root sessions ordered by pinned state and freshness.
func Search(sessions map[string]Session, query string) []SearchResult {
	return SearchWithOptions(sessions, query, SearchOptions{})
}

// SearchWithOptions ranks root sessions against query according to opts.
func SearchWithOptions(sessions map[string]Session, query string, opts SearchOptions) []SearchResult {
	allRoots := SortedRoots(sessions)
	roots := make([]Session, 0, len(allRoots))
	for _, r := range allRoots {
		if !opts.IncludeArchived && r.Archived {
			continue
		}
		roots = append(roots, r)
	}

	tokens := tokenizeQuery(query)
	if len(tokens) == 0 {
		results := make([]SearchResult, len(roots))
		for i, r := range roots {
			results[i] = SearchResult{Session: r}
		}
		return results
	}

	results := make([]SearchResult, 0, len(roots))
	for _, root := range roots {
		if res, ok := matchRootSession(root, tokens); ok {
			results = append(results, res)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Session.Pinned != results[j].Session.Pinned {
			return results[i].Session.Pinned
		}
		if !results[i].Session.UpdatedAt.Equal(results[j].Session.UpdatedAt) {
			return results[i].Session.UpdatedAt.After(results[j].Session.UpdatedAt)
		}
		return results[i].Session.ID < results[j].Session.ID
	})

	return results
}

func tokenizeQuery(query string) []string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	return fields
}

func matchRootSession(root Session, tokens []string) (SearchResult, bool) {
	totalScore := 0
	var matchedSubs map[string]bool

	for _, tok := range tokens {
		bestTokenScore := scoreSessionStructured(root, tok, 100)

		for _, child := range root.SubSessions {
			childScore := scoreSessionStructured(child, tok, 70)
			if childScore > 0 {
				if matchedSubs == nil {
					matchedSubs = make(map[string]bool)
				}
				matchedSubs[child.ID] = true
				if childScore > bestTokenScore {
					bestTokenScore = childScore
				}
			}
		}

		if bestTokenScore == 0 {
			return SearchResult{}, false
		}
		totalScore += bestTokenScore
	}

	return SearchResult{
		Session:       root,
		Score:         totalScore,
		MatchedSubIDs: matchedSubs,
	}, true
}

func scoreSessionStructured(s Session, token string, multiplierPct int) int {
	best := 0
	check := func(candidate string, weight int, allowFuzzy bool) {
		if candidate == "" {
			return
		}
		raw := scoreText(strings.ToLower(candidate), token, allowFuzzy)
		if raw == 0 {
			return
		}
		weighted := (raw * weight * multiplierPct) / 10000
		if weighted > best {
			best = weighted
		}
	}

	check(s.Title, 120, true)
	check(s.SubagentRole, 110, true)
	check(s.SubagentType, 60, true)
	check(s.WorkDir, 95, true)
	if short := utils.ShortenHome(s.WorkDir); short != s.WorkDir {
		check(short, 95, true)
	}
	for _, uri := range s.WorkspaceURIs {
		check(uri, 90, true)
	}
	for _, ki := range s.KnowledgeItems {
		check(ki.Title, 80, true)
		check(ki.Summary, 60, false)
	}
	check(s.FirstPrompt, 55, false)
	check(s.LastPrompt, 55, false)
	check(s.ID, 45, false)

	return best
}

func scoreText(textLower, token string, allowFuzzy bool) int {
	if idx := strings.Index(textLower, token); idx >= 0 {
		score := 60
		if idx == 0 {
			score += 25
		} else if isBoundaryByte(textLower[idx-1]) {
			score += 18
		}
		return score
	}

	if !allowFuzzy {
		return 0
	}
	return fuzzySubsequenceScore(textLower, token)
}

func fuzzySubsequenceScore(textLower, token string) int {
	tRunes := []rune(textLower)
	qRunes := []rune(token)
	if len(qRunes) == 0 || len(qRunes) > len(tRunes) {
		return 0
	}

	score := 0
	qi := 0
	prevMatch := -2

	for ti, r := range tRunes {
		if r != qRunes[qi] {
			continue
		}

		score += 10
		if ti == 0 || isBoundaryRune(tRunes[ti-1]) {
			score += 8
		}
		if ti == prevMatch+1 {
			score += 7
		}
		prevMatch = ti
		qi++
		if qi == len(qRunes) {
			return score
		}
	}

	return 0
}

func isBoundaryByte(b byte) bool {
	return b == ' ' || b == '/' || b == '-' || b == '_' || b == '.'
}

func isBoundaryRune(r rune) bool {
	return r == ' ' || r == '/' || r == '-' || r == '_' || r == '.'
}
