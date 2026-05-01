package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/sydlexius/mxlrcgo-svc/internal/auth"
	"github.com/sydlexius/mxlrcgo-svc/internal/models"
	"github.com/sydlexius/mxlrcgo-svc/internal/queue"
)

const maxWebhookBody = 1 << 20 // 1 MiB

// Authenticator validates API keys for HTTP endpoints.
type Authenticator interface {
	ValidateKey(ctx context.Context, raw string, required auth.Scope) (auth.Key, error)
}

// WorkQueue enqueues lyrics work from webhooks.
type WorkQueue interface {
	Enqueue(ctx context.Context, inputs models.Inputs, priority int) (queue.WorkItem, error)
	Cleanup(ctx context.Context, inputs models.Inputs) (int64, error)
}

// Handler serves the HTTP API.
type Handler struct {
	auth     Authenticator
	queue    WorkQueue
	outdir   string
	priority int
	mux      *http.ServeMux
}

// NewHandler creates an HTTP API handler.
func NewHandler(a Authenticator, q WorkQueue, outdir string) *Handler {
	h := &Handler{
		auth:     a,
		queue:    q,
		outdir:   outdir,
		priority: queue.PriorityWebhook,
		mux:      http.NewServeMux(),
	}
	h.mux.HandleFunc("POST /api/v1/webhooks/lidarr", h.handleLidarr)
	return h
}

// ServeHTTP logs requests and dispatches them to API routes.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	h.mux.ServeHTTP(rec, r)
	slog.Info("http request", "method", r.Method, "uri", redactURI(r.URL), "status", rec.status) //nolint:gosec // G706: request URI is logged as a structured slog field after apikey redaction; slog escapes values
}

func (h *Handler) handleLidarr(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		http.Error(w, "auth unavailable", http.StatusInternalServerError)
		return
	}
	if h.queue == nil {
		http.Error(w, "queue unavailable", http.StatusInternalServerError)
		return
	}
	if _, err := h.auth.ValidateKey(r.Context(), apiKey(r), auth.ScopeWebhook); err != nil {
		switch {
		case errors.Is(err, auth.ErrForbiddenScope):
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		case errors.Is(err, auth.ErrInvalidKey), errors.Is(err, auth.ErrRevokedKey):
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		default:
			slog.Error("lidarr webhook authentication failed", "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	var payload lidarrWebhook
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err := dec.Decode(&payload); err != nil {
		http.Error(w, "invalid lidarr webhook payload", http.StatusBadRequest)
		return
	}

	event := strings.TrimSpace(payload.EventType)
	switch event {
	case "Download", "TrackRetag":
		inputs, err := h.inputs(payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, v := range inputs {
			if _, err := h.queue.Enqueue(r.Context(), v, h.priority); err != nil {
				slog.Error("failed to enqueue work from lidarr webhook", "event", event, "error", err, "input", v)
				http.Error(w, "enqueue failed", http.StatusInternalServerError)
				return
			}
		}
		slog.Info("lidarr webhook enqueued", "event", event, "count", len(inputs))
	case "Grab":
		slog.Info("lidarr grab received", "artist", payload.Artist.ArtistName, "album", payload.Album.Title)
	case "Rename":
		slog.Info("lidarr rename received", "artist", payload.Artist.ArtistName, "album", payload.Album.Title)
	case "Delete":
		inputs, err := h.inputs(payload)
		if err != nil {
			slog.Info("lidarr delete received without cleanup target", "artist", payload.Artist.ArtistName, "album", payload.Album.Title)
			break
		}
		var removed int64
		for _, v := range inputs {
			n, err := h.queue.Cleanup(r.Context(), v)
			if err != nil {
				slog.Error("failed to clean queued work from lidarr webhook", "event", event, "error", err, "input", v)
				http.Error(w, "cleanup failed", http.StatusInternalServerError)
				return
			}
			removed += n
		}
		slog.Info("lidarr delete cleanup completed", "artist", payload.Artist.ArtistName, "album", payload.Album.Title, "removed", removed)
	default:
		http.Error(w, "unsupported lidarr event", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (h *Handler) inputs(payload lidarrWebhook) ([]models.Inputs, error) {
	artist := strings.TrimSpace(payload.Artist.ArtistName)
	if artist == "" {
		return nil, fmt.Errorf("missing artist")
	}
	album := strings.TrimSpace(payload.Album.Title)
	tracks := payload.Tracks
	if len(tracks) == 0 && payload.Track.Title != "" {
		tracks = []lidarrTrack{payload.Track}
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("missing tracks")
	}
	inputs := make([]models.Inputs, 0, len(tracks))
	for _, track := range tracks {
		title := strings.TrimSpace(track.Title)
		if title == "" {
			return nil, fmt.Errorf("missing track title")
		}
		inputs = append(inputs, models.Inputs{
			Track: models.Track{
				ArtistName: artist,
				TrackName:  title,
				AlbumName:  album,
			},
			Outdir: h.outdir,
			OutputPaths: []models.OutputPath{{
				Outdir: h.outdir,
			}},
		})
	}
	return inputs, nil
}

func apiKey(r *http.Request) string {
	if v := strings.TrimSpace(r.URL.Query().Get("apikey")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("Authorization")); v != "" {
		scheme, token, ok := strings.Cut(v, " ")
		if ok && strings.EqualFold(scheme, "Bearer") {
			return strings.TrimSpace(token)
		}
	}
	return ""
}

func redactURI(u *url.URL) string {
	if u == nil {
		return ""
	}
	cp := *u
	q := cp.Query()
	if _, ok := q["apikey"]; ok {
		q.Set("apikey", "REDACTED")
		cp.RawQuery = q.Encode()
	}
	return cp.RequestURI()
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

type lidarrWebhook struct {
	EventType string        `json:"eventType"`
	Artist    lidarrArtist  `json:"artist"`
	Album     lidarrAlbum   `json:"album"`
	Track     lidarrTrack   `json:"track"`
	Tracks    []lidarrTrack `json:"tracks"`
}

type lidarrArtist struct {
	ArtistName string `json:"artistName"`
}

type lidarrAlbum struct {
	Title string `json:"title"`
}

type lidarrTrack struct {
	Title string `json:"title"`
}
