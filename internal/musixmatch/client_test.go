package musixmatch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sydlexius/mxlrcgo-svc/internal/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestClientName(t *testing.T) {
	if got := NewClient("token").Name(); got != "musixmatch" {
		t.Fatalf("Name() = %q; want musixmatch", got)
	}
}

func TestFindLyricsBuildsRequestAndParsesSyncedLyrics(t *testing.T) {
	client := NewClient("test-token")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("method = %q; want GET", req.Method)
		}
		if got := req.URL.Query().Get("usertoken"); got != "test-token" {
			t.Fatalf("usertoken = %q; want test-token", got)
		}
		if got := req.URL.Query().Get("q_artist"); got != "artist" {
			t.Fatalf("q_artist = %q; want artist", got)
		}
		if got := req.URL.Query().Get("q_track"); got != "title" {
			t.Fatalf("q_track = %q; want title", got)
		}
		return jsonResponse(http.StatusOK, `{
			"message": {
				"header": {"status_code": 200},
				"body": {
					"macro_calls": {
						"matcher.track.get": {
							"message": {
								"header": {"status_code": 200},
								"body": {
									"track": {
										"track_name": "title",
										"artist_name": "artist",
										"album_name": "album",
										"has_subtitles": 1,
										"has_lyrics": 1
									}
								}
							}
						},
						"track.lyrics.get": {"message": {"body": {}}},
						"track.subtitles.get": {
							"message": {
								"body": {
									"subtitle_list": [
										{
											"subtitle": {
												"subtitle_body": "[{\"text\":\"line one\",\"time\":{\"total\":1.23,\"minutes\":0,\"seconds\":1,\"hundredths\":23}}]"
											}
										}
									]
								}
							}
						}
					}
				}
			}
		}`), nil
	})}

	song, err := client.FindLyrics(context.Background(), models.Track{
		TrackName:  "title",
		ArtistName: "artist",
		AlbumName:  "album",
	})
	if err != nil {
		t.Fatalf("FindLyrics: %v", err)
	}
	if song.Track.TrackName != "title" {
		t.Fatalf("track name = %q; want title", song.Track.TrackName)
	}
	if len(song.Subtitles.Lines) != 1 {
		t.Fatalf("subtitle lines = %d; want 1", len(song.Subtitles.Lines))
	}
	if got := song.Subtitles.Lines[0].Text; got != "line one" {
		t.Fatalf("subtitle text = %q; want line one", got)
	}
}

func TestFindLyricsParsesUnsyncedLyrics(t *testing.T) {
	client := newTestClient(http.StatusOK, `{
		"message": {
			"header": {"status_code": 200},
			"body": {
				"macro_calls": {
					"matcher.track.get": {
						"message": {
							"header": {"status_code": 200},
							"body": {
								"track": {
									"track_name": "title",
									"artist_name": "artist",
									"has_subtitles": 0,
									"has_lyrics": 1
								}
							}
						}
					},
					"track.lyrics.get": {
						"message": {
							"body": {
								"lyrics": {
									"lyrics_body": "plain lyrics",
									"restricted": 0
								}
							}
						}
					},
					"track.subtitles.get": {"message": {"body": {}}}
				}
			}
		}
	}`)

	song, err := client.FindLyrics(context.Background(), models.Track{TrackName: "title", ArtistName: "artist"})
	if err != nil {
		t.Fatalf("FindLyrics: %v", err)
	}
	if got := song.Lyrics.LyricsBody; got != "plain lyrics" {
		t.Fatalf("lyrics body = %q; want plain lyrics", got)
	}
}

func TestFindLyricsAcceptsInstrumentalTrack(t *testing.T) {
	client := newTestClient(http.StatusOK, `{
		"message": {
			"header": {"status_code": 200},
			"body": {
				"macro_calls": {
					"matcher.track.get": {
						"message": {
							"header": {"status_code": 200},
							"body": {
								"track": {
									"track_name": "instrumental",
									"artist_name": "artist",
									"has_subtitles": 0,
									"has_lyrics": 0,
									"instrumental": 1
								}
							}
						}
					},
					"track.lyrics.get": {"message": {"body": {}}},
					"track.subtitles.get": {"message": {"body": {}}}
				}
			}
		}
	}`)

	song, err := client.FindLyrics(context.Background(), models.Track{TrackName: "instrumental", ArtistName: "artist"})
	if err != nil {
		t.Fatalf("FindLyrics: %v", err)
	}
	if song.Track.Instrumental != 1 {
		t.Fatalf("instrumental = %d; want 1", song.Track.Instrumental)
	}
}

func TestFindLyricsErrors(t *testing.T) {
	tests := map[string]struct {
		client  *Client
		wantErr string
	}{
		"http unauthorized": {
			client:  newTestClient(http.StatusUnauthorized, ""),
			wantErr: "too many requests",
		},
		"http not found": {
			client:  newTestClient(http.StatusNotFound, ""),
			wantErr: "no results found",
		},
		"invalid token body": {
			client: newTestClient(http.StatusOK, `{
				"message": {
					"header": {
						"status_code": 401,
						"hint": "renew"
					}
				}
			}`),
			wantErr: "invalid token",
		},
		"restricted lyrics": {
			client: newTestClient(http.StatusOK, `{
				"message": {
					"header": {"status_code": 200},
					"body": {
						"macro_calls": {
							"matcher.track.get": {
								"message": {
									"header": {"status_code": 200},
									"body": {
										"track": {
											"track_name": "title",
											"artist_name": "artist",
											"has_subtitles": 0,
											"has_lyrics": 1
										}
									}
								}
							},
							"track.lyrics.get": {
								"message": {
									"body": {
										"lyrics": {"restricted": 1}
									}
								}
							},
							"track.subtitles.get": {"message": {"body": {}}}
						}
					}
				}
			}`),
			wantErr: "restricted lyrics",
		},
		"missing track": {
			client: newTestClient(http.StatusOK, `{
				"message": {
					"header": {"status_code": 200},
					"body": {
						"macro_calls": {
							"matcher.track.get": {
								"message": {
									"header": {"status_code": 200},
									"body": {}
								}
							},
							"track.lyrics.get": {"message": {"body": {}}},
							"track.subtitles.get": {"message": {"body": {}}}
						}
					}
				}
			}`),
			wantErr: "missing track data",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := tt.client.FindLyrics(context.Background(), models.Track{TrackName: "title", ArtistName: "artist"})
			if err == nil {
				t.Fatal("FindLyrics returned nil error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q; want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestFindLyricsReturnsTransportError(t *testing.T) {
	wantErr := errors.New("network down")
	client := NewClient("token")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, wantErr
	})}

	_, err := client.FindLyrics(context.Background(), models.Track{TrackName: "title", ArtistName: "artist"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("FindLyrics error = %v; want %v", err, wantErr)
	}
}

func newTestClient(status int, body string) *Client {
	client := NewClient("token")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(status, body), nil
	})}
	return client
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
