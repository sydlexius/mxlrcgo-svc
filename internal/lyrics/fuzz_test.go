package lyrics

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzSlugify(f *testing.F) {
	f.Add("")
	f.Add("Artist - Track")
	f.Add(`\/:*?"<>|`)
	f.Add("  Cafe\u0301 ---- \u4e16\u754c  ")

	f.Fuzz(func(t *testing.T, s string) {
		got := Slugify(s)
		if !utf8.ValidString(got) {
			t.Fatalf("Slugify returned invalid UTF-8: %q", got)
		}
		if strings.ContainsAny(got, `\/:*?"<>|`) {
			t.Fatalf("Slugify returned forbidden filename characters: %q", got)
		}
		if strings.HasPrefix(got, "-") || strings.HasPrefix(got, "_") ||
			strings.HasSuffix(got, "-") || strings.HasSuffix(got, "_") {
			t.Fatalf("Slugify returned untrimmed edge separator: %q", got)
		}
	})
}
