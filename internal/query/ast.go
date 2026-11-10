// Package query parses search queries into an AST.
//
// Grammar: words, "quoted
// phrases" (single or double quotes, backslash escapes), #tag, !keyword,
// case-insensitive and/or/not, parentheses, and implicit AND between adjacent
// operands. Precedence is NOT > AND > OR; AND and OR are left-associative.
package query

import (
	"fmt"
	"strconv"
	"strings"
)

// Expr is a node of a parsed search query.
type Expr interface{ expr() }

// Term is a free-text word, or a quoted phrase when Phrase is true. Text keeps
// its original case.
type Term struct {
	Text   string
	Phrase bool
}

// Tag matches a tag by name (lower-cased, without the "#").
type Tag struct{ Name string }

// Keyword is a special keyword such as "unread" (lower-cased, without the "!").
type Keyword struct{ Name string }

// Not negates X.
type Not struct{ X Expr }

// And requires both L and R.
type And struct{ L, R Expr }

// Or requires L or R.
type Or struct{ L, R Expr }

func (Term) expr()    {}
func (Tag) expr()     {}
func (Keyword) expr() {}
func (Not) expr()     {}
func (And) expr()     {}
func (Or) expr()      {}

// SyntaxError is returned by Parse for malformed queries. Pos is the rune
// offset in the whitespace-trimmed query.
type SyntaxError struct {
	Msg string
	Pos int
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("%s at position %d", e.Msg, e.Pos)
}

// AndAll joins the non-nil expressions with AND, left-associatively. It
// returns nil when there are none.
func AndAll(xs ...Expr) Expr {
	var out Expr
	for _, x := range xs {
		if x == nil {
			continue
		}
		if out == nil {
			out = x
			continue
		}
		out = And{out, x}
	}
	return out
}

// String returns the canonical S-expression form of e, used by tests and
// stored saved searches. It is unambiguous and fully parenthesised:
//
//	word        -> word            (bare term)
//	"a b"       -> "a b"           (phrase, Go-quoted)
//	#tag        -> #tag
//	!keyword    -> !keyword
//	not x       -> (not x)
//	x y / x and y -> (and x y)
//	x or y      -> (or x y)
//
// For example: (and a (or #b !unread)). A nil Expr yields "".
func String(e Expr) string {
	var sb strings.Builder
	writeExpr(&sb, e)
	return sb.String()
}

func writeExpr(sb *strings.Builder, e Expr) {
	switch v := e.(type) {
	case nil:
	case Term:
		if v.Phrase {
			sb.WriteString(strconv.Quote(v.Text))
		} else {
			sb.WriteString(v.Text)
		}
	case Tag:
		sb.WriteString("#" + v.Name)
	case Keyword:
		sb.WriteString("!" + v.Name)
	case Not:
		sb.WriteString("(not ")
		writeExpr(sb, v.X)
		sb.WriteString(")")
	case And:
		writeBin(sb, "and", v.L, v.R)
	case Or:
		writeBin(sb, "or", v.L, v.R)
	}
}

func writeBin(sb *strings.Builder, op string, l, r Expr) {
	sb.WriteString("(" + op + " ")
	writeExpr(sb, l)
	sb.WriteString(" ")
	writeExpr(sb, r)
	sb.WriteString(")")
}
