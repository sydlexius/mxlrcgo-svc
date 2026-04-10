package lyrics

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Package-level compiled regexes (was compiled per-call in utils.go).
var (
	reForbidden = regexp.MustCompile(`[\\\/:*?"<>|]`) // forbidden chars in filename
	reMultiDash = regexp.MustCompile(`[-]+`)          // multiple dashes
)

// Slugify sanitizes a string for use as a filename.
func Slugify(s string) string {
	s = norm.NFKC.String(s)
	s = reForbidden.ReplaceAllString(s, "")
	s = reMultiDash.ReplaceAllString(s, "-")
	return strings.Trim(s, "-_")
}
