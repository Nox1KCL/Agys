package session

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
	"github.com/Nox1KCL/Agys/internal/utils"
)

const (
	untitledSessionLabel = "(untitled)"
	maxScanWorkers       = 8
)

type scanJob struct {
	index      int
	id         string
	sessionDir string
	annPath    string
	dirModTime time.Time
}

type scanResult struct {
	id    string
	entry cachedSessionEntry
	hit   bool
}

// Collect scans Antigravity data directories, indexes prompts/workdirs, links
// Knowledge Items and subagent child sessions, and returns all sessions by UUID.
func Collect(cfg config.Config) (map[string]Session, error) {
	prevCache, hasCache := loadIndexCache(cfg.CacheFile, cfg.AntigravityDir)

	knowledgeFP, knowledgeExists := computeKnowledgeFingerprint(cfg.KnowledgeDir)
	var kiBySession map[string][]KnowledgeItem
	cacheDirty := !hasCache

	if hasCache && prevCache.KnowledgeExists == knowledgeExists && prevCache.KnowledgeFP == knowledgeFP {
		kiBySession = prevCache.KnowledgeBySession
	} else {
		var err error
		kiBySession, err = LoadKnowledgeItems(cfg.KnowledgeDir)
		if err != nil {
			return nil, err
		}
		cacheDirty = true
	}

	dirEntries, err := os.ReadDir(cfg.BrainDir)
	if err != nil {
		return nil, fmt.Errorf("read brain directory %q: %w", cfg.BrainDir, err)
	}

	jobs := make([]scanJob, 0, len(dirEntries))
	for _, entry := range dirEntries {
		if !entry.IsDir() || !utils.IsUUIDv4(entry.Name()) {
			continue
		}

		id := strings.ToLower(entry.Name())
		sessionDir := filepath.Join(cfg.BrainDir, entry.Name())

		var dirModTime time.Time
		if info, infoErr := entry.Info(); infoErr == nil {
			dirModTime = info.ModTime()
		}

		annPath := ""
		if cfg.AnnotationsDir != "" {
			annPath = filepath.Join(cfg.AnnotationsDir, id+".pbtxt")
		}

		jobs = append(jobs, scanJob{
			index:      len(jobs),
			id:         id,
			sessionDir: sessionDir,
			annPath:    annPath,
			dirModTime: dirModTime,
		})
	}

	results := scanSessionJobs(jobs, prevCache, hasCache)

	sessions := make(map[string]Session, len(results))
	detailsByID := make(map[string]transcriptDetails, len(results))
	mdTitles := make(map[string]string, len(results))
	var subagentLinks []subagentLink
	nextCacheSessions := make(map[string]cachedSessionEntry, len(results))

	for i, res := range results {
		if !res.hit {
			cacheDirty = true
		}

		rec := res.entry
		nextCacheSessions[res.id] = rec
		detailsByID[res.id] = rec.Details
		mdTitles[res.id] = rec.MarkdownTitle

		sessionDir := jobs[i].sessionDir

		var kis []KnowledgeItem
		if items := kiBySession[res.id]; len(items) > 0 {
			kis = make([]KnowledgeItem, len(items))
			copy(kis, items)
		}

		s := Session{
			ID:             res.id,
			Path:           sessionDir,
			WorkDir:        rec.Details.WorkDir,
			FirstPrompt:    rec.Details.FirstPrompt,
			LastPrompt:     rec.Details.LastPrompt,
			UpdatedAt:      rec.UpdatedAt,
			KnowledgeItems: kis,
		}

		if rec.HasAnnotation {
			s.Pinned = rec.AnnotationPinned
			s.Archived = rec.AnnotationArch
			s.LastViewedAt = rec.AnnotationViewed
			if rec.AnnotationTitle != "" {
				s.Title = rec.AnnotationTitle
				s.TitleSource = TitleSourceAnnotation
			}
		}

		if len(rec.SubagentLinks) > 0 {
			subagentLinks = append(subagentLinks, rec.SubagentLinks...)
		}
		sessions[res.id] = s
	}

	if hasCache && len(prevCache.Sessions) != len(nextCacheSessions) {
		cacheDirty = true
	}

	if cacheDirty && cfg.CacheFile != "" {
		_ = saveIndexCache(cfg.CacheFile, cachedIndex{
			Version:            cacheSchemaVersion,
			DataRoot:           cfg.AntigravityDir,
			KnowledgeFP:        knowledgeFP,
			KnowledgeExists:    knowledgeExists,
			KnowledgeBySession: kiBySession,
			Sessions:           nextCacheSessions,
		})
	}

	attachSubagentMetadata(sessions, detailsByID, subagentLinks)

	for id, s := range sessions {
		resolveTitleFallback(&s, detailsByID[id], mdTitles[id])
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

func scanSessionJobs(jobs []scanJob, prevCache cachedIndex, hasCache bool) []scanResult {
	results := make([]scanResult, len(jobs))
	if len(jobs) == 0 {
		return results
	}

	workerCount := runtime.GOMAXPROCS(0)
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > maxScanWorkers {
		workerCount = maxScanWorkers
	}
	if workerCount > len(jobs) {
		workerCount = len(jobs)
	}

	jobCh := make(chan scanJob, len(jobs))
	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)

	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for j := range jobCh {
				fp := computeSessionFingerprint(j.sessionDir, j.annPath)
				if hasCache {
					if cached, ok := prevCache.Sessions[j.id]; ok && cached.Fingerprint == fp {
						results[j.index] = scanResult{
							id:    j.id,
							entry: cached,
							hit:   true,
						}
						continue
					}
				}

				entry := buildSessionCacheEntry(j, fp)
				results[j.index] = scanResult{
					id:    j.id,
					entry: entry,
					hit:   false,
				}
			}
		}()
	}
	wg.Wait()

	return results
}

func buildSessionCacheEntry(j scanJob, fp uint64) cachedSessionEntry {
	td := extractTranscriptDetails(j.sessionDir)
	entry := cachedSessionEntry{
		Fingerprint:   fp,
		UpdatedAt:     latestFileMtime(j.sessionDir, j.dirModTime),
		Details:       td,
		MarkdownTitle: resolveMarkdownTitle(j.sessionDir),
		SubagentLinks: loadSubagentDescriptors(j.sessionDir, j.id),
	}

	if j.annPath != "" {
		if meta, ok, annErr := loadAnnotation(j.annPath); annErr == nil && ok {
			entry.HasAnnotation = true
			entry.AnnotationTitle = meta.Title
			entry.AnnotationPinned = meta.Pinned
			entry.AnnotationArch = meta.Archived
			entry.AnnotationViewed = meta.UpdatedAt
		}
	}

	return entry
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

func resolveTitleFallback(s *Session, td transcriptDetails, mdTitle string) {
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

	if mdTitle != "" {
		s.Title = mdTitle
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
