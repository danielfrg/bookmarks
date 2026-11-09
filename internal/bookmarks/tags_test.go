package bookmarks

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeTags(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"brief", []string{"Go, svelte", "go", " ", "Rust"}, []string{"go", "rust", "svelte"}},
		{"nil", nil, []string{}},
		{"tabs and newlines", []string{"a\tb\nc,,d"}, []string{"a", "b", "c", "d"}},
		{"drops too long", []string{strings.Repeat("a", 65), strings.Repeat("b", 64)}, []string{strings.Repeat("b", 64)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeTags(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	if !s.FaviconsEnabled || !s.PreviewsEnabled || s.WaybackEnabled || s.DefaultUnread || s.AutoTagRules != "" || string(s.UI) != "{}" {
		t.Errorf("unexpected defaults: %+v", s)
	}
}
