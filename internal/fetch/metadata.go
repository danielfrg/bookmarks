package fetch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const maxMetadataBytes = 5 << 20

// Metadata is the descriptive information extracted from a page head.
type Metadata struct {
	Title, Description, PreviewImageURL string
}

// Metadata fetches rawURL and extracts its metadata. Non-2xx is an error.
func (c *Client) Metadata(ctx context.Context, rawURL string) (Metadata, error) {
	resp, err := c.Get(ctx, rawURL, maxMetadataBytes)
	if err != nil {
		return Metadata{}, err
	}
	if resp.Status < 200 || resp.Status > 299 {
		return Metadata{}, fmt.Errorf("unexpected status %d", resp.Status)
	}
	body := resp.Body
	// Decode using Content-Type + meta sniffing.
	if r, err := charset.NewReader(bytes.NewReader(body), resp.ContentType); err == nil {
		if decoded, err := io.ReadAll(r); err == nil {
			body = decoded
		}
	}
	return parseDecoded(body, resp.FinalURL), nil
}

// ParseMetadata extracts title, description and preview image from html,
// stopping at the end of <head>. body must already be UTF-8 unless it declares
// its charset in a <meta> tag, in which case it is decoded.
func ParseMetadata(body []byte, baseURL string) Metadata {
	if r, err := charset.NewReader(bytes.NewReader(body), ""); err == nil {
		if decoded, err := io.ReadAll(r); err == nil {
			body = decoded
		}
	}
	return parseDecoded(body, baseURL)
}

// parseDecoded parses UTF-8 html; it must not decode charsets again.
func parseDecoded(body []byte, baseURL string) Metadata {
	var m Metadata
	var desc, ogDesc string
	z := html.NewTokenizer(bytes.NewReader(body))
loop:
	for {
		switch z.Next() {
		case html.ErrorToken:
			break loop
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "head" {
				break loop
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "body":
				break loop
			case "title":
				if m.Title == "" && z.Next() == html.TextToken {
					m.Title = strings.Join(strings.Fields(string(z.Text())), " ")
				}
			case "meta":
				if !hasAttr {
					continue
				}
				attrs := map[string]string{}
				for {
					k, v, more := z.TagAttr()
					attrs[strings.ToLower(string(k))] = string(v)
					if !more {
						break
					}
				}
				key := strings.ToLower(attrs["name"])
				prop := strings.ToLower(attrs["property"])
				content := strings.TrimSpace(attrs["content"])
				switch {
				case key == "description" && desc == "":
					desc = content
				case (prop == "og:description" || key == "og:description") && ogDesc == "":
					ogDesc = content
				case (prop == "og:image" || key == "og:image") && m.PreviewImageURL == "" && content != "":
					m.PreviewImageURL = absURL(baseURL, content)
				}
			}
		}
	}
	m.Description = desc
	if m.Description == "" {
		m.Description = ogDesc
	}
	return m
}

func absURL(base, ref string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}
