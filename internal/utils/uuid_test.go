package utils

import "testing"

func TestIsUUIDv4(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "valid lowercase", input: "05551c4c-c30f-41c0-8f9b-be6798f23c6f", want: true},
		{name: "valid uppercase", input: "DB42036D-1D96-4F7F-97A3-E7512D83A4C0", want: true},
		{name: "non-uuid dir", input: "tempmediaStorage", want: false},
		{name: "wrong version", input: "05551c4c-c30f-11c0-8f9b-be6798f23c6f", want: false},
		{name: "wrong variant", input: "05551c4c-c30f-41c0-0f9b-be6798f23c6f", want: false},
		{name: "bad length", input: "05551c4c-c30f-41c0-8f9b-be6798f23c6", want: false},
		{name: "non-hex char", input: "05551c4c-c30f-41c0-8f9b-be6798f23c6z", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUUIDv4(tc.input); got != tc.want {
				t.Fatalf("IsUUIDv4(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := TruncateRunes("Hello 🌍 World", 20); got != "Hello 🌍 World" {
		t.Fatalf("unexpected short result: %q", got)
	}
	if got := TruncateRunes("Café CLI utility tool", 11); got != "Café CLI..." {
		t.Fatalf("unexpected truncated result: %q", got)
	}
}
