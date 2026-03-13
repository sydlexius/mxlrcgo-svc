package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func writeLRC(song Song, filename string, outdir string) (success bool) {
	var fn string
	if fn = filename; filename == "" {
		fn = slugify(fmt.Sprintf("%s - %s", song.Track.ArtistName, song.Track.TrackName)) + ".lrc"
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
		log.Println(err)
		return false
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && success {
			log.Printf("error closing %s: %v", fp, cerr)
			success = false
		}
	}()

	buffer := bufio.NewWriter(f)
	for _, tag := range tags {
		if _, err := buffer.WriteString(tag + "\n"); err != nil {
			log.Println(err)
			return false
		}
	}

	if len(song.Subtitles.Lines) > 0 {
		log.Println("saving synced lyrics")
		success = writeSyncedLRC(song, buffer)
		if success {
			log.Printf("synced lyrics saved: %s", fp)
		}
		return success
	}
	if song.Lyrics.LyricsBody != "" {
		log.Println("saving unsynced lyrics")
		success = writeUnsyncedLRC(song, buffer)
		if success {
			log.Printf("unsynced lyrics saved: %s", fp)
		}
		return success
	}
	if song.Track.Instrumental == 1 {
		log.Println("saving instrumental")
		success = writeInstrumentalLRC(buffer)
		if success {
			log.Printf("instrumental lyrics saved: %s", fp)
		}
		return success
	}
	log.Println("Nothing to save")
	return false
}

func writeUnsyncedLRC(song Song, buff *bufio.Writer) bool {
	lines := strings.Split(song.Lyrics.LyricsBody, "\n")
	var text string
	for _, line := range lines {
		if text = line; line == "" {
			text = "♪"
		}
		_, err := buff.WriteString("[00:00.00]" + text + "\n")
		if err != nil {
			log.Println(err)
			return false
		}
	}

	if err := buff.Flush(); err != nil {
		log.Println(err)
		return false
	}
	return true
}

func writeSyncedLRC(song Song, buff *bufio.Writer) bool {
	var text string
	var fLine string
	for _, line := range song.Subtitles.Lines {
		if text = line.Text; line.Text == "" {
			text = "♪"
		}
		fLine = fmt.Sprintf("[%02d:%02d.%02d]%s", line.Time.Minutes, line.Time.Seconds, line.Time.Hundredths, text)
		_, err := buff.WriteString(fLine + "\n")
		if err != nil {
			log.Println(err)
			return false
		}
	}

	if err := buff.Flush(); err != nil {
		log.Println(err)
		return false
	}
	return true
}

func writeInstrumentalLRC(buff *bufio.Writer) bool {
	line := "[00:00.00]♪ Instrumental ♪"
	_, err := buff.WriteString(line + "\n")
	if err != nil {
		log.Println(err)
		return false
	}
	if err := buff.Flush(); err != nil {
		log.Println(err)
		return false
	}
	return true
}
