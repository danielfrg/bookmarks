package bookmarks

import (
	"strings"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"HTTPS://Example.COM/a/?b=2&a=1", "https://example.com/a?a=1&b=2"},
		{"https://example.com/", "https://example.com"},
		{"https://x.com/p#s", "https://x.com/p#s"},
		{"http://x.com:8080/", "http://x.com:8080"},

		// trailing slash handling
		{"https://example.com/path/", "https://example.com/path"},
		{"https://example.com/path/to/page/", "https://example.com/path/to/page"},
		{"https://example.com/path", "https://example.com/path"},

		// query parameters
		{"https://example.com?z=1&a=2", "https://example.com?a=2&z=1"},
		{"https://example.com?c=3&b=2&a=1", "https://example.com?a=1&b=2&c=3"},
		{"https://example.com?param=value", "https://example.com?param=value"},
		{"https://example.com?", "https://example.com"},
		{"https://example.com?empty=&filled=value", "https://example.com?empty=&filled=value"},
		{"https://example.com?z=1&a=2&z=3&b=4", "https://example.com?a=2&b=4&z=1&z=3"},
		{"https://example.com?param=value1&param=value2", "https://example.com?param=value1&param=value2"},
		{"https://example.com?special=%21%40%23%24%25", "https://example.com?special=%21%40%23%24%25"},
		{"https://example.com?a=b+c", "https://example.com?a=b%20c"},
		{"https://example.com?flag&a=1", "https://example.com?a=1&flag="},

		// case sensitivity
		{"https://EXAMPLE.com/Path/To/Page", "https://example.com/Path/To/Page"},
		{"https://EXAMPLE.COM/API/v1/Users", "https://example.com/API/v1/Users"},
		{"HTTPS://EXAMPLE.COM/path", "https://example.com/path"},

		// encoding
		{"https://example.com/path%20with%20spaces", "https://example.com/path%20with%20spaces"},
		{"https://example.com/caf%C3%A9", "https://example.com/caf%C3%A9"},
		{"https://example.com/path?q=hello%20world", "https://example.com/path?q=hello%20world"},
		{"https://example.com/pàth", "https://example.com/pàth"},

		// protocols
		{"FTP://example.com", "ftp://example.com"},
		{"HTTP://EXAMPLE.COM", "http://example.com"},
		{"https://example.com", "https://example.com"},
		{"file:///path/to/file", "file:///path/to/file"},

		// ports
		{"https://example.com:8080", "https://example.com:8080"},
		{"https://EXAMPLE.COM:8080", "https://example.com:8080"},
		{"http://example.com:80", "http://example.com:80"},
		{"https://example.com:443", "https://example.com:443"},

		// userinfo
		{"https://user:pass@EXAMPLE.COM", "https://user:pass@example.com"},
		{"https://user@EXAMPLE.COM", "https://user@example.com"},
		{"ftp://admin:secret@EXAMPLE.COM", "ftp://admin:secret@example.com"},

		// fragments
		{"https://example.com#", "https://example.com"},
		{"https://example.com#section", "https://example.com#section"},
		{"https://EXAMPLE.COM/path#Section", "https://example.com/path#Section"},
		{"https://EXAMPLE.COM/path/#Section", "https://example.com/path#Section"},
		{"https://example.com?a=1#fragment", "https://example.com?a=1#fragment"},
		{"https://example.com?z=2&a=1#fragment", "https://example.com?a=1&z=2#fragment"},

		// edge cases
		{"", ""},
		{"   ", ""},
		{"   https://example.com   ", "https://example.com"},
		{"not-a-url", "not-a-url"},
		{"://invalid", "://invalid"},

		// IDN
		{"https://xn--fsq.xn--0zwm56d", "https://xn--fsq.xn--0zwm56d"},
		{"https://测试.中国", "https://测试.中国"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := NormalizeURL(c.in); got != c.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	for _, bad := range []string{"ftp://x", "not a url", "", "   ", "https://", "javascript:alert(1)", "https://" + strings.Repeat("a", 2049)} {
		if _, err := ValidateURL(bad); err == nil {
			t.Errorf("ValidateURL(%.40q) expected error", bad)
		}
	}
	got, err := ValidateURL("  https://x.com  ")
	if err != nil || got != "https://x.com" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := ValidateURL("HTTP://x.com/a?b=1#c"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	long := "https://x.com/" + strings.Repeat("a", 2048-len("https://x.com/"))
	if _, err := ValidateURL(long); err != nil {
		t.Errorf("2048-char URL should be valid: %v", err)
	}
}
