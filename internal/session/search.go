package session

import (
	"os"
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

type queryToken struct {
	text  string
	runes []rune
}

type indexedField struct {
	lower      string
	runes      []rune
	weight     int
	allowFuzzy bool
}

type indexedSession struct {
	session Session
	fields  []indexedField
}

type indexedRoot struct {
	entry    indexedSession
	children []indexedSession
}

// SearchIndex stores pre-sorted root sessions and pre-normalized fields for fast keystroke filtering.
type SearchIndex struct {
	roots []indexedRoot
}

// NewSearchIndex builds a reusable search index from sessions ordered by pinned state and freshness.
func NewSearchIndex(sessions map[string]Session) *SearchIndex {
	allRoots := SortedRoots(sessions)
	home, _ := os.UserHomeDir()

	roots := make([]indexedRoot, len(allRoots))
	for i, root := range allRoots {
		children := make([]indexedSession, len(root.SubSessions))
		for j, child := range root.SubSessions {
			children[j] = buildIndexedSession(child, home)
		}
		roots[i] = indexedRoot{
			entry:    buildIndexedSession(root, home),
			children: children,
		}
	}

	return &SearchIndex{roots: roots}
}

// Search ranks matching root sessions against query without re-sorting or lowercasing candidates.
func (idx *SearchIndex) Search(query string, opts SearchOptions) []SearchResult {
	if idx == nil || len(idx.roots) == 0 {
		return nil
	}

	tokens := tokenizeQuery(query)
	if len(tokens) == 0 {
		results := make([]SearchResult, 0, len(idx.roots))
		for _, r := range idx.roots {
			if !opts.IncludeArchived && r.entry.session.Archived {
				continue
			}
			results = append(results, SearchResult{Session: r.entry.session})
		}
		return results
	}

	results := make([]SearchResult, 0, len(idx.roots))
	for _, r := range idx.roots {
		if !opts.IncludeArchived && r.entry.session.Archived {
			continue
		}
		if res, ok := matchIndexedRoot(r, tokens); ok {
			results = append(results, res)
		}
	}

	sortSearchResults(results)
	return results
}

// Search ranks non-archived root sessions against query using fzf-style multi-token matching.
// An empty query returns all non-archived root sessions ordered by pinned state and freshness.
func Search(sessions map[string]Session, query string) []SearchResult {
	return SearchWithOptions(sessions, query, SearchOptions{})
}

// SearchWithOptions ranks root sessions against query according to opts.
func SearchWithOptions(sessions map[string]Session, query string, opts SearchOptions) []SearchResult {
	tokens := tokenizeQuery(query)
	if len(tokens) == 0 {
		allRoots := SortedRoots(sessions)
		results := make([]SearchResult, 0, len(allRoots))
		for _, r := range allRoots {
			if !opts.IncludeArchived && r.Archived {
				continue
			}
			results = append(results, SearchResult{Session: r})
		}
		return results
	}

	return NewSearchIndex(sessions).Search(query, opts)
}

func buildIndexedSession(s Session, home string) indexedSession {
	fields := make([]indexedField, 0, 8+len(s.WorkspaceURIs)+2*len(s.KnowledgeItems))
	add := func(raw string, weight int, allowFuzzy bool) {
		if raw == "" {
			return
		}
		lower := strings.ToLower(raw)
		var runes []rune
		if allowFuzzy {
			runes = []rune(lower)
		}
		fields = append(fields, indexedField{
			lower:      lower,
			runes:      runes,
			weight:     weight,
			allowFuzzy: allowFuzzy,
		})
	}

	add(s.Title, 120, true)
	add(s.SubagentRole, 110, true)
	add(s.SubagentType, 60, true)
	add(s.WorkDir, 95, true)
	if short := utils.ShortenHomeWith(s.WorkDir, home); short != s.WorkDir {
		add(short, 95, true)
	}
	for _, uri := range s.WorkspaceURIs {
		add(uri, 90, true)
	}
	for _, ki := range s.KnowledgeItems {
		add(ki.Title, 80, true)
		add(ki.Summary, 60, false)
	}
	add(s.FirstPrompt, 55, false)
	add(s.LastPrompt, 55, false)
	add(s.ID, 45, false)

	return indexedSession{
		session: s,
		fields:  fields,
	}
}

func tokenizeQuery(query string) []queryToken {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(parts) == 0 {
		return nil
	}
	tokens := make([]queryToken, len(parts))
	for i, part := range parts {
		tokens[i] = queryToken{
			text:  part,
			runes: []rune(part),
		}
	}
	return tokens
}

func matchIndexedRoot(root indexedRoot, tokens []queryToken) (SearchResult, bool) {
	totalScore := 0
	var matchedSubs map[string]bool

	for _, tok := range tokens {
		bestTokenScore := scoreIndexedSession(root.entry, tok, 100)

		for _, child := range root.children {
			childScore := scoreIndexedSession(child, tok, 70)
			if childScore > 0 {
				if matchedSubs == nil {
					matchedSubs = make(map[string]bool)
				}
				matchedSubs[child.session.ID] = true
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
		Session:       root.entry.session,
		Score:         totalScore,
		MatchedSubIDs: matchedSubs,
	}, true
}

func scoreIndexedSession(s indexedSession, tok queryToken, multiplierPct int) int {
	best := 0
	for _, f := range s.fields {
		raw := scoreField(f, tok)
		if raw == 0 {
			continue
		}
		weighted := (raw * f.weight * multiplierPct) / 10000
		if weighted > best {
			best = weighted
		}
	}
	return best
}

func scoreField(f indexedField, tok queryToken) int {
	if idx := strings.Index(f.lower, tok.text); idx >= 0 {
		score := 60
		if idx == 0 {
			score += 25
		} else if isBoundaryByte(f.lower[idx-1]) {
			score += 18
		}
		return score
	}

	if !f.allowFuzzy {
		return 0
	}
	return fuzzyRunesScore(f.runes, tok.runes)
}

func fuzzyRunesScore(tRunes, qRunes []rune) int {
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

func sortSearchResults(results []SearchResult) {
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
}

func isBoundaryByte(b byte) bool {
	return b == ' ' || b == '/' || b == '-' || b == '_' || b == '.'
}

func isBoundaryRune(r rune) bool {
	return r == ' ' || r == '/' || r == '-' || r == '_' || r == '.'
}
