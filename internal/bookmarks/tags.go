package bookmarks

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxTagLength is the longest tag name (in characters) that is kept.
const MaxTagLength = 64

// NormalizeTags splits every entry on whitespace and commas, lower-cases,
// drops empty and over-long (> MaxTagLength) names, de-duplicates and sorts.
// It never returns nil.
func NormalizeTags(names []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, n := range names {
		parts := strings.FieldsFunc(n, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
		for _, p := range parts {
			p = strings.ToLower(p)
			if p == "" || utf8.RuneCountInString(p) > MaxTagLength {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
