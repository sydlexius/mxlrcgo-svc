package petitlyrics

import "errors"

// Sentinel errors returned by the Petit Lyrics client. Callers should use
// errors.Is to test for these classes rather than string-matching the message.
// These mirror the classes exposed by internal/musixmatch so the two providers
// can be handled uniformly by the worker and circuit breaker.
var (
	// ErrUnauthorized indicates HTTP 401 from petitlyrics.com. Treat as a
	// circuit-breaker signal.
	ErrUnauthorized = errors.New("petitlyrics: unauthorized")
	// ErrRateLimited indicates HTTP 403 or 429 from petitlyrics.com. Petit
	// Lyrics has no public API, so a 403 most commonly means the reverse-
	// engineered access path was blocked or throttled; both are treated as a
	// circuit-breaker signal.
	ErrRateLimited = errors.New("petitlyrics: rate limited")
	// ErrNotFound indicates no matching track or no lyrics id could be scraped
	// from the search response, meaning no usable lyrics were found.
	ErrNotFound = errors.New("petitlyrics: no results found")
)
