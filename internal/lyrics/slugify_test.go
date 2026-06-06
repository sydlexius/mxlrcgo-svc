package lyrics

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"\\/:*?\"<>|", ""},
		{"-Hello_", "Hello"},
		{"Hello ---- World", "Hello - World"},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("slugify=%d", i), func(t *testing.T) {
			got := Slugify(tc.input)
			if got != tc.want {
				t.Fatalf("got %v; want %v", got, tc.want)
			}
		})
	}
}

// TestSlugifyNeverReturnsSeparatorsOrAbs locks in the invariant that the writer
// relies on for the derived (Slugify) filename branch: an adversarial artist or
// track name can never produce a path separator or an absolute path. This backs
// the defense-in-depth post-compute base-name guard in WriteLRC.
func TestSlugifyNeverReturnsSeparatorsOrAbs(t *testing.T) {
	adversarial := []string{
		"AC/DC",
		`C:\evil`,
		`..\..\escape`,
		"/etc/passwd",
		"a/b/c",
		"name:with:colons",
		"foo|bar",
		"<script>",
		"....",
	}
	for _, in := range adversarial {
		got := Slugify(in)
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("Slugify(%q) = %q contains a path separator", in, got)
		}
		if filepath.IsAbs(got) {
			t.Errorf("Slugify(%q) = %q is an absolute path", in, got)
		}
	}
}
