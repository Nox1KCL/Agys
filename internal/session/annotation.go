package session

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type annotationMeta struct {
	Title     string
	UpdatedAt time.Time
	Pinned    bool
	Archived  bool
}

func loadAnnotation(path string) (annotationMeta, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return annotationMeta{}, false, nil
		}
		return annotationMeta{}, false, fmt.Errorf("read annotation %q: %w", path, err)
	}

	content := string(raw)
	meta := annotationMeta{
		UpdatedAt: extractProtoTimestamp(content, "last_user_view_time"),
		Pinned:    extractProtoBool(content, "pinned"),
		Archived:  extractProtoBool(content, "archived"),
	}

	if title, ok := extractProtoString(content, "title"); ok {
		meta.Title = strings.TrimSpace(title)
	}

	return meta, true, nil
}

func extractProtoString(content, field string) (string, bool) {
	idx := findFieldStart(content, field+":")
	if idx < 0 {
		return "", false
	}

	rest := strings.TrimLeft(content[idx+len(field)+1:], " \t")
	if len(rest) == 0 || rest[0] != '"' {
		return "", false
	}

	escaped := false
	for i := 1; i < len(rest); i++ {
		if escaped {
			escaped = false
			continue
		}
		if rest[i] == '\\' {
			escaped = true
			continue
		}
		if rest[i] == '"' {
			unquoted, err := strconv.Unquote(rest[:i+1])
			if err != nil {
				return rest[1:i], true
			}
			return unquoted, true
		}
	}

	return "", false
}

func extractProtoBool(content, field string) bool {
	idx := findFieldStart(content, field+":")
	if idx < 0 {
		return false
	}
	rest := strings.TrimLeft(content[idx+len(field)+1:], " \t")
	return strings.HasPrefix(rest, "true")
}

func extractProtoTimestamp(content, field string) time.Time {
	idx := findFieldStart(content, field+":")
	if idx < 0 {
		return time.Time{}
	}

	rest := content[idx+len(field)+1:]
	open := strings.IndexByte(rest, '{')
	if open < 0 {
		return time.Time{}
	}
	close := strings.IndexByte(rest[open:], '}')
	if close < 0 {
		return time.Time{}
	}

	block := rest[open+1 : open+close]
	seconds, ok := extractProtoInt64(block, "seconds")
	if !ok || seconds <= 0 {
		return time.Time{}
	}
	nanos, _ := extractProtoInt64(block, "nanos")
	return time.Unix(seconds, nanos)
}

func extractProtoInt64(content, field string) (int64, bool) {
	idx := findFieldStart(content, field+":")
	if idx < 0 {
		return 0, false
	}

	rest := strings.TrimLeft(content[idx+len(field)+1:], " \t")
	end := 0
	for end < len(rest) && (rest[end] >= '0' && rest[end] <= '9') {
		end++
	}
	if end == 0 {
		return 0, false
	}

	val, err := strconv.ParseInt(rest[:end], 10, 64)
	if err != nil {
		return 0, false
	}
	return val, true
}

func findFieldStart(content, token string) int {
	offset := 0
	for {
		idx := strings.Index(content[offset:], token)
		if idx < 0 {
			return -1
		}
		abs := offset + idx
		if abs == 0 {
			return abs
		}
		prev := content[abs-1]
		if prev == ' ' || prev == '\t' || prev == '\n' || prev == '\r' || prev == '{' {
			return abs
		}
		offset = abs + len(token)
	}
}
