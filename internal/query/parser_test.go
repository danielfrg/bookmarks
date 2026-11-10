package query

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseCanonical(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"programming", "programming"},
		{"programming and books", "(and programming books)"},
		{"programming or books", "(or programming books)"},
		{"not programming", "(not programming)"},
		{"programming or books and streaming", "(or programming (and books streaming))"},
		{"not programming and books", "(and (not programming) books)"},
		{"a and b and c", "(and (and a b) c)"},
		{"a or b or c", "(or (or a b) c)"},
		{"not not a", "(not (not a))"},
		{"(a)", "a"},
		{"((a))", "a"},
		{"(a or b) and c", "(and (or a b) c)"},
		{"a and (b or c) and not client-side", "(and (and a (or b c)) (not client-side))"},
		{"a AND b OR c", "(or (and a b) c)"},
		{"a AND b OR c NOT d", "(or (and a b) (and c (not d)))"},
		{"a AND b OR c AND NOT d", "(or (and a b) (and c (not d)))"},
		{"123 and 456", "(and 123 456)"},
		{"test@example.com and file.txt", "(and test@example.com file.txt)"},
		{"https://example.com/foo/bar", "https://example.com/foo/bar"},
		{"https://github.com or https://gitlab.com", "(or https://github.com https://gitlab.com)"},
		{"android and notification", "(and android notification)"},
		{"android notarization", "(and android notarization)"},
		// Phrases.
		{`"good and bad"`, `"good and bad"`},
		{`'hello world'`, `"hello world"`},
		{`"good and bad" or programming`, `(or "good and bad" programming)`},
		{`programming and not "bad practices"`, `(and programming (not "bad practices"))`},
		{`("good morning" or "good evening") and coffee`, `(and (or "good morning" "good evening") coffee)`},
		{`"say \"hello\""`, `"say \"hello\""`},
		{`'don\'t worry'`, `"don't worry"`},
		{`"a\nb"`, `"a\nb"`},
		{`"a\qb"`, `"aqb"`},
		{`"unclosed quote`, `"unclosed quote"`},
		{`'!unread and !untagged'`, `"!unread and !untagged"`},
		// Tags and keywords.
		{"#python", "#python"},
		{"#Machine-Learning", "#machine-learning"},
		{"#Go !UNREAD", "(and #go !unread)"},
		{"not #x", "(not #x)"},
		{"#frontend or #backend", "(or #frontend #backend)"},
		{"(#frontend or #backend) and javascript", "(and (or #frontend #backend) javascript)"},
		{"!unread and !untagged", "(and !unread !untagged)"},
		{"not !unread", "(not !unread)"},
		{`"machine learning" and #python`, `(and "machine learning" #python)`},
		{"Hello #Tag", "(and Hello #tag)"},
		// Lone # and ! are dropped.
		{"#", ""},
		{"# ", ""},
		{"!", ""},
		{"python # and django", "(and python django)"},
		// Implicit AND.
		{"python machine learning", "(and (and python machine) learning)"},
		{"tutorial #python", "(and tutorial #python)"},
		{`"deep learning" #python tutorial`, `(and (and "deep learning" #python) tutorial)`},
		{"python tutorial or java guide", "(or (and python tutorial) (and java guide))"},
		{"a or b c", "(or a (and b c))"},
		{"not deprecated tutorial", "(and (not deprecated) tutorial)"},
		{"python tutorial not deprecated", "(and (and python tutorial) (not deprecated))"},
		{"(python tutorial) or java", "(or (and python tutorial) java)"},
		{"machine learning and (python or r) tutorial #beginner",
			"(and (and (and (and machine learning) (or python r)) tutorial) #beginner)"},
		{"frontend and (react or vue or angular) and not jquery",
			"(and (and frontend (or (or react vue) angular)) (not jquery))"},
		// Terms split at special characters.
		{"foo#bar", "(and foo #bar)"},
		{"foo(bar)", "(and foo bar)"},
		{"  a   and    b  ", "(and a b)"},
	}
	for _, c := range cases {
		e, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", c.in, err)
			continue
		}
		if got := String(e); got != c.want {
			t.Errorf("Parse(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestParseNilOnEmpty(t *testing.T) {
	for _, in := range []string{"", "  \t\n", "#", "!"} {
		e, err := Parse(in)
		if e != nil || err != nil {
			t.Errorf("Parse(%q) = %v, %v; want nil, nil", in, e, err)
		}
	}
}

func TestParseStructure(t *testing.T) {
	got, err := Parse("#Go !UNREAD")
	if err != nil {
		t.Fatal(err)
	}
	want := And{Tag{"go"}, Keyword{"unread"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v want %#v", got, want)
	}

	got, _ = Parse(`a or "b c"`)
	want2 := Or{Term{Text: "a"}, Term{Text: "b c", Phrase: true}}
	if !reflect.DeepEqual(got, want2) {
		t.Errorf("got %#v want %#v", got, want2)
	}

	got, _ = Parse("not #x")
	if !reflect.DeepEqual(got, Not{Tag{"x"}}) {
		t.Errorf("got %#v", got)
	}
}

func TestParseKeepsTermCase(t *testing.T) {
	got, _ := Parse("API")
	if !reflect.DeepEqual(got, Term{Text: "API"}) {
		t.Errorf("got %#v", got)
	}
}

func TestSyntaxErrors(t *testing.T) {
	cases := []struct {
		in  string
		msg string
		pos int
	}{
		{"(a and b", "expected ')' but found end of query", 8},
		{"a and b)", "unexpected ')'", 7},
		{"()", "unexpected ')'", 1},
		{"and", "unexpected 'and'", 0},
		{"a and", "unexpected end of query", 5},
		{"a and or b", "unexpected 'or'", 6},
		{"not", "unexpected end of query", 3},
		{"a or", "unexpected end of query", 4},
		{"  (a", "expected ')' but found end of query", 2},
	}
	for _, c := range cases {
		e, err := Parse(c.in)
		var se *SyntaxError
		if !errors.As(err, &se) {
			t.Errorf("Parse(%q) err = %v, want *SyntaxError", c.in, err)
			continue
		}
		if e != nil {
			t.Errorf("Parse(%q) returned expr with error", c.in)
		}
		if se.Msg != c.msg || se.Pos != c.pos {
			t.Errorf("Parse(%q) = {%q, %d}, want {%q, %d}", c.in, se.Msg, se.Pos, c.msg, c.pos)
		}
	}
	_, err := Parse("(a")
	if err.Error() == "" {
		t.Error("empty error string")
	}
}

func TestPositionsAreRunes(t *testing.T) {
	_, err := Parse("é )")
	var se *SyntaxError
	if !errors.As(err, &se) || se.Pos != 2 {
		t.Errorf("err = %v", err)
	}
}

func TestAndAll(t *testing.T) {
	if AndAll() != nil || AndAll(nil, nil) != nil {
		t.Error("want nil")
	}
	a, b, c := Term{Text: "a"}, Tag{"b"}, Keyword{"c"}
	if got := AndAll(nil, a, nil); !reflect.DeepEqual(got, a) {
		t.Errorf("got %#v", got)
	}
	if got := String(AndAll(a, nil, b, c)); got != "(and (and a #b) !c)" {
		t.Errorf("got %s", got)
	}
}

func TestStringNil(t *testing.T) {
	if String(nil) != "" {
		t.Error("want empty")
	}
}
