package config

import "testing"

func TestValidateTLSSelection(t *testing.T) {
	cases := []struct {
		name       string
		selfSigned bool
		cert, key  string
		wantErr    bool
	}{
		{"none set", false, "", "", false},
		{"self-signed only", true, "", "", false},
		{"cert+key together", false, "/c.pem", "/k.key", false},
		{"self-signed + cert", true, "/c.pem", "", true},
		{"self-signed + key", true, "", "/k.key", true},
		{"self-signed + both", true, "/c.pem", "/k.key", true},
		{"cert alone", false, "/c.pem", "", true},
		{"key alone", false, "", "/k.key", true},
	}
	for _, c := range cases {
		err := ValidateTLSSelection(c.selfSigned, c.cert, c.key)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: ValidateTLSSelection(%v,%q,%q) err=%v wantErr=%v", c.name, c.selfSigned, c.cert, c.key, err, c.wantErr)
		}
	}
}
