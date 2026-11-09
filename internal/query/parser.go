package query

import "strings"

// Parse parses a search query. Empty or whitespace-only input (or input that
// lexes to nothing, such as "#") returns nil, nil. Malformed input returns a
// *SyntaxError.
func Parse(s string) (Expr, error) {
	p := &parser{toks: lex(s)}
	if len(p.toks) == 1 {
		return nil, nil
	}
	e, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.cur().kind != tokEOF {
		return nil, p.unexpected()
	}
	return e, nil
}

type parser struct {
	toks []token
	i    int
}

func (p *parser) cur() token { return p.toks[p.i] }

func (p *parser) advance() {
	if p.i < len(p.toks)-1 {
		p.i++
	}
}

func (p *parser) unexpected() error {
	return &SyntaxError{Msg: "unexpected " + describe(p.cur()), Pos: p.cur().pos}
}

func describe(t token) string {
	switch t.kind {
	case tokEOF:
		return "end of query"
	case tokTag:
		return "'#" + t.value + "'"
	case tokKeyword:
		return "'!" + t.value + "'"
	case tokPhrase:
		return "quoted phrase"
	}
	return "'" + t.value + "'"
}

func (p *parser) parseOr() (Expr, error) {
	l, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tokOr {
		p.advance()
		r, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l = Or{l, r}
	}
	return l, nil
}

func (p *parser) parseAnd() (Expr, error) {
	l, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		switch p.cur().kind {
		case tokAnd:
			p.advance()
		case tokTerm, tokPhrase, tokTag, tokKeyword, tokLParen, tokNot:
			// implicit AND
		default:
			return l, nil
		}
		r, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		l = And{l, r}
	}
}

func (p *parser) parseNot() (Expr, error) {
	if p.cur().kind == tokNot {
		p.advance()
		x, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return Not{x}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Expr, error) {
	t := p.cur()
	switch t.kind {
	case tokTerm:
		p.advance()
		return Term{Text: t.value}, nil
	case tokPhrase:
		p.advance()
		return Term{Text: t.value, Phrase: true}, nil
	case tokTag:
		p.advance()
		return Tag{strings.ToLower(t.value)}, nil
	case tokKeyword:
		p.advance()
		return Keyword{strings.ToLower(t.value)}, nil
	case tokLParen:
		p.advance()
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.cur().kind != tokRParen {
			return nil, &SyntaxError{Msg: "expected ')' but found " + describe(p.cur()), Pos: p.cur().pos}
		}
		p.advance()
		return e, nil
	}
	return nil, p.unexpected()
}
