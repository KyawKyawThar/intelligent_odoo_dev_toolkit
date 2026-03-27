package domain

import "fmt"

// TokenType classifies lexer tokens.
type TokenType int

const (
	TokenLBracket TokenType = iota // [
	TokenRBracket                  // ]
	TokenLParen                    // (
	TokenRParen                    // )
	TokenComma                     // ,
	TokenString                    // 'hello' or "hello"
	TokenInt                       // 42
	TokenFloat                     // 3.14
	TokenTrue                      // True
	TokenFalse                     // False
	TokenNone                      // None
	TokenAnd                       // '&'
	TokenOr                        // '|'
	TokenNot                       // '!'
	TokenRef                       // dotted reference like user.id, company_id
	TokenEOF                       // end of input
)

// Token is a single lexer token with its position in the source.
type Token struct {
	Type TokenType
	Val  string // raw text of the token
	Pos  int    // byte offset in source
}

func (t Token) String() string {
	return fmt.Sprintf("Token(%d, %q, pos=%d)", t.Type, t.Val, t.Pos)
}
