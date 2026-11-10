package bookmarks

import (
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// MaxURLLength is the longest URL accepted by ValidateURL.
const MaxURLLength = 2048

// ErrInvalidURL is wrapped by every ValidateURL failure.
var ErrInvalidURL = errors.New("invalid url")

func invalidURL(msg string) error { return errors.Join(ErrInvalidURL, errors.New(msg)) }

// ValidateURL trims raw and checks it is an absolute http(s) URL with a host
// and at most MaxURLLength characters. It returns the trimmed URL.
func ValidateURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", invalidURL("url is required")
	}
	if len(s) > MaxURLLength {
		return "", invalidURL("url is too long")
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", invalidURL("url is malformed")
	}
	if sc := strings.ToLower(u.Scheme); sc != "http" && sc != "https" {
		return "", invalidURL("url must start with http:// or https://")
	}
	if u.Hostname() == "" {
		return "", invalidURL("url must have a host")
	}
	return s, nil
}

// parsedURL mirrors the pieces of Python's urllib.parse.urlparse result that
// normalization and auto-tagging rely on. Components are raw (not
// percent-decoded).
type parsedURL struct {
	scheme   string
	netloc   string
	userinfo string
	hostname string // without userinfo/port, brackets kept for IPv6
	port     string // as written, "" when absent
	path     string
	params   string
	query    string
	fragment string
}

func splitURL(s string) parsedURL {
	var p parsedURL
	s = strings.NewReplacer("\t", "", "\r", "", "\n", "").Replace(s)

	if i := strings.IndexByte(s, ':'); i > 0 && isScheme(s[:i]) {
		p.scheme = strings.ToLower(s[:i])
		s = s[i+1:]
	}
	if strings.HasPrefix(s, "//") {
		rest := s[2:]
		end := strings.IndexAny(rest, "/?#")
		if end < 0 {
			end = len(rest)
		}
		p.netloc, s = rest[:end], rest[end:]
	}
	if i := strings.IndexByte(s, '#'); i >= 0 {
		p.fragment, s = s[i+1:], s[:i]
	}
	if i := strings.IndexByte(s, '?'); i >= 0 {
		p.query, s = s[i+1:], s[:i]
	}
	// params: ";..." on the last path segment
	seg := strings.LastIndexByte(s, '/') + 1
	if i := strings.IndexByte(s[seg:], ';'); i >= 0 {
		p.params, s = s[seg+i+1:], s[:seg+i]
	}
	p.path = s

	host := p.netloc
	if i := strings.LastIndexByte(host, '@'); i >= 0 {
		p.userinfo, host = host[:i], host[i+1:]
	}
	if strings.HasPrefix(host, "[") {
		if i := strings.IndexByte(host, ']'); i >= 0 {
			p.hostname = host[:i+1]
			if strings.HasPrefix(host[i+1:], ":") {
				p.port = host[i+2:]
			}
			return p
		}
	}
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		host, p.port = host[:i], host[i+1:]
	}
	p.hostname = host
	return p
}

func isScheme(s string) bool {
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.'):
		default:
			return false
		}
	}
	return s != ""
}

// Schemes for which Python's urlunparse always emits "//".
var usesNetloc = map[string]bool{
	"": true, "ftp": true, "http": true, "gopher": true, "nntp": true, "telnet": true,
	"imap": true, "wais": true, "file": true, "mms": true, "https": true, "shttp": true,
	"snews": true, "prospero": true, "rtsp": true, "rtspu": true, "rsync": true,
	"svn": true, "svn+ssh": true, "sftp": true, "nfs": true, "git": true,
	"git+ssh": true, "ws": true, "wss": true,
}

// NormalizeURL normalizes a URL for duplicate detection:
// lower-case scheme and host, keep userinfo and explicit port, strip trailing
// slashes from the path, sort query parameters by (key, value) and re-encode
// them, keep the fragment. Input that cannot be parsed is returned trimmed.
func NormalizeURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	p := splitURL(s)

	netloc := strings.ToLower(p.hostname)
	if p.port != "" {
		n, err := strconv.Atoi(p.port)
		if err != nil || n < 0 || n > 65535 {
			return s // Python: ValueError -> original url
		}
		if n != 0 {
			netloc += ":" + strconv.Itoa(n)
		}
	}
	if p.userinfo != "" {
		user, pass, _ := strings.Cut(p.userinfo, ":")
		if user != "" {
			auth := user
			if pass != "" {
				auth += ":" + pass
			}
			netloc = auth + "@" + netloc
		}
	}

	path := strings.TrimRight(p.path, "/")

	query := ""
	if p.query != "" {
		pairs := parseQSL(p.query)
		sort.SliceStable(pairs, func(i, j int) bool {
			if pairs[i][0] != pairs[j][0] {
				return pairs[i][0] < pairs[j][0]
			}
			return pairs[i][1] < pairs[j][1]
		})
		parts := make([]string, len(pairs))
		for i, kv := range pairs {
			parts[i] = quote(kv[0]) + "=" + quote(kv[1])
		}
		query = strings.Join(parts, "&")
	}

	// urlunparse
	out := path
	if p.params != "" {
		out += ";" + p.params
	}
	if netloc != "" || (usesNetloc[p.scheme] && p.scheme != "" && !strings.HasPrefix(out, "//")) {
		if out != "" && !strings.HasPrefix(out, "/") {
			out = "/" + out
		}
		out = "//" + netloc + out
	}
	if p.scheme != "" {
		out = p.scheme + ":" + out
	}
	if query != "" {
		out += "?" + query
	}
	if p.fragment != "" {
		out += "#" + p.fragment
	}
	return out
}

// parseQSL mirrors urllib.parse.parse_qsl(keep_blank_values=True).
func parseQSL(qs string) [][2]string {
	var out [][2]string
	for _, piece := range strings.Split(qs, "&") {
		if piece == "" {
			continue
		}
		k, v, _ := strings.Cut(piece, "=")
		out = append(out, [2]string{unquotePlus(k), unquotePlus(v)})
	}
	return out
}

// parseQS mirrors urllib.parse.parse_qs(keep_blank_values=True).
func parseQS(qs string) map[string][]string {
	m := map[string][]string{}
	for _, kv := range parseQSL(qs) {
		m[kv[0]] = append(m[kv[0]], kv[1])
	}
	return m
}

func unquotePlus(s string) string {
	s = strings.ReplaceAll(s, "+", " ")
	if !strings.Contains(s, "%") {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			n, _ := strconv.ParseUint(s[i+1:i+3], 16, 8)
			b = append(b, byte(n))
			i += 2
			continue
		}
		b = append(b, s[i])
	}
	return strings.ToValidUTF8(string(b), "�")
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// quote mirrors urllib.parse.quote(s, safe="").
func quote(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}
