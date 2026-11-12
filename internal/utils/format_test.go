package utils

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "zero", at: time.Time{}, want: "-"},
		{name: "just now", at: now.Add(-20 * time.Second), want: "now"},
		{name: "minutes", at: now.Add(-15 * time.Minute), want: "15m ago"},
		{name: "hours", at: now.Add(-3 * time.Hour), want: "3h ago"},
		{name: "days", at: now.Add(-2 * 24 * time.Hour), want: "2d ago"},
		{name: "weeks", at: now.Add(-14 * 24 * time.Hour), want: "2w ago"},
		{name: "months", at: now.Add(-45 * 24 * time.Hour), want: "1mo ago"},
		{name: "years", at: now.Add(-400 * 24 * time.Hour), want: "1y ago"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RelativeTime(tc.at, now); got != tc.want {
				t.Fatalf("RelativeTime() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShortenHomeWith(t *testing.T) {
	home := "/home/nox"
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: ""},
		{input: "/home/nox", want: "~"},
		{input: "/home/nox/IT-projects/Agys", want: "~/IT-projects/Agys"},
		{input: "/home/nox2/other", want: "/home/nox2/other"},
		{input: "/tmp/work", want: "/tmp/work"},
	}

	for _, tc := range tests {
		if got := ShortenHomeWith(tc.input, home); got != tc.want {
			t.Fatalf("ShortenHomeWith(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
