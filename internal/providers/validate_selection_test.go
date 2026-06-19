package providers

import "testing"

func TestValidateSelection(t *testing.T) {
	cases := []struct {
		name     string
		primary  string
		disabled []string
		wantErr  bool
	}{
		{"nothing disabled", "musixmatch", nil, false},
		{"non-primary disabled", "musixmatch", []string{"petitlyrics"}, false},
		{"primary disabled", "musixmatch", []string{"musixmatch"}, true},
		{"all disabled", "musixmatch", []string{"musixmatch", "petitlyrics"}, true},
		{"empty primary defaults to musixmatch, not disabled", "", []string{"petitlyrics"}, false},
		{"empty primary defaults to musixmatch, disabled", "", []string{"musixmatch"}, true},
		{"case-insensitive primary", "MusixMatch", []string{"musixmatch"}, true},
	}
	for _, c := range cases {
		err := ValidateSelection(c.primary, c.disabled)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: ValidateSelection(%q,%v) err=%v, wantErr=%v", c.name, c.primary, c.disabled, err, c.wantErr)
		}
	}
}
