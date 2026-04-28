package musixmatch

import (
	"context"

	"github.com/sydlexius/mxlrcgo-svc/internal/models"
)

// Fetcher abstracts lyrics lookup from the Musixmatch API.
type Fetcher interface {
	FindLyrics(ctx context.Context, track models.Track) (models.Song, error)
}
