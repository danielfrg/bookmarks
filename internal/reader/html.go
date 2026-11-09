package reader

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
)

func extractHTML(ctx context.Context, data []byte, pageURL string) (Result, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Result{}, ErrNoContent
	}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return Result{}, fmt.Errorf("parse html: %w", err)
	}
	// Single-file archives inline images as data: URIs; drop them (and the
	// images themselves) before readability to keep memory and output small.
	stripDataURIs(doc)

	var u *url.URL
	if pageURL != "" {
		u, _ = url.Parse(pageURL)
	}
	art, err := readability.FromDocument(doc, u)
	if err != nil || art.Node == nil {
		return Result{}, ErrNoContent
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	removeImages(art.Node)

	conv := converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(), commonmark.NewCommonmarkPlugin(), table.NewTablePlugin(),
	))
	var opts []converter.ConvertOptionFunc
	if u != nil && u.Host != "" {
		opts = append(opts, converter.WithDomain(u.Scheme+"://"+u.Host))
	}
	md, err := conv.ConvertNode(art.Node, opts...)
	if err != nil {
		return Result{}, fmt.Errorf("convert to markdown: %w", err)
	}
	return Result{Title: strings.TrimSpace(art.Title()), Markdown: strings.TrimSpace(string(md))}, nil
}

// stripDataURIs removes src/srcset/poster attributes holding data: URIs.
func stripDataURIs(n *html.Node) {
	if n.Type == html.ElementNode {
		attrs := n.Attr[:0]
		for _, a := range n.Attr {
			switch a.Key {
			case "src", "srcset", "poster", "data-src":
				if strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.Val)), "data:") {
					continue
				}
			}
			attrs = append(attrs, a)
		}
		n.Attr = attrs
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		stripDataURIs(c)
	}
}

// removeImages drops image-like elements; reader markdown is text only.
func removeImages(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode {
			switch c.Data {
			case "img", "picture", "svg", "video", "audio", "canvas":
				n.RemoveChild(c)
				c = next
				continue
			}
		}
		removeImages(c)
		c = next
	}
}
