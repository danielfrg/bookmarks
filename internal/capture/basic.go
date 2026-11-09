package capture

import (
	"bytes"
	"context"
	"encoding/base64"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/danielfrg/bookmarks/internal/fetch"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"
)

const (
	maxPageBytes     = 15 << 20
	maxResourceBytes = 5 << 20
	maxTotalBytes    = 25 << 20
	maxResources     = 100
)

// Basic is the pure-Go capturer: it fetches the page and inlines stylesheets
// and images. JavaScript is not executed and is stripped from the output.
type Basic struct {
	client *fetch.Client
	now    func() time.Time
}

// NewBasic returns a Capturer that uses c for all fetches.
func NewBasic(c *fetch.Client) Capturer {
	return &Basic{client: c, now: time.Now}
}

// Capture implements Capturer.
func (b *Basic) Capture(ctx context.Context, rawURL string) (Result, error) {
	resp, err := b.client.Get(ctx, rawURL, maxPageBytes)
	if err != nil {
		return Result{}, err
	}
	if resp.Status < 200 || resp.Status > 299 {
		return Result{}, &HTTPError{Status: resp.Status}
	}
	if resp.Truncated {
		return Result{}, ErrTooLarge
	}
	ct := resp.ContentType
	if ct == "" {
		ct = http.DetectContentType(resp.Body)
	}
	mt, _, _ := mime.ParseMediaType(ct)
	switch mt {
	case "application/pdf":
		return Result{ContentType: "application/pdf", Body: resp.Body}, nil
	case "text/html", "application/xhtml+xml":
	default:
		return Result{}, ErrUnsupported
	}

	r, err := charset.NewReader(bytes.NewReader(resp.Body), ct)
	if err != nil {
		return Result{}, err
	}
	doc, err := html.Parse(r)
	if err != nil {
		return Result{}, err
	}
	pageURL, err := url.Parse(resp.FinalURL)
	if err != nil {
		return Result{}, err
	}
	p := &processor{ctx: ctx, client: b.client, base: pageURL}
	if h := findBase(doc); h != "" {
		if u, err := pageURL.Parse(h); err == nil {
			p.base = u
		}
	}
	p.walk(doc)
	insertHeader(doc, rawURL, b.now().UTC())

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return Result{}, err
	}
	return Result{ContentType: "text/html; charset=utf-8", Body: buf.Bytes()}, nil
}

func findBase(n *html.Node) string {
	if n.Type == html.ElementNode && n.DataAtom == atom.Base {
		if v, ok := attr(n, "href"); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if h := findBase(c); h != "" {
			return h
		}
	}
	return ""
}

func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func setAttr(n *html.Node, key, val string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

func delAttr(n *html.Node, key string) {
	out := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Key != key {
			out = append(out, a)
		}
	}
	n.Attr = out
}

// insertHeader puts <meta charset> and the capture comment at the start of head.
func insertHeader(doc *html.Node, rawURL string, at time.Time) {
	var head *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if head != nil {
			return
		}
		if n.Type == html.ElementNode && n.DataAtom == atom.Head {
			head = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	if head == nil { // html.Parse always creates one, but stay safe
		head = &html.Node{Type: html.ElementNode, DataAtom: atom.Head, Data: "head"}
		doc.AppendChild(head)
	}
	meta := &html.Node{
		Type: html.ElementNode, DataAtom: atom.Meta, Data: "meta",
		Attr: []html.Attribute{{Key: "charset", Val: "utf-8"}},
	}
	safe := strings.NewReplacer("--", "-%2D", ">", "%3E").Replace(rawURL)
	comment := &html.Node{
		Type: html.CommentNode,
		Data: " bookmarks basic capture of " + safe + " at " + at.Format(time.RFC3339) + " ",
	}
	if head.FirstChild != nil {
		head.InsertBefore(meta, head.FirstChild)
	} else {
		head.AppendChild(meta)
	}
	head.InsertBefore(comment, meta.NextSibling)
}

type processor struct {
	ctx    context.Context
	client *fetch.Client
	base   *url.URL
	count  int
	total  int64
}

func (p *processor) walk(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode {
			if !p.element(c) {
				n.RemoveChild(c)
				c = next
				continue
			}
		}
		p.walk(c)
		c = next
	}
}

