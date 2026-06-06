package lyrics

import "testing"

func TestIsUnsafeBaseName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"plain base name", "Adele - Hello.lrc", false},
		{"empty", "", false},
		{"dotfile is a valid base name", ".lrc", false},
		{"forward slash", "a/b.lrc", true},
		{"back slash", `a\b.lrc`, true},
		{"absolute unix", "/etc/passwd", true},
		{"nested traversal", "../escape.lrc", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUnsafeBaseName(tc.input); got != tc.want {
				t.Errorf("isUnsafeBaseName(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
