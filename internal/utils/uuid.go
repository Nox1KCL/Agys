package utils

import "unicode/utf8"

// IsUUIDv4 reports whether s matches the canonical 36-character UUID v4 format
// (xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx, case-insensitive).
func IsUUIDv4(s string) bool {
	if len(s) != 36 {
		return false
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		case 14:
			if c != '4' {
				return false
			}
		case 19:
			if c != '8' && c != '9' && c != 'a' && c != 'b' && c != 'A' && c != 'B' {
				return false
			}
		default:
			if !isHexDigit(c) {
				return false
			}
		}
	}

	return true
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// TruncateRunes shortens s to at most maxRunes runes, appending "..." when truncated.
func TruncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}

	const ellipsis = "..."
	if maxRunes <= len(ellipsis) {
		return string([]rune(s)[:maxRunes])
	}

	runes := []rune(s)
	return string(runes[:maxRunes-len(ellipsis)]) + ellipsis
}
