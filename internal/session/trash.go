package session

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
	"github.com/Nox1KCL/Agys/internal/utils"
)

// TrashedSession represents a session entry stored in the user's FreeDesktop Trash.
type TrashedSession struct {
	TrashName    string
	ID           string
	Title        string
	OriginalPath string
	DeletedAt    time.Time
}

type trashEntryRecord struct {
	infoPath     string
	filePath     string
	trashName    string
	originalPath string
	deletedAt    time.Time
}

// ListTrashedSessions discovers Antigravity sessions belonging to cfg.BrainDir inside FreeDesktop Trash.
func ListTrashedSessions(cfg config.Config) ([]TrashedSession, error) {
	if strings.TrimSpace(cfg.BrainDir) == "" {
		return nil, nil
	}
	trashDir, err := ResolveTrashDir()
	if err != nil {
		return nil, err
	}

	records, err := scanTrashRecords(trashDir)
	if err != nil {
		return nil, err
	}

	cleanBrain := filepath.Clean(cfg.BrainDir)
	userState, _ := LoadUserState(cfg.StateFile)

	var out []TrashedSession
	for _, rec := range records {
		cleanOrig := filepath.Clean(rec.originalPath)
		if filepath.Dir(cleanOrig) != cleanBrain {
			continue
		}
		id := strings.ToLower(strings.TrimSpace(filepath.Base(cleanOrig)))
		if !utils.IsUUIDv4(id) {
			continue
		}
		info, statErr := os.Stat(rec.filePath)
		if statErr != nil || !info.IsDir() {
			continue
		}

		title := resolveTrashedTitle(rec.filePath, id, userState)
		out = append(out, TrashedSession{
			TrashName:    rec.trashName,
			ID:           id,
			Title:        title,
			OriginalPath: cleanOrig,
			DeletedAt:    rec.deletedAt,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if !out[i].DeletedAt.Equal(out[j].DeletedAt) {
			return out[i].DeletedAt.After(out[j].DeletedAt)
		}
		return out[i].TrashName < out[j].TrashName
	})

	return out, nil
}

// RestoreTrashedSession moves a trashed session (matched by TrashName or UUID) back into cfg.BrainDir.
func RestoreTrashedSession(cfg config.Config, target string) (TrashedSession, error) {
	item, trashDir, records, err := findTrashedSession(cfg, target)
	if err != nil {
		return TrashedSession{}, err
	}

	cleanBrain := filepath.Clean(cfg.BrainDir)
	destDir := filepath.Join(cleanBrain, item.ID)
	if _, statErr := os.Lstat(destDir); statErr == nil || !errors.Is(statErr, os.ErrNotExist) {
		return TrashedSession{}, fmt.Errorf("destination session %q already exists in brain directory", item.ID)
	}
	if err := os.MkdirAll(cleanBrain, 0o755); err != nil {
		return TrashedSession{}, fmt.Errorf("create brain directory %q: %w", cleanBrain, err)
	}

	srcDir := filepath.Join(trashDir, "files", item.TrashName)
	infoPath := filepath.Join(trashDir, "info", item.TrashName+".trashinfo")
	if err := restorePath(srcDir, destDir, cleanBrain); err != nil {
		return TrashedSession{}, fmt.Errorf("restore session %s: %w", item.ID, err)
	}
	_ = os.Remove(infoPath)

	// Restore companion annotation file from Trash when present.
	if strings.TrimSpace(cfg.AnnotationsDir) != "" {
		expectedAnn := filepath.Join(filepath.Clean(cfg.AnnotationsDir), item.ID+".pbtxt")
		if _, statErr := os.Lstat(expectedAnn); errors.Is(statErr, os.ErrNotExist) {
			if annRec, ok := findLatestMatchingTrashRecord(records, expectedAnn); ok {
				if err := os.MkdirAll(filepath.Dir(expectedAnn), 0o755); err == nil {
					if restoreErr := restorePath(annRec.filePath, expectedAnn, filepath.Dir(expectedAnn)); restoreErr == nil {
						_ = os.Remove(annRec.infoPath)
					}
				}
			}
		}
	}

	return item, nil
}

// PurgeTrashedSession permanently deletes a trashed session (matched by TrashName or UUID)
// along with any matching trashed annotation and cleans user state/cache references.
func PurgeTrashedSession(cfg config.Config, target string) (TrashedSession, error) {
	item, trashDir, records, err := findTrashedSession(cfg, target)
	if err != nil {
		return TrashedSession{}, err
	}

	srcDir := filepath.Join(trashDir, "files", item.TrashName)
	infoPath := filepath.Join(trashDir, "info", item.TrashName+".trashinfo")

	if err := os.RemoveAll(srcDir); err != nil {
		return TrashedSession{}, fmt.Errorf("purge session payload %s: %w", item.ID, err)
	}
	_ = os.Remove(infoPath)

	if strings.TrimSpace(cfg.AnnotationsDir) != "" {
		expectedAnn := filepath.Join(filepath.Clean(cfg.AnnotationsDir), item.ID+".pbtxt")
		for _, rec := range records {
			if filepath.Clean(rec.originalPath) == expectedAnn {
				_ = os.RemoveAll(rec.filePath)
				_ = os.Remove(rec.infoPath)
			}
		}
	}

	if st, err := LoadUserState(cfg.StateFile); err == nil && st.Delete(item.ID) {
		_ = SaveUserState(cfg.StateFile, st)
	}
	_ = removeSessionsFromCache(cfg.CacheFile, cfg.AntigravityDir, []string{item.ID})

	return item, nil
}

func findTrashedSession(cfg config.Config, target string) (TrashedSession, string, []trashEntryRecord, error) {
	cleanTarget := strings.TrimSpace(target)
	if cleanTarget == "" {
		return TrashedSession{}, "", nil, fmt.Errorf("missing trash session target")
	}

	trashDir, err := ResolveTrashDir()
	if err != nil {
		return TrashedSession{}, "", nil, err
	}
	records, err := scanTrashRecords(trashDir)
	if err != nil {
		return TrashedSession{}, "", nil, err
	}

	items, err := ListTrashedSessions(cfg)
	if err != nil {
		return TrashedSession{}, "", nil, err
	}

	for _, item := range items {
		if item.TrashName == cleanTarget || strings.EqualFold(item.ID, cleanTarget) {
			return item, trashDir, records, nil
		}
	}
	return TrashedSession{}, "", nil, fmt.Errorf("trashed session %q not found", target)
}

func findLatestMatchingTrashRecord(records []trashEntryRecord, origPath string) (trashEntryRecord, bool) {
	cleanWant := filepath.Clean(origPath)
	var best trashEntryRecord
	found := false
	for _, rec := range records {
		if filepath.Clean(rec.originalPath) != cleanWant {
			continue
		}
		if _, err := os.Lstat(rec.filePath); err != nil {
			continue
		}
		if !found || rec.deletedAt.After(best.deletedAt) {
			best = rec
			found = true
		}
	}
	return best, found
}

func scanTrashRecords(trashDir string) ([]trashEntryRecord, error) {
	infoDir := filepath.Join(trashDir, "info")
	filesDir := filepath.Join(trashDir, "files")
	entries, err := os.ReadDir(infoDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read trash info directory %q: %w", infoDir, err)
	}

	records := make([]trashEntryRecord, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".trashinfo") {
			continue
		}
		infoPath := filepath.Join(infoDir, name)
		raw, readErr := os.ReadFile(infoPath)
		if readErr != nil {
			continue
		}
		origPath, deletedAt := parseTrashInfo(string(raw))
		if origPath == "" {
			continue
		}
		trashName := strings.TrimSuffix(name, ".trashinfo")
		records = append(records, trashEntryRecord{
			infoPath:     infoPath,
			filePath:     filepath.Join(filesDir, trashName),
			trashName:    trashName,
			originalPath: origPath,
			deletedAt:    deletedAt,
		})
	}
	return records, nil
}

