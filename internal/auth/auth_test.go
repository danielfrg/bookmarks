package auth

import "testing"

func TestTokenFromHeader(t *testing.T) {
	for _, c := range []struct {
		in  string
		tok string
		ok  bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer  abc ", "abc", true},
		{"Token abc", "abc", true},
		{"Basic Zm9v", "", false},
		{"", "", false},
		{"Bearer", "", true},
	} {
		tok, ok := TokenFromHeader(c.in)
		if tok != c.tok || ok != c.ok {
			t.Errorf("%q: got %q,%v", c.in, tok, ok)
		}
	}
}
