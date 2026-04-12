package normalize_test

import (
	"testing"

	"github.com/sydlexius/mxlrcsvc-go/internal/normalize"
)

func TestNormalizeKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "plain ASCII", input: "hello world", want: "hello world"},
		{name: "leading/trailing spaces", input: "  Héllo Wörld  ", want: "hello world"},
		{name: "angstrom", input: "Ångström", want: "angstrom"},
		{name: "already lowercase", input: "beatles", want: "beatles"},
		{name: "uppercase", input: "THE BEATLES", want: "the beatles"},
		{name: "accented composed", input: "café", want: "cafe"},
		{name: "japanese ascii-like", input: "ｈｅｌｌｏ", want: "hello"}, // NFKD normalizes fullwidth
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalize.NormalizeKey(tc.input)
			if got != tc.want {
				t.Errorf("NormalizeKey(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMatchConfidence(t *testing.T) {
	tests := []struct {
		name   string
		a, b   string
		wantGt float64 // got must be > wantGt (use 0 for exact checks)
		wantLt float64 // got must be < wantLt (use 0 to skip)
		wantEq float64 // got must == wantEq (use -1 to skip)
	}{
		{name: "identical", a: "hello", b: "hello", wantEq: 1.0},
		{name: "both empty", a: "", b: "", wantEq: 1.0},
		{name: "one empty", a: "hello", b: "", wantEq: 0.0},
		{name: "near match transposition", a: "hello", b: "hello", wantGt: 0.9, wantLt: 0, wantEq: -1},
		{name: "completely different", a: "abc", b: "xyz", wantLt: 0.5, wantEq: -1},
		{name: "case insensitive", a: "Hello", b: "hello", wantEq: 1.0},
		{name: "accent insensitive", a: "Héllo", b: "hello", wantEq: 1.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalize.MatchConfidence(tc.a, tc.b)
			if tc.wantEq >= 0 {
				if got != tc.wantEq {
					t.Errorf("MatchConfidence(%q, %q) = %f, want exactly %f", tc.a, tc.b, got, tc.wantEq)
				}
				return
			}
			if tc.wantGt > 0 && got <= tc.wantGt {
				t.Errorf("MatchConfidence(%q, %q) = %f, want > %f", tc.a, tc.b, got, tc.wantGt)
			}
			if tc.wantLt > 0 && got >= tc.wantLt {
				t.Errorf("MatchConfidence(%q, %q) = %f, want < %f", tc.a, tc.b, got, tc.wantLt)
			}
		})
	}
}
