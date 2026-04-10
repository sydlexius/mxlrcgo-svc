package lyrics

import (
	"fmt"
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
