package backoff

import (
	"testing"
	"time"
)

func TestGeometricRampsAndCaps(t *testing.T) {
	base := time.Second
	max := 4 * time.Second

	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 0, want: time.Second},
		{attempts: 1, want: time.Second},
		{attempts: 2, want: 2 * time.Second},
		{attempts: 3, want: 4 * time.Second},
		{attempts: 4, want: 4 * time.Second},
		{attempts: 100, want: 4 * time.Second},
	}
	for _, tc := range tests {
		if got := Geometric(tc.attempts, base, max); got != tc.want {
			t.Fatalf("Geometric(%d, %s, %s) = %s; want %s", tc.attempts, base, max, got, tc.want)
		}
	}
}

func TestGeometricZeroBaseOrMaxReturnsZero(t *testing.T) {
	if got := Geometric(3, 0, time.Hour); got != 0 {
		t.Fatalf("Geometric with zero base = %s; want 0", got)
	}
	if got := Geometric(3, time.Minute, 0); got != 0 {
		t.Fatalf("Geometric with zero max = %s; want 0", got)
	}
	if got := Geometric(3, -time.Minute, time.Hour); got != 0 {
		t.Fatalf("Geometric with negative base = %s; want 0", got)
	}
}

func TestGeometricDefaultsCadence(t *testing.T) {
	want := []time.Duration{
		1 * time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
		32 * time.Minute,
		time.Hour,
		time.Hour,
	}
	for i, w := range want {
		if got := Geometric(i+1, DefaultBase, DefaultMax); got != w {
			t.Fatalf("Geometric(%d, defaults) = %s; want %s", i+1, got, w)
		}
	}
}
