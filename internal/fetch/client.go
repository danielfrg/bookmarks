// Package fetch is an SSRF-safe HTTP client plus page metadata, favicon and
// preview image helpers.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// DefaultUserAgent is a current desktop Chrome user agent.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

const (
	defaultTimeout = 30 * time.Second
	maxRedirects   = 10
)

// ErrBlockedAddress is returned when a connection targets a non-public address.
var ErrBlockedAddress = errors.New("address not allowed")

// Options configures a Client.
type Options struct {
	// AllowPrivate disables the non-public address check.
	AllowPrivate bool
	UserAgent    string
	// Timeout for a whole request; defaults to 30s.
	Timeout time.Duration
}

// Response is the result of Get.
type Response struct {
	FinalURL    string
	ContentType string
	Body        []byte
	Truncated   bool
	Status      int
}

// Client fetches URLs, refusing connections to non-public addresses.
type Client struct {
	http      *http.Client
	userAgent string
	// isAllowed is checked for every connection after DNS resolution.
	isAllowed func(ip net.IP, port int) bool
}

// NewClient builds a Client.
func NewClient(o Options) *Client {
	c := &Client{userAgent: o.UserAgent}
	if c.userAgent == "" {
		c.userAgent = DefaultUserAgent
	}
	if o.AllowPrivate {
		c.isAllowed = func(net.IP, int) bool { return true }
	} else {
		c.isAllowed = func(ip net.IP, _ int) bool { return isPublicIP(ip) }
	}
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, portStr, err := net.SplitHostPort(address)
			if err != nil {
				return ErrBlockedAddress
			}
			ip := net.ParseIP(host)
			port, _ := net.LookupPort("tcp", portStr)
			if ip == nil || !c.isAllowed(ip, port) {
				return ErrBlockedAddress
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:                 nil, // proxies would bypass the dial-time check
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
	}
	c.http = &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if s := req.URL.Scheme; s != "http" && s != "https" {
				return fmt.Errorf("redirect to unsupported scheme %q", s)
			}
			return nil
		},
	}
	return c
}

var nonPublicPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
		"169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16", "224.0.0.0/4",
		"240.0.0.0/4",                   // reserved, includes broadcast
		"192.0.0.0/24", "198.18.0.0/15", // IETF protocol assignments, benchmarking
		"64:ff9b::/96", "2002::/16", "2001::/32", // NAT64, 6to4, Teredo
		"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8",
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// isPublicIP reports whether ip is a globally routable unicast address.
func isPublicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	for _, p := range nonPublicPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// Get fetches rawURL (http/https only) and reads at most maxBytes of the body.
// Non-2xx statuses are returned, not treated as errors.
func (c *Client) Get(ctx context.Context, rawURL string, maxBytes int64) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if s := req.URL.Scheme; s != "http" && s != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q", s)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "*/*")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	truncated := int64(len(body)) > maxBytes
	if truncated {
		body = body[:maxBytes]
	}
	return &Response{
		FinalURL:    resp.Request.URL.String(),
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,
		Truncated:   truncated,
		Status:      resp.StatusCode,
	}, nil
}
