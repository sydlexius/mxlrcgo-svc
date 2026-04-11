package scanner

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/dhowden/tag"
	"github.com/sydlexius/mxlrcsvc-go/internal/app"
	"github.com/sydlexius/mxlrcsvc-go/internal/models"
)

// supportedFileTypes lists audio file extensions that can have metadata read.
var supportedFileTypes = []string{".mp3", ".m4a", ".m4b", ".m4p", ".alac", ".flac", ".ogg", ".dsf"}

// Scanner handles parsing input sources and populating the work queue.
type Scanner struct{}

// NewScanner creates a new Scanner.
func NewScanner() *Scanner {
	return &Scanner{}
}

// AssertInput validates a "artist,title" string and returns a Track, or nil if invalid.
// Uses csv.NewReader so that fields containing commas (RFC-4180 quoting) are handled
// correctly -- this matches the csv.Writer used in app.handleFailed.
func AssertInput(song string) *models.Track {
	r := csv.NewReader(strings.NewReader(song))
	fields, err := r.Read()
	if err != nil || len(fields) != 2 {
		return nil
	}
	return &models.Track{
		ArtistName: strings.TrimSpace(fields[0]),
		TrackName:  strings.TrimSpace(fields[1]),
	}
}

// GetSongMulti processes multiple "artist,title" pairs into the work queue.
func (sc *Scanner) GetSongMulti(songList []string, savePath string, songs *app.InputsQueue) {
	for _, song := range songList {
		track := AssertInput(song)
		if track == nil {
			slog.Warn("invalid input", "song", song)
			continue
		}
		songs.Push(models.Inputs{Track: *track, Outdir: savePath, Filename: ""})
	}
}

// GetSongText reads a text file with "artist,title" lines and populates the queue.
func (sc *Scanner) GetSongText(textFn string, savePath string, songs *app.InputsQueue) error {
	f, err := os.Open(textFn) //nolint:gosec // path comes from user CLI argument
	if err != nil {
		return fmt.Errorf("opening text file %s: %w", textFn, err)
	}
	s := bufio.NewScanner(f)
	s.Split(bufio.ScanLines)
	var songList []string
	for s.Scan() {
		songList = append(songList, s.Text())
	}
	_ = f.Close()
	if err := s.Err(); err != nil {
		return fmt.Errorf("reading text file %s: %w", textFn, err)
	}
	sc.GetSongMulti(songList, savePath, songs)
	return nil
}

// GetSongDir scans a directory for audio files and populates the queue with metadata.
func (sc *Scanner) GetSongDir(dir string, songs *app.InputsQueue, update bool, limit int, depth int, bfs bool) error {
	slog.Info("scanning directory", "path", dir)
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", dir, err)
	}

	sort.Slice(files, func(i int, j int) bool {
		id1, id2 := files[i].IsDir(), files[j].IsDir()
		if id1 == id2 {
			return files[i].Name() < files[j].Name()
		}
		return bfs != id1
	})

	for _, file := range files {
		if file.IsDir() {
			if depth < limit {
				if err := sc.GetSongDir(filepath.Join(dir, file.Name()), songs, update, limit, depth+1, bfs); err != nil {
					return err
				}
			}
			continue
		}
		if filepath.Ext(file.Name()) == ".lrc" {
			continue
		}
		lrcFile := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())) + ".lrc"
		if _, err := os.Stat(filepath.Join(dir, lrcFile)); err == nil && !update {
			slog.Debug("skipping file, lyrics exist", "file", file.Name())
			continue
		}

		if !slices.Contains(supportedFileTypes, strings.ToLower(filepath.Ext(file.Name()))) {
			slog.Debug("skipping file, unsupported format", "file", file.Name())
			continue
		}

		f, err := os.Open(filepath.Join(dir, file.Name())) //nolint:gosec // path from directory scan
		if err != nil {
			slog.Warn("error reading file", "error", err)
			continue
		}

		m, err := tag.ReadFrom(f)
		_ = f.Close()
		if err != nil {
			slog.Warn("error reading metadata", "file", file.Name(), "error", err)
			continue
		}

		slog.Debug("adding file", "file", file.Name())
		song := models.Inputs{
			Track:    models.Track{ArtistName: m.Artist(), TrackName: m.Title()},
			Outdir:   dir,
			Filename: strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())) + ".lrc",
		}
		songs.Push(song)
	}
	return nil
}

// ParseInput determines the input mode and populates the work queue accordingly.
func (sc *Scanner) ParseInput(songs []string, outdir string, update bool, depth int, bfs bool, queue *app.InputsQueue) (string, error) {
	if len(songs) == 1 {
		fi, err := os.Stat(songs[0])
		if err == nil {
			if !fi.IsDir() {
				if err := sc.GetSongText(songs[0], outdir, queue); err != nil {
					return "", err
				}
				return "text", nil
			}
			if err := sc.GetSongDir(songs[0], queue, update, depth, 0, bfs); err != nil {
				return "", err
			}
			return "dir", nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("checking input path %s: %w", songs[0], err)
		}
	}
	sc.GetSongMulti(songs, outdir, queue)
	return "cli", nil
}
