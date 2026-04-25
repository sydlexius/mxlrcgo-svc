package models

// Track represents a song's metadata from the Musixmatch API.
type Track struct {
	TrackName    string `json:"track_name,omitempty"`
	ArtistName   string `json:"artist_name,omitempty"`
	AlbumName    string `json:"album_name,omitempty"`
	TrackLength  int    `json:"track_length,omitempty"`
	Instrumental int    `json:"instrumental,omitempty"`
	HasLyrics    int    `json:"has_lyrics,omitempty"`
	HasSubtitles int    `json:"has_subtitles,omitempty"`
}

// Lyrics holds unsynced lyrics text.
type Lyrics struct {
	LyricsBody string `json:"lyrics_body,omitempty"`
}

// Synced holds time-synced subtitle lines.
type Synced struct {
	Lines []Lines
}

// Lines represents a single synced lyrics line with text and timestamp.
type Lines struct {
	Text string `json:"text,omitempty"`
	Time Time   `json:"time,omitempty"`
}

// Time represents a timestamp for a synced lyrics line.
type Time struct {
	Total      float64 `json:"total,omitempty"`
	Minutes    int     `json:"minutes,omitempty"`
	Seconds    int     `json:"seconds,omitempty"`
	Hundredths int     `json:"hundredths,omitempty"`
}

// Song represents the complete result from a lyrics lookup.
type Song struct {
	Track     Track
	Lyrics    Lyrics
	Subtitles Synced
}

// Inputs represents a single work item in the processing queue.
type Inputs struct {
	Track    Track
	Outdir   string
	Filename string
}

// Library represents a configured music library root.
type Library struct {
	ID        int64
	Path      string
	Name      string
	CreatedAt string
	UpdatedAt string
}

// ScanResult represents an audio file discovered during a library scan.
type ScanResult struct {
	ID        int64
	LibraryID int64
	FilePath  string
	Track     Track
	Outdir    string
	Filename  string
	Status    string
	CreatedAt string
	UpdatedAt string
}
