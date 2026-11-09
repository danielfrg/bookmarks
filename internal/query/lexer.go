package query

import (
	"strings"
	"unicode"
)

type tokenKind int

const (
	tokTerm tokenKind = iota
	tokPhrase
	tokTag
	tokKeyword
	tokAnd
	tokOr
	tokNot
	tokLParen
	tokRParen
	tokEOF
)

type token struct {
	kind  tokenKind
	value string
	pos   int // rune offset
}

// lex mirrors linkding's SearchQueryTokenizer, including its leniency: a lone
// "#" or "!" is dropped, an unclosed quote runs to the end of the input, and a
// bare word stops at any of ( ) " ' # !.
func lex(s string) []token {
	r := []rune(strings.TrimSpace(s))
	var toks []token
	i := 0
	for i < len(r) {
		for i < len(r) && unicode.IsSpace(r[i]) {
			i++
		}
		if i >= len(r) {
			break
		}
		start := i
		switch c := r[i]; {
		case c == '(':
			toks = append(toks, token{tokLParen, "(", start})
			i++
		case c == ')':
			toks = append(toks, token{tokRParen, ")", start})
			i++
		case c == '"' || c == '\'':
			i++
			var sb strings.Builder
			for i < len(r) && r[i] != c {
				if r[i] == '\\' {
					i++
					if i < len(r) {
						switch r[i] {
						case 'n':
							sb.WriteRune('\n')
						case 't':
							sb.WriteRune('\t')
						case 'r':
							sb.WriteRune('\r')
						default: // \\, the quote itself, or any other char: as-is
							sb.WriteRune(r[i])
						}
						i++
					}
					continue
				}
				sb.WriteRune(r[i])
				i++
			}
			if i < len(r) {
				i++ // closing quote
			}
			toks = append(toks, token{tokPhrase, sb.String(), start})
		case c == '#' || c == '!':
			i++
			b := i
			for i < len(r) && !unicode.IsSpace(r[i]) && !strings.ContainsRune("()\"'", r[i]) {
				i++
			}
			if i > b {
				kind := tokTag
				if c == '!' {
					kind = tokKeyword
				}
				toks = append(toks, token{kind, string(r[b:i]), start})
			}
		default:
			for i < len(r) && !unicode.IsSpace(r[i]) && !strings.ContainsRune("()\"'#!", r[i]) {
				i++
			}
			w := string(r[start:i])
			switch strings.ToLower(w) {
			case "and":
				toks = append(toks, token{tokAnd, w, start})
			case "or":
				toks = append(toks, token{tokOr, w, start})
			case "not":
				toks = append(toks, token{tokNot, w, start})
			default:
				toks = append(toks, token{tokTerm, w, start})
			}
		}
	}
	return append(toks, token{tokEOF, "", len(r)})
}
