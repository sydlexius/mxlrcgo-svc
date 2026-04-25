package queue

import (
	"testing"

	"github.com/sydlexius/mxlrcsvc-go/internal/models"
)

func TestNext_EmptyQueue(t *testing.T) {
	q := NewInputsQueue()
	_, err := q.Next()
	if err == nil {
		t.Fatal("expected error from Next() on empty queue, got nil")
	}
}

func TestPop_EmptyQueue(t *testing.T) {
	q := NewInputsQueue()
	_, err := q.Pop()
	if err == nil {
		t.Fatal("expected error from Pop() on empty queue, got nil")
	}
}

func TestNext_NonEmptyQueue(t *testing.T) {
	q := NewInputsQueue()
	item := models.Inputs{
		Track:  models.Track{ArtistName: "Artist", TrackName: "Track"},
		Outdir: "out",
	}
	q.Push(item)

	got, err := q.Next()
	if err != nil {
		t.Fatalf("unexpected error from Next() on non-empty queue: %v", err)
	}
	if got.Track.ArtistName != item.Track.ArtistName || got.Track.TrackName != item.Track.TrackName {
		t.Fatalf("got %+v; want %+v", got, item)
	}
	// Queue should be unchanged (Next is non-destructive)
	if q.Len() != 1 {
		t.Fatalf("queue length should be 1 after Next(), got %d", q.Len())
	}
}

func TestPop_NonEmptyQueue(t *testing.T) {
	q := NewInputsQueue()
	item := models.Inputs{
		Track:  models.Track{ArtistName: "Artist", TrackName: "Track"},
		Outdir: "out",
	}
	q.Push(item)

	got, err := q.Pop()
	if err != nil {
		t.Fatalf("unexpected error from Pop() on non-empty queue: %v", err)
	}
	if got.Track.ArtistName != item.Track.ArtistName || got.Track.TrackName != item.Track.TrackName {
		t.Fatalf("got %+v; want %+v", got, item)
	}
	// Queue should be shortened by 1
	if q.Len() != 0 {
		t.Fatalf("queue length should be 0 after Pop(), got %d", q.Len())
	}
}
