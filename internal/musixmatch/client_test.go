package musixmatch

import "testing"

func TestClientName(t *testing.T) {
	if got := NewClient("token").Name(); got != "musixmatch" {
		t.Fatalf("Name() = %q; want musixmatch", got)
	}
}
