package session

import (
	"errors"
	"fmt"
	"io"
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

// DeleteMode selects whether session files are moved to Trash or permanently removed.
type DeleteMode int

const (
	// DeleteModeTrash moves session files to the user's FreeDesktop Trash directory.
	DeleteModeTrash DeleteMode = iota
	// DeleteModePermanent removes session files permanently from disk.
	DeleteModePermanent
)

// DeletePlan describes the selected session and any descendant subsessions to remove together.
type DeletePlan struct {
	Target   Session
	Children []Session
}

// AllIDs returns the target session ID followed by all descendant subsession IDs.
func (p DeletePlan) AllIDs() []string {
	ids := make([]string, 0, 1+len(p.Children))
	if p.Target.ID != "" {
		ids = append(ids, strings.ToLower(strings.TrimSpace(p.Target.ID)))
	}
	for _, child := range p.Children {
		if id := strings.ToLower(strings.TrimSpace(child.ID)); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// DeleteResult summarizes the sessions deleted from disk and the Trash location used (when applicable).
type DeleteResult struct {
	DeletedIDs []string
	TrashDir   string
}

type deleteTarget struct {
	id         string
	sessionDir string
	annPath    string
}

// CollectDeletePlan resolves targetID and recursively collects all descendant child subsessions.
func CollectDeletePlan(sessions map[string]Session, targetID string) (DeletePlan, bool) {
	cleanTarget := strings.ToLower(strings.TrimSpace(targetID))
	target, ok := lookupSessionCaseInsensitive(sessions, cleanTarget)
	if !ok {
		return DeletePlan{}, false
	}

	childrenByParent := buildChildAdjacency(sessions)
	visited := map[string]bool{strings.ToLower(target.ID): true}
	queue := append([]string(nil), childrenByParent[strings.ToLower(target.ID)]...)
	var children []Session

	for len(queue) > 0 {
		childID := queue[0]
		queue = queue[1:]
		if visited[childID] {
			continue
		}
		visited[childID] = true

		child, exists := lookupSessionCaseInsensitive(sessions, childID)
		if !exists {
			continue
		}
		children = append(children, child)
		queue = append(queue, childrenByParent[childID]...)
	}

	return DeletePlan{
		Target:   target,
		Children: children,
	}, true
}

func lookupSessionCaseInsensitive(sessions map[string]Session, want string) (Session, bool) {
	if s, ok := sessions[want]; ok {
		return s, true
	}
	for id, s := range sessions {
		if strings.EqualFold(id, want) {
			return s, true
		}
	}
	return Session{}, false
}

func buildChildAdjacency(sessions map[string]Session) map[string][]string {
	adj := make(map[string][]string)
	seenEdge := make(map[string]map[string]bool)

	addEdge := func(parentID, childID string) {
		p := strings.ToLower(strings.TrimSpace(parentID))
		c := strings.ToLower(strings.TrimSpace(childID))
		if p == "" || c == "" || p == c {
			return
		}
		if seenEdge[p] == nil {
			seenEdge[p] = make(map[string]bool)
		}
		if seenEdge[p][c] {
			return
		}
		seenEdge[p][c] = true
		adj[p] = append(adj[p], c)
	}

	// Preserve existing SubSessions display order first.
	parentIDs := make([]string, 0, len(sessions))
	for id := range sessions {
		parentIDs = append(parentIDs, id)
	}
	sort.Strings(parentIDs)

	for _, id := range parentIDs {
		s := sessions[id]
		for _, sub := range s.SubSessions {
			addEdge(id, sub.ID)
		}
	}
	for _, id := range parentIDs {
		s := sessions[id]
		if s.ParentID != "" {
			addEdge(s.ParentID, id)
		}
	}

	return adj
}

// ResolveTrashDir returns the FreeDesktop.org Trash directory path ($XDG_DATA_HOME/Trash or ~/.local/share/Trash).
func ResolveTrashDir() (string, error) {
	if xdgData := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdgData != "" {
		resolved, err := utils.ResolveAbsPath(xdgData)
		if err != nil {
			return "", fmt.Errorf("resolve XDG_DATA_HOME %q: %w", xdgData, err)
		}
		if resolved != "" {
			return filepath.Join(resolved, "Trash"), nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "Trash"), nil
}

// DeleteSession moves (DeleteModeTrash) or permanently deletes (DeleteModePermanent)
// targetID along with its descendant subsessions, annotations, and cached index entries.
func DeleteSession(cfg config.Config, sessions map[string]Session, targetID string, mode DeleteMode) (DeleteResult, error) {
	plan, ok := CollectDeletePlan(sessions, targetID)
	if !ok {
		return DeleteResult{}, fmt.Errorf("session %q not found", targetID)
	}

	// Delete children bottom-up before their parent so partial errors never create orphan roots.
	orderedSessions := make([]Session, 0, 1+len(plan.Children))
	for i := len(plan.Children) - 1; i >= 0; i-- {
		orderedSessions = append(orderedSessions, plan.Children[i])
	}
	orderedSessions = append(orderedSessions, plan.Target)

	targets := make([]deleteTarget, 0, len(orderedSessions))
	for _, s := range orderedSessions {
		id := strings.ToLower(strings.TrimSpace(s.ID))
		if !utils.IsUUIDv4(id) {
			return DeleteResult{}, fmt.Errorf("refusing to delete invalid session UUID %q", s.ID)
		}
		sessionDir, err := resolveSessionDirForDelete(cfg.BrainDir, id, s.Path)
		if err != nil {
			return DeleteResult{}, err
		}

		annPath := ""
		if strings.TrimSpace(cfg.AnnotationsDir) != "" {
			annPath = filepath.Join(filepath.Clean(cfg.AnnotationsDir), id+".pbtxt")
		}
		targets = append(targets, deleteTarget{
			id:         id,
			sessionDir: sessionDir,
			annPath:    annPath,
		})
	}

	var trashDir, filesDir, infoDir string
	if mode == DeleteModeTrash {
		var err error
		trashDir, err = ResolveTrashDir()
		if err != nil {
			return DeleteResult{}, err
		}
		filesDir = filepath.Join(trashDir, "files")
		infoDir = filepath.Join(trashDir, "info")
		if err := os.MkdirAll(filesDir, 0o700); err != nil {
			return DeleteResult{}, fmt.Errorf("create trash files directory %q: %w", filesDir, err)
		}
		if err := os.MkdirAll(infoDir, 0o700); err != nil {
			return DeleteResult{}, fmt.Errorf("create trash info directory %q: %w", infoDir, err)
		}
	}

	now := time.Now()
	deletedIDs := make([]string, 0, len(targets))
	for _, tgt := range targets {
		if tgt.annPath != "" {
			if err := deletePath(tgt.annPath, mode, filesDir, infoDir, now); err != nil {
				return DeleteResult{}, fmt.Errorf("delete annotation for %s: %w", tgt.id, err)
			}
		}
		if err := deletePath(tgt.sessionDir, mode, filesDir, infoDir, now); err != nil {
			return DeleteResult{}, fmt.Errorf("delete session %s: %w", tgt.id, err)
		}
		deletedIDs = append(deletedIDs, tgt.id)
	}

	_ = removeSessionsFromCache(cfg.CacheFile, cfg.AntigravityDir, deletedIDs)

	return DeleteResult{
		DeletedIDs: deletedIDs,
		TrashDir:   trashDir,
	}, nil
}

func resolveSessionDirForDelete(brainDir, id, candidate string) (string, error) {
	if strings.TrimSpace(brainDir) == "" {
		return "", fmt.Errorf("brain directory is not configured")
	}
	cleanBrain := filepath.Clean(brainDir)

	chosen := strings.TrimSpace(candidate)
	if chosen == "" {
		chosen = filepath.Join(cleanBrain, id)
	}
	cleanTarget := filepath.Clean(chosen)

	if filepath.Dir(cleanTarget) != cleanBrain || !strings.EqualFold(filepath.Base(cleanTarget), id) {
		return "", fmt.Errorf("refusing to delete path %q outside brain directory %q", cleanTarget, cleanBrain)
	}
	return cleanTarget, nil
}

func deletePath(path string, mode DeleteMode, filesDir, infoDir string, now time.Time) error {
	if mode == DeleteModePermanent {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		return nil
	}
	return movePathToTrash(path, filesDir, infoDir, now, os.Rename)
}

type renameFunc func(oldpath, newpath string) error

func movePathToTrash(srcPath, filesDir, infoDir string, now time.Time, rename renameFunc) error {
	info, err := os.Lstat(srcPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("resolve absolute path for %q: %w", srcPath, err)
	}

	trashName, infoPath, err := reserveTrashEntry(absSrc, info.IsDir(), filesDir, infoDir, now)
	if err != nil {
		return err
	}

	dstPath := filepath.Join(filesDir, trashName)
	err = rename(absSrc, dstPath)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		_ = os.Remove(infoPath)
		return err
	}

	if copyErr := copyAndRemoveCrossDevice(absSrc, dstPath, filesDir); copyErr != nil {
		_ = os.Remove(infoPath)
		return copyErr
	}
	return nil
}

func reserveTrashEntry(absSrc string, isDir bool, filesDir, infoDir string, now time.Time) (trashName, infoPath string, err error) {
	baseName := filepath.Base(absSrc)
	ext := ""
	stem := baseName
	if !isDir {
		ext = filepath.Ext(baseName)
		if ext != "" && ext != baseName {
			stem = strings.TrimSuffix(baseName, ext)
		} else {
			ext = ""
		}
	}

	metadata := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		encodeTrashPath(absSrc),
		now.Local().Format("2006-01-02T15:04:05"),
	)

	for attempt := 1; attempt <= 10000; attempt++ {
		candidate := baseName
		if attempt > 1 {
			candidate = fmt.Sprintf("%s.%d%s", stem, attempt, ext)
		}

		dstPath := filepath.Join(filesDir, candidate)
		if _, statErr := os.Lstat(dstPath); statErr == nil || !errors.Is(statErr, os.ErrNotExist) {
			continue
		}

		candidateInfo := filepath.Join(infoDir, candidate+".trashinfo")
		f, openErr := os.OpenFile(candidateInfo, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if openErr != nil {
			if errors.Is(openErr, os.ErrExist) {
				continue
			}
			return "", "", openErr
		}

		_, writeErr := f.WriteString(metadata)
		closeErr := f.Close()
		if writeErr != nil {
			_ = os.Remove(candidateInfo)
			return "", "", writeErr
		}
		if closeErr != nil {
			_ = os.Remove(candidateInfo)
			return "", "", closeErr
		}

		return candidate, candidateInfo, nil
	}

	return "", "", fmt.Errorf("could not allocate unique trash filename for %q", absSrc)
}

func encodeTrashPath(absPath string) string {
	slashPath := filepath.ToSlash(absPath)
	parts := strings.Split(slashPath, "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func copyAndRemoveCrossDevice(srcPath, dstPath, stagingDir string) error {
	stageParent, err := os.MkdirTemp(stagingDir, ".agys-stage-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(stageParent)
	}()

	stagePath := filepath.Join(stageParent, "payload")
	if err := copyEntryRecursive(srcPath, stagePath); err != nil {
		return err
	}
	if err := os.Rename(stagePath, dstPath); err != nil {
		return err
	}

	if err := os.RemoveAll(srcPath); err != nil {
		return err
	}
	return nil
}

func copyEntryRecursive(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	}

	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode().Perm()|0o700); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyEntryRecursive(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return os.Chmod(dst, info.Mode().Perm())
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported non-regular file %q", src)
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
