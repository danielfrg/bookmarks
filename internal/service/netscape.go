package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/store"
	"golang.org/x/net/html"
)

// Netscape bookmark files use linkding's conventions: notes live inside the
// <DD> description between these markers. linkding also marks its archived
// bookmarks with a pseudo-tag; this app has no such state, so an import drops
// that tag (it never becomes a real tag) and an export never writes it.
const (
	notesOpen           = "[linkding-notes]"
	notesClose          = "[/linkding-notes]"
	linkdingArchivedTag = "linkding:bookmarks.archived"
)

// ImportResult counts what an import did.
type ImportResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

// netscapeEntry is one bookmark of a Netscape file.
type netscapeEntry struct {
	href, title, description, notes string
	added, modified                 *time.Time
	tags                            []string
	unread                          bool
}

// parseNetscape tokenizes a Netscape bookmark file leniently (real exports are
// not well-formed HTML) and calls fn for every <A> entry in document order.
// Parsing stops at the first error from fn or from r.
func parseNetscape(r io.Reader, fn func(netscapeEntry) error) error {
	z := html.NewTokenizer(r)
	var (
		cur    *netscapeEntry
		title  strings.Builder
		desc   strings.Builder
		inA    bool
		inDD   bool
		flushE = func() error {
			if cur == nil {
				return nil
			}
			e := *cur
			cur = nil
			e.title = strings.TrimSpace(title.String())
			e.description, e.notes = splitNotes(strings.TrimSpace(desc.String()))
			title.Reset()
			desc.Reset()
			inA, inDD = false, false
			return fn(e)
		}
	)
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if err := z.Err(); err != io.EOF {
				return err
			}
			return flushE()
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "a":
				if err := flushE(); err != nil {
					return err
				}
				cur = &netscapeEntry{}
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					cur.setAttr(string(k), string(v))
				}
				inA = true
			case "dd":
				inDD = cur != nil
				inA = false
			case "dt", "dl", "h3":
				if err := flushE(); err != nil {
					return err
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "a":
				inA = false
			case "dl":
				if err := flushE(); err != nil {
					return err
				}
			}
		case html.TextToken:
			switch {
			case inA:
				title.Write(z.Text())
			case inDD:
				desc.Write(z.Text())
			}
		}
	}
}

func (e *netscapeEntry) setAttr(key, val string) {
	switch strings.ToLower(key) {
	case "href":
		e.href = strings.TrimSpace(val)
	case "add_date":
		e.added = unixTime(val)
	case "last_modified":
		e.modified = unixTime(val)
	case "toread":
		e.unread = val == "1"
	case "tags":
		for _, t := range strings.Split(val, ",") {
			t = strings.TrimSpace(t)
			switch {
			case t == "":
			case strings.EqualFold(t, linkdingArchivedTag):
				// Dropped: not a tag, and there is no archived state.
			default:
				e.tags = append(e.tags, t)
			}
		}
	}
}

// unixTime parses a Unix timestamp in seconds. Some exporters write
// milliseconds or microseconds, so large values are scaled down.
func unixTime(s string) *time.Time {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || v <= 0 {
		return nil
	}
	switch {
	case v >= 1e15:
		v /= 1e6
	case v >= 1e11:
		v /= 1e3
	}
	t := time.Unix(v, 0).UTC()
	return &t
}

// splitNotes separates the description from the [linkding-notes] section.
func splitNotes(desc string) (description, notes string) {
	i := strings.Index(desc, notesOpen)
	if i < 0 {
		return desc, ""
	}
	rest := desc[i+len(notesOpen):]
	if j := strings.Index(rest, notesClose); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(desc[:i]), rest
}

