package lyrics

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/sydlexius/mxlrcsvc-go/internal/models"
)

// Writer abstracts LRC file output.
type Writer interface {
	WriteLRC(song models.Song, filename string, outdir string) error
}

// LRCWriter writes songs to .lrc files.
type LRCWriter struct{}

// NewLRCWriter creates a new LRCWriter.
func NewLRCWriter() *LRCWriter {
	return &LRCWriter{}
}

// WriteLRC writes the song lyrics to an LRC file in the given output directory.
func (w *LRCWriter) WriteLRC(song models.Song, filename string, outdir string) (retErr error) {
	var fn string
	if fn = filename; filename == "" {
		fn = Slugify(fmt.Sprintf("%s - %s", song.Track.ArtistName, song.Track.TrackName)) + ".lrc"
	}
	fp := filepath.Join(outdir, fn)

	tags := []string{
		"[by:fashni]",
		fmt.Sprintf("[ar:%s]", song.Track.ArtistName),
		fmt.Sprintf("[ti:%s]", song.Track.TrackName),
	}
	if song.Track.AlbumName != "" {
		tags = append(tags, fmt.Sprintf("[al:%s]", song.Track.AlbumName))
	}
	if song.Track.TrackLength != 0 {
		tags = append(tags, fmt.Sprintf("[length:%02d:%02d]", song.Track.TrackLength/60, song.Track.TrackLength%60))
	}

	f, err := os.Create(fp) //nolint:gosec // path is constructed from sanitized song metadata
	if err != nil {
		return fmt.Errorf("creating %s: %w", fp, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && retErr == nil {
			retErr = fmt.Errorf("closing %s: %w", fp, cerr)
		}
	}()

	buffer := bufio.NewWriter(f)
	for _, tag := range tags {
		if _, err := buffer.WriteString(tag + "\n"); err != nil {
			return fmt.Errorf("writing tag: %w", err)
		}
	}

	if len(song.Subtitles.Lines) > 0 {
		slog.Info("saving synced lyrics")
		if err := writeSyncedLRC(song, buffer); err != nil {
			return err
		}
		slog.Info("lyrics saved", "path", fp, "type", "synced")
		return nil
	}
	if song.Lyrics.LyricsBody != "" {
		slog.Info("saving unsynced lyrics")
		if err := writeUnsyncedLRC(song, buffer); err != nil {
			return err
		}
		slog.Info("lyrics saved", "path", fp, "type", "unsynced")
		return nil
	}
	if song.Track.Instrumental == 1 {
		slog.Info("saving instrumental")
		if err := writeInstrumentalLRC(buffer); err != nil {
			return err
		}
		slog.Info("lyrics saved", "path", fp, "type", "instrumental")
		return nil
	}
	return fmt.Errorf("nothing to save for %s - %s", song.Track.ArtistName, song.Track.TrackName)
}

func writeSyncedLRC(song models.Song, buff *bufio.Writer) error {
	var text string
	var fLine string
	for _, line := range song.Subtitles.Lines {
		if text = line.Text; line.Text == "" {
			text = "\u266a"
		}
		fLine = fmt.Sprintf("[%02d:%02d.%02d]%s", line.Time.Minutes, line.Time.Seconds, line.Time.Hundredths, text)
		if _, err := buff.WriteString(fLine + "\n"); err != nil {
			return fmt.Errorf("writing synced line: %w", err)
		}
	}

	if err := buff.Flush(); err != nil {
		return fmt.Errorf("flushing synced lyrics: %w", err)
	}
	return nil
}

func writeUnsyncedLRC(song models.Song, buff *bufio.Writer) error {
	lines := strings.Split(song.Lyrics.LyricsBody, "\n")
	var text string
	for _, line := range lines {
		if text = line; line == "" {
			text = "\u266a"
		}
		if _, err := buff.WriteString("[00:00.00]" + text + "\n"); err != nil {
			return fmt.Errorf("writing unsynced line: %w", err)
		}
	}

	if err := buff.Flush(); err != nil {
		return fmt.Errorf("flushing unsynced lyrics: %w", err)
	}
	return nil
}

func writeInstrumentalLRC(buff *bufio.Writer) error {
	line := "[00:00.00]\u266a Instrumental \u266a"
	if _, err := buff.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("writing instrumental line: %w", err)
	}
	if err := buff.Flush(); err != nil {
		return fmt.Errorf("flushing instrumental lyrics: %w", err)
	}
	return nil
}
