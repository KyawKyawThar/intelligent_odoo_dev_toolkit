package domain

import (
	"fmt"
	"strconv"
)

// Parse converts an Odoo domain string (Python syntax) into a Go AST.
//
// Odoo domains use prefix (Polish) notation for logical operators:
//
//	[('field', '=', 1)]                          → single condition
//	[('a', '=', 1), ('b', '=', 2)]              → implicit AND of two conditions
//	['|', ('a', '=', 1), ('b', '=', 2)]         → OR of two conditions
//	['&', '|', ('a', '=', 1), ('b', '=', 2), ('c', '=', 3)]
//	                                              → AND(OR(a,b), c)
//	['!', ('a', '=', 1)]                         → NOT(a)
//
// Returns a single Node: either a Condition, a BoolOp, or nil for an empty domain [].
func Parse(src string) (Node, error) {
	tokens, err := Lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	return p.parseDomain()
}

type parser struct {
	tokens []Token
	pos    int
}

func (p *parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) advance() Token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func (p *parser) check(typ TokenType) bool {
	return p.peek().Type == typ
}

func (p *parser) expect(typ TokenType) error {
	tok := p.peek()
	if tok.Type != typ {
		return fmt.Errorf("expected token type %d at position %d, got %s", typ, tok.Pos, tok)
	}
	p.advance()
	return nil
}

func (p *parser) consumeIf(typ TokenType) bool {
	if p.check(typ) {
		p.advance()
		return true
	}
	return false
}

// parseDomain parses the top-level domain: '[' elements... ']'
func (p *parser) parseDomain() (Node, error) {
	if err := p.expect(TokenLBracket); err != nil {
		return nil, fmt.Errorf("domain must start with '[': %w", err)
	}

	// Collect all elements (operators and conditions) in the list.
	var elements []any // each is either LogicOp or Node
	for !p.check(TokenRBracket) && !p.check(TokenEOF) {
		if len(elements) > 0 {
			// Commas between elements are optional but typical
			p.consumeIf(TokenComma)
		}
		if p.check(TokenRBracket) {
			break
		}

		el, err := p.parseElement()
		if err != nil {
			return nil, err
		}
		elements = append(elements, el)
	}

	if err := p.expect(TokenRBracket); err != nil {
		return nil, fmt.Errorf("domain must end with ']': %w", err)
	}

	// Empty domain = no filter
	if len(elements) == 0 {
		return &MatchAllNode{}, nil
	}

	// Convert the flat prefix-notation list into a tree.
	return buildTree(elements)
}

// parseElement parses a single element inside the domain list:
// either a logical operator string ('&', '|', '!') or a condition tuple.
func (p *parser) parseElement() (any, error) {
	tok := p.peek()
	switch tok.Type {
	case TokenAnd:
		p.advance()
		return LogicAnd, nil
	case TokenOr:
		p.advance()
		return LogicOr, nil
	case TokenNot:
		p.advance()
		return LogicNot, nil
	case TokenLParen:
		return p.parseCondition()
	case TokenComma, TokenRBracket, TokenRParen, TokenEOF, TokenLBracket, TokenString, TokenInt, TokenFloat, TokenTrue, TokenFalse, TokenNone, TokenRef:
		return nil, fmt.Errorf("parse error at position %d: expected '&', '|', '!' or '(' but got %s", tok.Pos, tok)
	}
	return nil, fmt.Errorf("parse error at position %d: expected '&', '|', '!' or '(' but got %s", tok.Pos, tok)
}

