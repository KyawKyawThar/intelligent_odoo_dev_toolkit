package domain

import (
	"fmt"
	"strings"
	"unicode"
)

// Lexer tokenizes an Odoo domain string (Python syntax).
type Lexer struct {
	src    string
	pos    int
	tokens []Token
}

// Lex tokenizes the full input and returns all tokens (including EOF).
func Lex(src string) ([]Token, error) {
	l := &Lexer{src: src}
	if err := l.run(); err != nil {
		return nil, err
	}
	return l.tokens, nil
}

func (l *Lexer) run() error {
	for {
		l.skipWhitespace()
		if l.pos >= len(l.src) {
			l.emit(TokenEOF, "", l.pos)
			return nil
		}

		if err := l.lexNextToken(); err != nil {
			return err
		}
	}
}

func (l *Lexer) lexNextToken() error {
	ch := l.src[l.pos]
	switch ch {
	case '[', ']', '(', ')', ',':
		l.lexSimpleToken(ch)
	case '\'', '"':
		return l.lexString(ch)
	case '-', '+':
		return l.lexSignedNumber()
	default:
		return l.lexComplexToken(ch)
	}
	return nil
}

func (l *Lexer) lexSimpleToken(ch byte) {
	var tokenType TokenType
	switch ch {
	case '[':
		tokenType = TokenLBracket
	case ']':
		tokenType = TokenRBracket
	case '(':
		tokenType = TokenLParen
	case ')':
		tokenType = TokenRParen
	case ',':
		tokenType = TokenComma
	}
	l.emit(tokenType, string(ch), l.pos)
	l.pos++
}

func (l *Lexer) lexSignedNumber() error {
	if l.pos+1 < len(l.src) && (isDigit(l.src[l.pos+1]) || l.src[l.pos+1] == '.') {
		return l.lexNumber()
	}
	return l.errorf("unexpected character %q", l.src[l.pos])
}

func (l *Lexer) lexComplexToken(ch byte) error {
	if isDigit(ch) || ch == '.' {
		return l.lexNumber()
	}
	if isIdentStart(ch) {
		l.lexIdentOrKeyword()
		return nil
	}
	return l.errorf("unexpected character %q", ch)
}

func (l *Lexer) emit(typ TokenType, val string, pos int) {
	l.tokens = append(l.tokens, Token{Type: typ, Val: val, Pos: pos})
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.src) && unicode.IsSpace(rune(l.src[l.pos])) {
		l.pos++
	}
}

// lexString reads a single- or double-quoted string.
// It detects logical operators '&', '|', '!' and classifies them accordingly.
func (l *Lexer) lexString(quote byte) error {
	start := l.pos
	l.pos++ // skip opening quote

	var sb strings.Builder
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '\\' && l.pos+1 < len(l.src) {
			// escaped character
			l.pos++
			next := l.src[l.pos]
			switch next {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case '\\':
				sb.WriteByte('\\')
			case '\'':
				sb.WriteByte('\'')
			case '"':
				sb.WriteByte('"')
			default:
				sb.WriteByte('\\')
				sb.WriteByte(next)
			}
			l.pos++
			continue
		}
		if ch == quote {
			l.pos++ // skip closing quote
			val := sb.String()
			// Classify logical operators that appear as quoted strings
			switch val {
			case "&":
				l.emit(TokenAnd, val, start)
			case "|":
				l.emit(TokenOr, val, start)
			case "!":
				l.emit(TokenNot, val, start)
			default:
				l.emit(TokenString, val, start)
			}
			return nil
		}
		sb.WriteByte(ch)
		l.pos++
	}
	return l.errorf("unterminated string starting at position %d", start)
}

// lexNumber reads an integer or float literal.
func (l *Lexer) lexNumber() error {
	start := l.pos
	hasDot := false

	// optional sign
	if l.pos < len(l.src) && (l.src[l.pos] == '-' || l.src[l.pos] == '+') {
		l.pos++
	}

Loop:
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		switch {
		case ch == '.':
			if hasDot {
				break Loop
			}
			hasDot = true
			l.pos++
		case isDigit(ch):
			l.pos++
		default:
			break Loop
		}
	}

	val := l.src[start:l.pos]
	if val == "." || val == "-" || val == "+" || val == "-." || val == "+." {
		return l.errorf("invalid number %q at position %d", val, start)
	}

	if hasDot {
		l.emit(TokenFloat, val, start)
	} else {
		l.emit(TokenInt, val, start)
	}
	return nil
}

// lexIdentOrKeyword reads an identifier (possibly dotted) or keyword.
func (l *Lexer) lexIdentOrKeyword() {
	start := l.pos

	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if isIdentPart(ch) || ch == '.' {
			l.pos++
		} else {
			break
		}
	}

	val := l.src[start:l.pos]
	switch val {
	case "True":
		l.emit(TokenTrue, val, start)
	case "False":
		l.emit(TokenFalse, val, start)
	case "None":
		l.emit(TokenNone, val, start)
	default:
		l.emit(TokenRef, val, start)
	}
}

func (l *Lexer) errorf(format string, args ...any) error {
	return fmt.Errorf("lexer error at position %d: %s", l.pos, fmt.Sprintf(format, args...))
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch)
}