// element rewrites n in place and reports whether it should be kept.
func (p *processor) element(n *html.Node) bool {
	switch n.DataAtom {
	case atom.Script, atom.Noscript, atom.Iframe, atom.Base:
		return false
	case atom.Meta:
		if _, ok := attr(n, "charset"); ok {
			return false
		}
		if v, _ := attr(n, "http-equiv"); strings.EqualFold(v, "content-type") {
			return false
		}
	}
	// Event handlers.
	out := n.Attr[:0]
	for _, a := range n.Attr {
		if !(len(a.Key) > 2 && strings.HasPrefix(strings.ToLower(a.Key), "on")) {
			out = append(out, a)
		}
	}
	n.Attr = out

	switch n.DataAtom {
	case atom.Link:
		return p.link(n)
	case atom.Img:
		p.img(n)
	}
	for _, key := range []string{"href", "src"} {
		if v, ok := attr(n, key); ok {
			if abs, keep := p.absolute(p.base, v); keep {
				setAttr(n, key, abs)
			} else {
				delAttr(n, key)
			}
		}
	}
	return true
}

// absolute resolves ref against base. keep is false for javascript: URLs.
func (p *processor) absolute(base *url.URL, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(strings.ToLower(ref), "javascript:") {
		return "", false
	}
	if ref == "" || strings.HasPrefix(ref, "#") {
		return ref, true
	}
	u, err := base.Parse(ref)
	if err != nil {
		return ref, true
	}
	return u.String(), true
}

func (p *processor) link(n *html.Node) bool {
	rel, _ := attr(n, "rel")
	tokens := strings.Fields(strings.ToLower(rel))
	has := func(s string) bool {
		for _, t := range tokens {
			if t == s {
				return true
			}
		}
		return false
	}
	if has("preload") || has("modulepreload") || has("prefetch") {
		return false
	}
	href, _ := attr(n, "href")
	if href != "" {
		if abs, keep := p.absolute(p.base, href); keep {
			setAttr(n, "href", abs)
			href = abs
		} else {
			delAttr(n, "href")
			href = ""
		}
	}
	if !has("stylesheet") || href == "" {
		return true
	}
	css, finalURL, ok := p.fetchCSS(href)
	if !ok {
		return true
	}
	style := &html.Node{Type: html.ElementNode, DataAtom: atom.Style, Data: "style"}
	if m, ok := attr(n, "media"); ok {
		style.Attr = []html.Attribute{{Key: "media", Val: m}}
	}
	css = strings.ReplaceAll(css, "</style", `<\/style`)
	style.AppendChild(&html.Node{Type: html.TextNode, Data: p.absolutizeCSS(finalURL, css)})
	n.Parent.InsertBefore(style, n)
	return false
}

func (p *processor) img(n *html.Node) {
	src, _ := attr(n, "src")
	src = strings.TrimSpace(src)
	srcset, hasSet := attr(n, "srcset")
	inlined := false
	if src != "" && !strings.HasPrefix(src, "data:") {
		if abs, _ := p.absolute(p.base, src); abs != "" {
			if d, ok := p.fetchImage(abs); ok {
				setAttr(n, "src", d)
				inlined = true
			} else {
				setAttr(n, "src", abs)
			}
		}
	} else if strings.HasPrefix(src, "data:") {
		inlined = true
	}
	if !hasSet {
		return
	}
	if src == "" {
		if first := firstCandidate(srcset); first != "" {
			if abs, _ := p.absolute(p.base, first); abs != "" {
				if d, ok := p.fetchImage(abs); ok {
					setAttr(n, "src", d)
					inlined = true
				}
			}
		}
	}
	if inlined {
		delAttr(n, "srcset")
		return
	}
	setAttr(n, "srcset", p.absoluteSrcset(srcset))
}

