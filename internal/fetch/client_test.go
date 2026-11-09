package fetch

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestClientBlocksPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hi"))
	}))
	defer srv.Close()
	c := NewClient(Options{})
	_, err := c.Get(context.Background(), srv.URL, 1024)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress", err)
	}
}

func TestClientAllowPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hi"))
	}))
	defer srv.Close()
	c := NewClient(Options{AllowPrivate: true})
	resp, err := c.Get(context.Background(), srv.URL, 1024)
	if err != nil || string(resp.Body) != "hi" || resp.Status != 200 {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestClientBlocksPrivateRedirect(t *testing.T) {
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secret"))
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, second.URL, http.StatusFound)
	}))
	defer first.Close()
	_, firstPortStr, _ := net.SplitHostPort(strings.TrimPrefix(first.URL, "http://"))
	firstPort, _ := strconv.Atoi(firstPortStr)

	c := NewClient(Options{})
	c.isAllowed = func(ip net.IP, port int) bool { return port == firstPort }
	_, err := c.Get(context.Background(), first.URL, 1024)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress", err)
	}
}

func TestClientRedirectNonHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer srv.Close()
	c := NewClient(Options{AllowPrivate: true})
	if _, err := c.Get(context.Background(), srv.URL, 1024); err == nil {
		t.Fatal("expected error")
	}
}

func TestClientRedirectLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
	}))
	defer srv.Close()
	c := NewClient(Options{AllowPrivate: true})
	if _, err := c.Get(context.Background(), srv.URL+"/", 1024); err == nil {
		t.Fatal("expected redirect limit error")
	}
}

func TestClientSchemeRejected(t *testing.T) {
	c := NewClient(Options{AllowPrivate: true})
	if _, err := c.Get(context.Background(), "ftp://example.com/x", 10); err == nil {
		t.Fatal("expected error")
	}
}

func TestClientMaxBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("a", 2048)))
	}))
	defer srv.Close()
	c := NewClient(Options{AllowPrivate: true})
	resp, err := c.Get(context.Background(), srv.URL, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Truncated || len(resp.Body) != 1024 {
		t.Fatalf("truncated=%v len=%d", resp.Truncated, len(resp.Body))
	}
	resp, _ = c.Get(context.Background(), srv.URL, 2048)
	if resp.Truncated || len(resp.Body) != 2048 {
		t.Fatalf("exact: truncated=%v len=%d", resp.Truncated, len(resp.Body))
	}
}

func TestClientNon2xxNotError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := NewClient(Options{AllowPrivate: true})
	resp, err := c.Get(context.Background(), srv.URL, 1024)
	if err != nil || resp.Status != 404 {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestClientUserAgent(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
	}))
	defer srv.Close()
	c := NewClient(Options{AllowPrivate: true})
	c.Get(context.Background(), srv.URL, 10)
	if got != DefaultUserAgent {
		t.Fatalf("ua = %q", got)
	}
	c = NewClient(Options{AllowPrivate: true, UserAgent: "x/1"})
	c.Get(context.Background(), srv.URL, 10)
	if got != "x/1" {
		t.Fatalf("ua = %q", got)
	}
}

func TestIsPublicIP(t *testing.T) {
	cases := map[string]bool{
		"10.0.0.1": false, "172.16.0.1": false, "192.168.1.1": false,
		"169.254.169.254": false, "127.0.0.1": false, "::1": false,
		"fc00::1": false, "fe80::1": false, "100.64.0.1": false,
		"0.0.0.0": false, "0.1.2.3": false, "::": false, "224.0.0.1": false,
		"255.255.255.255": false, "::ffff:10.0.0.1": false, "::ffff:127.0.0.1": false,
		"64:ff9b::808:808": false, "2002:808:808::1": false, "2001:0:4136:e378::1": false,
		"192.0.0.1": false, "198.18.0.1": false, "198.19.255.255": false,
		"1.1.1.1": true, "8.8.8.8": true, "2606:4700:4700::1111": true,
	}
	for s, want := range cases {
		if got := isPublicIP(net.ParseIP(s)); got != want {
			t.Errorf("isPublicIP(%s) = %v, want %v", s, got, want)
		}
	}
}
