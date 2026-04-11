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

// WriteLRC writes the song lyrics to an LRC or TXT file in the given output directory.
// Synced lyrics and instrumentals are written as .lrc; unsynced lyrics are written as .txt.
func (w *LRCWriter) WriteLRC(song models.Song, filename string, outdir string) (retErr error) {
	// Eligibility gate -- determine content type before touching disk.
	var writeContent func(*bufio.Writer) error
	var writeTags bool
	switch {
	case len(song.Subtitles.Lines) > 0:
		slog.Info("saving synced lyrics")
		writeContent = func(buf *bufio.Writer) error { return writeSyncedLRC(song, buf) }
		writeTags = true
	case song.Lyrics.LyricsBody != "":
		slog.Info("saving unsynced lyrics")
		writeContent = func(buf *bufio.Writer) error { return writeUnsyncedLRC(song, buf) }
		writeTags = false
	case song.Track.Instrumental == 1:
		slog.Info("saving instrumental")
		writeContent = writeInstrumentalLRC
		writeTags = true
	default:
		return fmt.Errorf("nothing to save for %s - %s", song.Track.ArtistName, song.Track.TrackName)
	}

	var fn string
	switch {
	case filename != "":
		// In dir mode the scanner sets an explicit .lrc filename. For unsynced
		// content, swap the extension to .txt so the file type matches content.
		if !writeTags {
			fn = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".txt"
			slog.Info("save unsynced lyrics", "path", filepath.Join(outdir, fn))
		} else {
			fn = filename
		}
	case writeTags:
		fn = Slugify(fmt.Sprintf("%s - %s", song.Track.ArtistName, song.Track.TrackName)) + ".lrc"
	default:
		fn = Slugify(fmt.Sprintf("%s - %s", song.Track.ArtistName, song.Track.TrackName)) + ".txt"
	}
	fp := filepath.Join(outdir, fn)

	var tags []string
	if writeTags {
		tags = []string{
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
	}

	// Write to a temp file in the same directory, then rename atomically so a
	// mid-write failure never leaves a partial .lrc at the final path.
	tmp, err := os.CreateTemp(outdir, fn+".*.tmp") //nolint:gosec // path is constructed from sanitized song metadata
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", outdir, err)
	}
	tmpPath := tmp.Name()
	tmpClosed := false
	defer func() {
		if !tmpClosed {
			if cerr := tmp.Close(); cerr != nil && retErr == nil {
				retErr = fmt.Errorf("closing %s: %w", tmpPath, cerr)
			}
		}
		if retErr != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	buffer := bufio.NewWriter(tmp)
	for _, tag := range tags {
		if _, err := buffer.WriteString(tag + "\n"); err != nil {
			return fmt.Errorf("writing tag: %w", err)
		}
	}
	if err := writeContent(buffer); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmpPath, err)
	}
	tmpClosed = true
	// Restore typical output file permissions (0666, subject to umask).
	// os.CreateTemp creates files with mode 0600; chmod before rename so the
	// final .lrc has the same permissions as a file created with os.Create.
	if err := os.Chmod(tmpPath, 0o666); err != nil { //nolint:gosec // mode is a fixed constant, not user input
		return fmt.Errorf("chmod %s: %w", tmpPath, err)
	}
	// On Windows, os.Rename fails when the destination already exists.
	// Remove it first so overwrite semantics are preserved cross-platform.
	if err := os.Remove(fp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing existing %s: %w", fp, err)
	}
	if err := os.Rename(tmpPath, fp); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, fp, err)
	}
	// Remove the opposite sidecar so format transitions never leave both files on disk.
	// Writing .lrc removes a stale .txt (upgrade), writing .txt removes a stale .lrc (downgrade).
	switch filepath.Ext(fp) {
	case ".lrc":
		stale := strings.TrimSuffix(fp, ".lrc") + ".txt"
		if err := os.Remove(stale); err != nil && !os.IsNotExist(err) {
			slog.Warn("could not remove stale sidecar", "path", stale, "error", err)
		}
	case ".txt":
		stale := strings.TrimSuffix(fp, ".txt") + ".lrc"
		if err := os.Remove(stale); err != nil && !os.IsNotExist(err) {
			slog.Warn("could not remove stale sidecar", "path", stale, "error", err)
		}
	}
	slog.Info("lyrics saved", "path", fp)
	return nil
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
	if _, err := buff.WriteString(song.Lyrics.LyricsBody); err != nil {
		return fmt.Errorf("writing unsynced lyrics: %w", err)
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