// ImportNetscape reads a Netscape bookmark file and saves every entry without
// server capture. Entries whose URL is invalid are skipped; an existing
// bookmark with the same URL is updated. It returns the counts so far along
// with any read or store error.
func (s *Service) ImportNetscape(ctx context.Context, r io.Reader) (ImportResult, error) {
	var res ImportResult
	err := parseNetscape(r, func(e netscapeEntry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		in := SaveInput{
			URL: e.href, Description: &e.description, Tags: &e.tags,
			Unread:    &e.unread,
			DateAdded: e.added, DateModified: e.modified, Capture: CaptureNone,
		}
		if e.title != "" {
			in.Title = &e.title
		}
		if e.notes != "" {
			in.Notes = &e.notes
		}
		// Like linkding, importing over an existing bookmark merges: tags are
		// unioned and empty or absent file values never erase what is stored.
		if u, err := validateURL(e.href); err == nil {
			existing, err := s.st.GetBookmarkByURL(ctx, u)
			switch {
			case err == nil:
				merged := append(append([]string(nil), existing.Tags...), e.tags...)
				in.Tags = &merged
				// An import never rewrites when an existing bookmark was added or modified.
				in.DateAdded, in.DateModified = nil, nil
				if e.description == "" {
					in.Description = nil
				}
				if !e.unread {
					in.Unread = nil
				}
			case !errors.Is(err, store.ErrNotFound):
				return err
			}
		}
		_, created, err := s.Save(ctx, in)
		var ve *ValidationError
		switch {
		case errors.As(err, &ve):
			res.Skipped++
		case err != nil:
			return err
		case created:
			res.Created++
		default:
			res.Updated++
		}
		return nil
	})
	return res, err
}

const exportPageSize = 500

// ExportNetscape writes every bookmark as a Netscape
// bookmark file in linkding's format, oldest first.
//
// Every bookmark is loaded before the first byte is written, so a search
// failure surfaces as an error instead of a truncated file.
func (s *Service) ExportNetscape(ctx context.Context, w io.Writer) error {
	var all []bookmarks.Bookmark
	q := SearchInput{Sort: "added_asc", Limit: exportPageSize}
	for {
		res, err := s.Search(ctx, q)
		if err != nil {
			return err
		}
		for _, h := range res.Hits {
			all = append(all, h.Bookmark)
		}
		q.Offset += len(res.Hits)
		if len(res.Hits) == 0 || q.Offset >= res.Total {
			break
		}
	}
	bw := bufio.NewWriter(w)
	bw.WriteString("<!DOCTYPE NETSCAPE-Bookmark-file-1>\n")
	bw.WriteString("<META HTTP-EQUIV=\"Content-Type\" CONTENT=\"text/html; charset=UTF-8\">\n")
	bw.WriteString("<TITLE>Bookmarks</TITLE>\n")
	bw.WriteString("<H1>Bookmarks</H1>\n")
	bw.WriteString("<DL><p>\n")
	for _, b := range all {
		writeNetscapeBookmark(bw, b)
	}
	bw.WriteString("</DL><p>\n")
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("write export: %w", err)
	}
	return nil
}

func writeNetscapeBookmark(w *bufio.Writer, b bookmarks.Bookmark) {
	title := b.Title
	if title == "" {
		title = b.URL
	}
	desc := stdhtml.EscapeString(b.Description)
	if b.Notes != "" {
		desc += notesOpen + stdhtml.EscapeString(b.Notes) + notesClose
	}
	tags := append([]string(nil), b.Tags...)
	for i, t := range tags {
		tags[i] = stdhtml.EscapeString(t)
	}
	toread := "0"
	if b.Unread {
		toread = "1"
	}
	fmt.Fprintf(w, "<DT><A HREF=\"%s\" ADD_DATE=\"%d\" LAST_MODIFIED=\"%d\" PRIVATE=\"1\" TOREAD=\"%s\" TAGS=\"%s\">%s</A>\n",
		stdhtml.EscapeString(b.URL), b.DateAdded.Unix(), b.DateModified.Unix(), toread, strings.Join(tags, ","), stdhtml.EscapeString(title))
	if desc != "" {
		fmt.Fprintf(w, "<DD>%s\n", desc)
	}
}
