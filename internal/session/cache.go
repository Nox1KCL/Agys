package session

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"hash"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const cacheSchemaVersion uint32 = 1

type cachedIndex struct {
	Version            uint32
	DataRoot           string
	KnowledgeFP        uint64
	KnowledgeExists    bool
	KnowledgeBySession map[string][]KnowledgeItem
	Sessions           map[string]cachedSessionEntry
}

type cachedSessionEntry struct {
	Fingerprint      uint64
	UpdatedAt        time.Time
	Details          transcriptDetails
	MarkdownTitle    string
	HasAnnotation    bool
	AnnotationTitle  string
	AnnotationPinned bool
	AnnotationArch   bool
	AnnotationViewed time.Time
	SubagentLinks    []subagentLink
}

func loadIndexCache(path, dataRoot string) (cachedIndex, bool) {
	if strings.TrimSpace(path) == "" {
		return cachedIndex{}, false
	}

	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return cachedIndex{}, false
	}

	var idx cachedIndex
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&idx); err != nil {
		return cachedIndex{}, false
	}
	if idx.Version != cacheSchemaVersion || idx.DataRoot != dataRoot {
		return cachedIndex{}, false
	}
	if idx.Sessions == nil {
		idx.Sessions = make(map[string]cachedSessionEntry)
	}
	if idx.KnowledgeBySession == nil {
		idx.KnowledgeBySession = make(map[string][]KnowledgeItem)
	}
	return idx, true
}

func saveIndexCache(path string, idx cachedIndex) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(idx); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".agys-cache-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func removeSessionsFromCache(cachePath, dataRoot string, ids []string) error {
	if strings.TrimSpace(cachePath) == "" || len(ids) == 0 {
		return nil
	}
	idx, ok := loadIndexCache(cachePath, dataRoot)
	if !ok || len(idx.Sessions) == 0 {
		return nil
	}
	changed := false
	for _, id := range ids {
		key := strings.ToLower(strings.TrimSpace(id))
		if key == "" {
			continue
		}
		if _, exists := idx.Sessions[key]; exists {
			delete(idx.Sessions, key)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return saveIndexCache(cachePath, idx)
}

func computeSessionFingerprint(sessionDir, annPath string) uint64 {
	h := fnv.New64a()

	// Top-level directory + direct entries (catches markdown artifacts and root changes).
	hashPathStat(h, sessionDir)
	if entries, err := os.ReadDir(sessionDir); err == nil {
		for _, entry := range entries {
			if entry.Name() == ".system_generated" {
				continue
			}
			hashDirEntryStat(h, entry)
		}
	}

	// Key internal Antigravity markers without descending into large steps/ or messages/ trees.
	sysDir := filepath.Join(sessionDir, ".system_generated")
	hashPathStat(h, sysDir)

	logsDir := filepath.Join(sysDir, "logs")
	hashPathStat(h, logsDir)
	hashPathStat(h, filepath.Join(logsDir, "transcript.jsonl"))

	hashPathStat(h, filepath.Join(sysDir, "steps"))
	hashPathStat(h, filepath.Join(sysDir, "messages"))

	subagentsDir := filepath.Join(sysDir, "subagents")
	hashPathStat(h, subagentsDir)
	if entries, err := os.ReadDir(subagentsDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				hashDirEntryStat(h, entry)
			}
		}
	}

	if annPath != "" {
		hashStringField(h, "ann")
		hashPathStat(h, annPath)
	}

	return h.Sum64()
}

func computeKnowledgeFingerprint(knowledgeDir string) (uint64, bool) {
	if strings.TrimSpace(knowledgeDir) == "" {
		return 0, false
	}

	dirInfo, err := os.Stat(knowledgeDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 1, false
		}
		return 0, false
	}

	h := fnv.New64a()
	hashFileInfo(h, ".", dirInfo)

	entries, err := os.ReadDir(knowledgeDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			hashDirEntryStat(h, entry)
			hashPathStat(h, filepath.Join(knowledgeDir, entry.Name(), "metadata.json"))
		}
	}

	return h.Sum64(), true
}

func hashPathStat(h hash.Hash64, path string) {
	info, err := os.Stat(path)
	if err != nil {
		hashUint8(h, 0)
		return
	}
	hashUint8(h, 1)
	hashFileInfo(h, filepath.Base(path), info)
}

func hashDirEntryStat(h hash.Hash64, entry fs.DirEntry) {
	info, err := entry.Info()
	if err != nil {
		hashUint8(h, 0)
		hashStringField(h, entry.Name())
		return
	}
	hashUint8(h, 1)
	hashFileInfo(h, entry.Name(), info)
}

func hashFileInfo(h hash.Hash64, name string, info fs.FileInfo) {
	hashStringField(h, name)
	if info.IsDir() {
		hashUint8(h, 1)
	} else {
		hashUint8(h, 0)
	}
	hashInt64(h, info.ModTime().UnixNano())
	hashInt64(h, info.Size())
}

func hashStringField(h hash.Hash64, s string) {
	hashInt64(h, int64(len(s)))
	_, _ = h.Write([]byte(s))
}

func hashUint8(h hash.Hash64, v uint8) {
	_, _ = h.Write([]byte{v})
}

func hashInt64(h hash.Hash64, v int64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(v))
	_, _ = h.Write(buf[:])
}
