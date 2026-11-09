package bookmarks

import (
	"reflect"
	"testing"
)

func TestAutoTags(t *testing.T) {
	cases := []struct {
		name, rules, url string
		want             []string
	}{
		{"domain", "example.com example\ntest.com test", "https://example.com/", []string{"example"}},
		{"invalid urls", "example.com example", "https://", []string{}},
		{"no scheme", "example.com example", "example.com", []string{}},
		{"port", "example.com example", "https://example.com:8080/", []string{"example"}},
		{"ignores case", "EXAMPLE.com example", "https://example.com/", []string{"example"}},
		{"all tags", "example.com one two three", "https://example.com/", []string{"one", "three", "two"}},
		{"idn rule unicode, url punycode", "रजिस्ट्री.भारत tag1", "https://www.xn--81bg3cc2b2bk5hb.xn--h2brj9c/", []string{"tag1"}},
		{"idn rule punycode, url unicode", "xn--81bg3cc2b2bk5hb.xn--h2brj9c tag1", "https://www.रजिस्ट्री.भारत/", []string{"tag1"}},
		{"path", "example.com/one one\nexample.com/two two\ntest.com test", "https://example.com/one/", []string{"one"}},
		{"path ignores case", "example.com/One one", "https://example.com/one/", []string{"one"}},
		{"path ltr", "example.com/one one\nexample.com/two two", "https://example.com/one/two", []string{"one"}},
		{"domain in path ignored", "example.com example", "https://test.com/example.com", []string{}},
		{"subdomains", "example.com example\ntest.example.com test\nsome.example.com some", "https://test.example.com/", []string{"example", "test"}},
		{"domain rtl", "example.com example", "https://example.com.bad-website.com/", []string{}},
		{"ignores scheme", "https://example.com/ https\nhttp://example.com/ http", "http://example.com/", []string{"http", "https"}},
		{"line without tags", "example.com", "https://example.com/", []string{}},
		{"query", `example.com/page?a=b tag1     # true, matches a=b
example.com/page?a=c&c=d tag2 # true
example.com/page?c=d&l=p tag3 # false, l=p doesn't exist
example.com/page?a=bb tag4    # false bb != b
example.com/page?a=b&a=c tag5 # true
example.com/page?a=B tag6     # true, case insensitive
example.com/page?A=b tag7     # true, case insensitive`,
			"https://example.com/page/some?z=x&a=b&v=b&c=d&o=p&a=c", []string{"tag1", "tag2", "tag5", "tag6", "tag7"}},
		{"query empty value", "example.com/page?a= tag1\nexample.com/page?b= tag2", "https://example.com/page/some?a=value", []string{"tag1"}},
		{"query encoded", "example.com/page?a=йцу tag1\nexample.com/page?a=%D0%B9%D1%86%D1%83 tag2", "https://example.com/page?a=%D0%B9%D1%86%D1%83", []string{"tag1", "tag2"}},
		{"fragment", "example.com/#/section/1 section1\nexample.com/#/section/2 section2", "https://example.com/#/section/1", []string{"section1"}},
		{"fragment partial", "example.com/#/section section", "https://example.com/#/section/1", []string{"section"}},
		{"fragment case", "example.com/#SECTION section", "https://example.com/#section", []string{"section"}},
		{"fragment and comment", "example.com/#section1 section1 #This is a comment", "https://example.com/#section1", []string{"section1"}},
		{"comment lines and CRLF", "# comment\r\n\r\nexample.com Go, Web\r\n", "https://example.com/", []string{"go", "web"}},
		{"empty rules", "", "https://example.com/", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AutoTags(c.rules, c.url); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
