package musixmatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sydlexius/mxlrcgo-svc/internal/models"
	"github.com/valyala/fastjson"
)

const apiURL = "https://apic-desktop.musixmatch.com/ws/1.1/macro.subtitles.get"

// Client communicates with the Musixmatch desktop API.
type Client struct {
	Token      string
	httpClient *http.Client
}

// NewClient creates a new Musixmatch API client.
func NewClient(token string) *Client {
	return &Client{
		Token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// FindLyrics looks up lyrics for the given track from the Musixmatch API.
func (c *Client) FindLyrics(ctx context.Context, track models.Track) (models.Song, error) {
	song := models.Song{}
	baseURL, err := url.Parse(apiURL)
	if err != nil {
		return song, fmt.Errorf("failed to parse API URL: %w", err)
	}
	params := url.Values{
		"format":            {"json"},
		"namespace":         {"lyrics_richsynched"},
		"subtitle_format":   {"mxm"},
		"app_id":            {"web-desktop-app-v1.0"},
		"usertoken":         {c.Token},
		"q_album":           {track.AlbumName},
		"q_artist":          {track.ArtistName},
		"q_artists":         {track.ArtistName},
		"q_track":           {track.TrackName},
		"track_spotify_id":  {""},
		"q_duration":        {""},
		"f_subtitle_length": {""},
	}
	baseURL.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL.String(), nil)
	if err != nil {
		return song, err
	}

	req.Header = http.Header{
		"authority": {"apic-desktop.musixmatch.com"},
		"cookie":    {"x-mxm-token-guid="},
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return song, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		switch res.StatusCode {
		case http.StatusUnauthorized:
			return song, errors.New("too many requests: increase the cooldown time and try again in a few minutes")
		case http.StatusNotFound:
			return song, errors.New("no results found")
		default:
			errBody, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
			return song, fmt.Errorf("musixmatch API error: status %d, body: %s", res.StatusCode, strings.TrimSpace(string(errBody)))
		}
	}

	const maxResponseSize = 2 << 20 // 2 MiB
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseSize+1))
	if err != nil {
		return song, err
	}
	if len(body) > maxResponseSize {
		return song, fmt.Errorf("musixmatch API response too large (%d bytes)", len(body))
	}

	var p fastjson.Parser
	v, err := p.Parse(string(body))
	if err != nil {
		return song, err
	}

	if v.GetInt("message", "header", "status_code") == 401 && string(v.GetStringBytes("message", "header", "hint")) == "renew" {
		return song, errors.New("invalid token")
	}

	mtg := v.Get("message", "body", "macro_calls", "matcher.track.get", "message")
	tlg := v.Get("message", "body", "macro_calls", "track.lyrics.get", "message")
	tsg := v.Get("message", "body", "macro_calls", "track.subtitles.get", "message")

	switch mtg.GetInt("header", "status_code") {
	case 200:
		trackNode := mtg.Get("body", "track")
		if trackNode == nil {
			return song, errors.New("musixmatch API response missing track data")
		}
		if err := json.Unmarshal(trackNode.MarshalTo(nil), &song.Track); err != nil {
			return song, err
		}
	case 401:
		return song, errors.New("too many requests: increase the cooldown time and try again in a few minutes")
	case 404:
		return song, errors.New("no results found")
	default:
		return song, errors.New("unknown error")
	}

	if song.Track.HasSubtitles == 1 {
		if err := json.Unmarshal(tsg.GetStringBytes("body", "subtitle_list", "0", "subtitle", "subtitle_body"), &song.Subtitles.Lines); err != nil {
			return song, err
		}
	} else {
		slog.Info("no synced lyrics found")
		if song.Track.HasLyrics == 1 {
			if tlg.GetInt("body", "lyrics", "restricted") == 1 {
				return song, errors.New("restricted lyrics")
			}
			lyricsNode := tlg.Get("body", "lyrics")
			if lyricsNode == nil {
				return song, errors.New("musixmatch API response missing lyrics data")
			}
			if err := json.Unmarshal(lyricsNode.MarshalTo(nil), &song.Lyrics); err != nil {
				return song, err
			}
		} else if song.Track.Instrumental == 1 {
			slog.Info("song is instrumental")
		} else {
			return song, errors.New("no lyrics found")
		}
	}
	return song, nil
}
