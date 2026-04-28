package normalize_test

import (
	"testing"

	"github.com/sydlexius/mxlrcgo-svc/internal/normalize"
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
		{name: "japanese ascii-like", input: "ｈｅｌｌｏ", want: "hello"},        // NFKD normalizes fullwidth
		{name: "invalid utf-8 replaced", input: "hell\x80o", want: "hello"}, // invalid byte stripped
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

func fptr(f float64) *float64 { return &f }

func TestMatchConfidence(t *testing.T) {
	tests := []struct {
		name   string
		a, b   string
		wantGt *float64 // got must be > *wantGt; nil to skip
		wantLt *float64 // got must be < *wantLt; nil to skip
		wantEq *float64 // exact expected value; nil to skip
	}{
		{name: "identical", a: "hello", b: "hello", wantEq: fptr(1.0)},
		{name: "both empty", a: "", b: "", wantEq: fptr(1.0)},
		{name: "one empty", a: "hello", b: "", wantEq: fptr(0.0)},
		{name: "near match transposition", a: "hello", b: "helol", wantGt: fptr(0.9), wantLt: fptr(1.0)},
		{name: "completely different", a: "abc", b: "xyz", wantLt: fptr(0.5)},
		{name: "case insensitive", a: "Hello", b: "hello", wantEq: fptr(1.0)},
		{name: "accent insensitive", a: "Héllo", b: "hello", wantEq: fptr(1.0)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantEq != nil && (tc.wantGt != nil || tc.wantLt != nil) {
				t.Fatalf("invalid test case %q: wantEq cannot be combined with wantGt/wantLt", tc.name)
			}
			got := normalize.MatchConfidence(tc.a, tc.b)
			if tc.wantEq != nil {
				if got != *tc.wantEq {
					t.Errorf("MatchConfidence(%q, %q) = %f, want exactly %f", tc.a, tc.b, got, *tc.wantEq)
				}
				return
			}
			if tc.wantGt != nil && got <= *tc.wantGt {
				t.Errorf("MatchConfidence(%q, %q) = %f, want > %f", tc.a, tc.b, got, *tc.wantGt)
			}
			if tc.wantLt != nil && got >= *tc.wantLt {
				t.Errorf("MatchConfidence(%q, %q) = %f, want < %f", tc.a, tc.b, got, *tc.wantLt)
			}
		})
	}
}