func parseTrashInfo(content string) (origPath string, deletedAt time.Time) {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Path="):
			raw := strings.TrimPrefix(trimmed, "Path=")
			if unescaped, err := url.PathUnescape(raw); err == nil {
				origPath = unescaped
			} else {
				origPath = raw
			}
		case strings.HasPrefix(trimmed, "DeletionDate="):
			raw := strings.TrimPrefix(trimmed, "DeletionDate=")
			if t, err := time.ParseInLocation("2006-01-02T15:04:05", raw, time.Local); err == nil {
				deletedAt = t
			} else if t, err := time.Parse(time.RFC3339, raw); err == nil {
				deletedAt = t
			}
		}
	}
	return origPath, deletedAt
}

func resolveTrashedTitle(sessionDir, id string, userState UserState) string {
	if ov, ok := userState.Sessions[id]; ok && ov.Title != nil {
		if clean := NormalizeTitle(*ov.Title); clean != "" {
			return clean
		}
	}
	if mdTitle := resolveMarkdownTitle(sessionDir); mdTitle != "" {
		return mdTitle
	}
	td := extractTranscriptDetails(sessionDir)
	if clean := NormalizeTitle(td.FirstPromptTitle); clean != "" {
		return clean
	}
	if clean := NormalizeTitle(td.FirstPrompt); clean != "" {
		return utils.TruncateRunes(clean, 80)
	}
	short := id
	if len(short) > 8 {
		short = short[:8]
	}
	return "Session " + short
}

func restorePath(srcPath, dstPath, stagingParent string) error {
	err := os.Rename(srcPath, dstPath)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	return copyAndRemoveCrossDevice(srcPath, dstPath, stagingParent)
}