// parseCondition parses a condition tuple: ('field', 'operator', value)
func (p *parser) parseCondition() (Node, error) {
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	// Field name (always a string)
	field, err := p.expectString()
	if err != nil {
		return nil, fmt.Errorf("condition field: %w", err)
	}
	p.consumeIf(TokenComma)

	// Operator (always a string)
	opStr, err := p.expectString()
	if err != nil {
		return nil, fmt.Errorf("condition operator: %w", err)
	}
	op, err := parseOperator(opStr)
	if err != nil {
		return nil, err
	}
	p.consumeIf(TokenComma)

	// Value (can be many types)
	val, err := p.parseValue()
	if err != nil {
		return nil, fmt.Errorf("condition value: %w", err)
	}

	if err := p.expect(TokenRParen); err != nil {
		return nil, fmt.Errorf("condition must end with ')': %w", err)
	}

	return &Condition{Field: field, Op: op, Value: val}, nil
}

// parseValue parses the right-hand side of a condition.
func (p *parser) parseValue() (Value, error) {
	tok := p.peek()
	switch tok.Type {
	case TokenString:
		p.advance()
		return StringValue{Val: tok.Val}, nil
	case TokenInt:
		p.advance()
		n, err := strconv.ParseInt(tok.Val, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid int %q: %w", tok.Val, err)
		}
		return IntValue{Val: n}, nil
	case TokenFloat:
		p.advance()
		f, err := strconv.ParseFloat(tok.Val, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float %q: %w", tok.Val, err)
		}
		return FloatValue{Val: f}, nil
	case TokenTrue:
		p.advance()
		return BoolValue{Val: true}, nil
	case TokenFalse:
		p.advance()
		return BoolValue{Val: false}, nil
	case TokenNone:
		p.advance()
		return NoneValue{}, nil
	case TokenRef:
		p.advance()
		return RefValue{Ref: tok.Val}, nil
	case TokenLBracket:
		return p.parseListValue()
	case TokenLParen:
		return p.parseTupleValue()
	case TokenRBracket, TokenRParen, TokenComma, TokenAnd, TokenOr, TokenNot, TokenEOF:
		return nil, fmt.Errorf("unexpected token %s at position %d for value", tok, tok.Pos)
	}
	return nil, fmt.Errorf("unexpected token %s at position %d for value", tok, tok.Pos)
}

// parseListValue parses [val, val, ...] as a Value (not a domain).
func (p *parser) parseListValue() (Value, error) {
	items, err := p.parseItems(TokenRBracket)
	if err != nil {
		return nil, fmt.Errorf("list value must end with ']': %w", err)
	}
	return ListValue{Items: items}, nil
}

// parseTupleValue parses (val, val, ...) as a tuple Value.
func (p *parser) parseTupleValue() (Value, error) {
	items, err := p.parseItems(TokenRParen)
	if err != nil {
		return nil, fmt.Errorf("tuple value must end with ')': %w", err)
	}
	return TupleValue{Items: items}, nil
}