func firstCandidate(srcset string) string {
	s := strings.TrimSpace(srcset)
	if i := strings.IndexAny(s, " \t\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimRight(s, ",")
}

func (p *processor) absoluteSrcset(srcset string) string {
	var parts []string
	for _, cand := range strings.Split(srcset, ",") {
		f := strings.Fields(cand)
		if len(f) == 0 {
			continue
		}
		if abs, keep := p.absolute(p.base, f[0]); keep {
			f[0] = abs
		}
		parts = append(parts, strings.Join(f, " "))
	}
	return strings.Join(parts, ", ")
}

// fetch gets a sub-resource, enforcing the count and size budgets.
func (p *processor) fetch(u string) (*fetch.Response, bool) {
	if p.ctx.Err() != nil || p.count >= maxResources || p.total >= maxTotalBytes {
		return nil, false
	}
	p.count++
	resp, err := p.client.Get(p.ctx, u, maxResourceBytes)
	if err != nil || resp.Status < 200 || resp.Status > 299 || resp.Truncated {
		return nil, false
	}
	if p.total+int64(len(resp.Body)) > maxTotalBytes {
		return nil, false
	}
	p.total += int64(len(resp.Body))
	return resp, true
}

func (p *processor) fetchImage(u string) (string, bool) {
	resp, ok := p.fetch(u)
	if !ok {
		return "", false
	}
	mt, _, err := mime.ParseMediaType(resp.ContentType)
	if err != nil || !strings.HasPrefix(mt, "image/") {
		return "", false
	}
	return "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(resp.Body), true
}

func (p *processor) fetchCSS(u string) (css, finalURL string, ok bool) {
	resp, ok := p.fetch(u)
	if !ok {
		return "", "", false
	}
	if resp.ContentType == "" {
		if pu, err := url.Parse(resp.FinalURL); err != nil || path.Ext(pu.Path) != ".css" {
			return "", "", false
		}
	} else if mt, _, err := mime.ParseMediaType(resp.ContentType); err != nil || mt != "text/css" {
		return "", "", false
	}
	return strings.ToValidUTF8(string(resp.Body), "�"), resp.FinalURL, true
}

var (
	cssURL    = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]*)"|'([^']*)'|([^)\s"']*))\s*\)`)
	cssImport = regexp.MustCompile(`(?i)@import\s+(?:"([^"]*)"|'([^']*)')`)
)

// absolutizeCSS makes url() and @import references absolute against the
// stylesheet's own URL so they keep working once inlined.
func (p *processor) absolutizeCSS(sheetURL, css string) string {
	base, err := url.Parse(sheetURL)
	if err != nil {
		return css
	}
	resolve := func(ref string) string {
		ref = strings.TrimSpace(ref)
		if ref == "" || strings.HasPrefix(ref, "data:") || strings.HasPrefix(ref, "#") {
			return ref
		}
		if u, err := base.Parse(ref); err == nil {
			return u.String()
		}
		return ref
	}
	css = cssURL.ReplaceAllStringFunc(css, func(m string) string {
		g := cssURL.FindStringSubmatch(m)
		switch {
		case g[1] != "" || strings.Contains(m, `"`):
			return `url("` + resolve(g[1]) + `")`
		case strings.Contains(m, "'"):
			return `url('` + resolve(g[2]) + `')`
		}
		return "url(" + resolve(g[3]) + ")"
	})
	return cssImport.ReplaceAllStringFunc(css, func(m string) string {
		g := cssImport.FindStringSubmatch(m)
		if g[1] != "" || strings.Contains(m, `"`) {
			return `@import "` + resolve(g[1]) + `"`
		}
		return `@import '` + resolve(g[2]) + `'`
	})
}