// parseItems parses a sequence of values until a given closer token.
func (p *parser) parseItems(closer TokenType) ([]Value, error) {
	p.advance() // consume '[' or '('
	var items []Value
	for !p.check(closer) && !p.check(TokenEOF) {
		if len(items) > 0 {
			p.consumeIf(TokenComma)
		}
		if p.check(closer) {
			break
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	if err := p.expect(closer); err != nil {
		return nil, err
	}
	return items, nil
}

// expectString consumes the next token and returns its string value.
func (p *parser) expectString() (string, error) {
	tok := p.peek()
	if tok.Type != TokenString {
		return "", fmt.Errorf("expected string at position %d, got %s", tok.Pos, tok)
	}
	p.advance()
	return tok.Val, nil
}

var operatorMap = map[string]Operator{
	"=":         OpEqual,
	"!=":        OpNotEqual,
	"<":         OpLess,
	"<=":        OpLessEq,
	">":         OpGreater,
	">=":        OpGreaterEq,
	"in":        OpIn,
	"not in":    OpNotIn,
	"like":      OpLike,
	"not like":  OpNotLike,
	"ilike":     OpILike,
	"not ilike": OpNotILike,
	"=like":     OpEqLike,
	"=ilike":    OpEqILike,
	"child_of":  OpChildOf,
	"parent_of": OpParentOf,
}

func parseOperator(s string) (Operator, error) {
	if op, ok := operatorMap[s]; ok {
		return op, nil
	}
	return "", fmt.Errorf("unknown operator %q", s)
}

// buildTree converts a flat prefix-notation list into an AST.
//
// Odoo domain semantics:
//   - '&' consumes the next 2 operands (default between adjacent leaves)
//   - '|' consumes the next 2 operands
//   - '!' consumes the next 1 operand
//   - Adjacent conditions without explicit operators get implicit '&'
//
// The algorithm: first insert implicit '&' operators, then recursively
// consume operands in prefix order.
func buildTree(elements []any) (Node, error) {
	// Insert implicit '&' operators.
	// Count how many operands (conditions) vs operators we have.
	// Each '&'/'|' consumes 2 operands into 1 (net -1 operand).
	// Each '!' consumes 1 operand into 1 (net 0 operand change but is an operator).
	// For N operands we need N-1 binary operators total.
	expanded := insertImplicitAnd(elements)

	idx := 0
	node, err := consumeNode(expanded, &idx)
	if err != nil {
		return nil, err
	}

	if idx < len(expanded) {
		return nil, fmt.Errorf("unexpected trailing elements in domain at index %d", idx)
	}
	return node, nil
}

// insertImplicitAnd adds '&' operators between adjacent operands/sub-expressions
// that don't already have an explicit operator.
func insertImplicitAnd(elements []any) []any {
	if len(elements) <= 1 {
		return elements
	}

	// Count operands to determine how many implicit '&' we need.
	// Walk through and figure out where implicit ANDs should go.
	//
	// Strategy: simulate the prefix-notation stack.
	// An operator needs N operands. If after consuming all explicit operators
	// there are M resulting operands, we need M-1 implicit '&' prefixed.
	operands := 0
	operators := 0
	for _, el := range elements {
		switch v := el.(type) {
		case LogicOp:
			switch v {
			case LogicAnd, LogicOr:
				operators++
			case LogicNot:
				// NOT doesn't reduce operand count (1 in, 1 out)
				operators++
			}
		default:
			operands++
		}
	}

	// Each binary op (& or |) reduces operand count by 1.
	// Count only binary ops for implicit AND calculation.
	binaryOps := 0
	for _, el := range elements {
		if op, ok := el.(LogicOp); ok && (op == LogicAnd || op == LogicOr) {
			binaryOps++
		}
	}

	needed := operands - binaryOps - 1
	if needed <= 0 {
		return elements
	}

	// Prepend the needed '&' operators.
	result := make([]any, 0, len(elements)+needed)
	for i := 0; i < needed; i++ {
		result = append(result, LogicAnd)
	}
	_ = operators // used above in counting
	result = append(result, elements...)
	return result
}

// consumeNode recursively reads one complete operand from elements starting at *idx.
func consumeNode(elements []any, idx *int) (Node, error) {
	if *idx >= len(elements) {
		return nil, fmt.Errorf("unexpected end of domain: expected operand at index %d", *idx)
	}

	el := elements[*idx]
	*idx++

	switch v := el.(type) {
	case LogicOp:
		switch v {
		case LogicAnd, LogicOr:
			left, err := consumeNode(elements, idx)
			if err != nil {
				return nil, fmt.Errorf("operator %q left operand: %w", v, err)
			}
			right, err := consumeNode(elements, idx)
			if err != nil {
				return nil, fmt.Errorf("operator %q right operand: %w", v, err)
			}
			return &BoolOp{Op: v, Children: []Node{left, right}}, nil
		case LogicNot:
			child, err := consumeNode(elements, idx)
			if err != nil {
				return nil, fmt.Errorf("operator '!' operand: %w", err)
			}
			return &BoolOp{Op: LogicNot, Children: []Node{child}}, nil
		}
	case Node:
		return v, nil
	default:
		return nil, fmt.Errorf("unexpected element type %T at index %d", el, *idx-1)
	}
	return nil, fmt.Errorf("internal error: unhandled element %T in consumeNode", el)
}
