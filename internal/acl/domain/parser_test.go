package domain

import (
	"testing"
)

// --- Lexer tests ---

func TestLexSimpleCondition(t *testing.T) {
	tokens, err := Lex("[('name', '=', 'admin')]")
	if err != nil {
		t.Fatalf("Lex error: %v", err)
	}
	// [  (  'name'  ,  '='  ,  'admin'  )  ]  EOF
	expected := []TokenType{
		TokenLBracket, TokenLParen, TokenString, TokenComma, TokenString,
		TokenComma, TokenString, TokenRParen, TokenRBracket, TokenEOF,
	}
	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(expected))
	}
	for i, tok := range tokens {
		if tok.Type != expected[i] {
			t.Errorf("token[%d] type = %d, want %d (val=%q)", i, tok.Type, expected[i], tok.Val)
		}
	}
}

func TestLexLogicalOperators(t *testing.T) {
	tokens, err := Lex("['|', '&', '!']")
	if err != nil {
		t.Fatalf("Lex error: %v", err)
	}
	expected := []TokenType{
		TokenLBracket, TokenOr, TokenComma, TokenAnd, TokenComma, TokenNot,
		TokenRBracket, TokenEOF,
	}
	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d\ntokens: %v", len(tokens), len(expected), tokens)
	}
	for i, tok := range tokens {
		if tok.Type != expected[i] {
			t.Errorf("token[%d] type = %d, want %d (val=%q)", i, tok.Type, expected[i], tok.Val)
		}
	}
}

func TestLexNumbers(t *testing.T) {
	tokens, err := Lex("[42, -7, 3.14]")
	if err != nil {
		t.Fatalf("Lex error: %v", err)
	}
	// [ 42 , -7 , 3.14 ] EOF
	checks := []struct {
		typ TokenType
		val string
	}{
		{TokenLBracket, "["},
		{TokenInt, "42"},
		{TokenComma, ","},
		{TokenInt, "-7"},
		{TokenComma, ","},
		{TokenFloat, "3.14"},
		{TokenRBracket, "]"},
		{TokenEOF, ""},
	}
	if len(tokens) != len(checks) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(checks))
	}
	for i, c := range checks {
		if tokens[i].Type != c.typ || tokens[i].Val != c.val {
			t.Errorf("token[%d] = (%d, %q), want (%d, %q)", i, tokens[i].Type, tokens[i].Val, c.typ, c.val)
		}
	}
}

func TestLexKeywords(t *testing.T) {
	tokens, err := Lex("[True, False, None]")
	if err != nil {
		t.Fatalf("Lex error: %v", err)
	}
	expected := []TokenType{
		TokenLBracket, TokenTrue, TokenComma, TokenFalse, TokenComma,
		TokenNone, TokenRBracket, TokenEOF,
	}
	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(expected))
	}
	for i, tok := range tokens {
		if tok.Type != expected[i] {
			t.Errorf("token[%d] type = %d, want %d", i, tok.Type, expected[i])
		}
	}
}

func TestLexRef(t *testing.T) {
	tokens, err := Lex("[user.id, company_id]")
	if err != nil {
		t.Fatalf("Lex error: %v", err)
	}
	// [ user.id , company_id ] EOF
	if tokens[1].Type != TokenRef || tokens[1].Val != "user.id" {
		t.Errorf("expected ref 'user.id', got %v", tokens[1])
	}
	if tokens[3].Type != TokenRef || tokens[3].Val != "company_id" {
		t.Errorf("expected ref 'company_id', got %v", tokens[3])
	}
}

func TestLexEscapedString(t *testing.T) {
	tokens, err := Lex(`['hello\'world']`)
	if err != nil {
		t.Fatalf("Lex error: %v", err)
	}
	if tokens[1].Type != TokenString || tokens[1].Val != "hello'world" {
		t.Errorf("expected escaped string, got %v", tokens[1])
	}
}

func TestLexUnterminatedString(t *testing.T) {
	_, err := Lex("['unterminated")
	if err == nil {
		t.Fatal("expected error for unterminated string")
	}
}

// --- Parser tests ---

func TestParseEmptyDomain(t *testing.T) {
	node, err := Parse("[]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if _, ok := node.(*MatchAllNode); !ok {
		t.Errorf("expected *MatchAllNode for empty domain, got %T", node)
	}
}

func TestParseSingleCondition(t *testing.T) {
	node, err := Parse("[('name', '=', 'admin')]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond, ok := node.(*Condition)
	if !ok {
		t.Fatalf("expected *Condition, got %T", node)
	}
	if cond.Field != "name" {
		t.Errorf("field = %q, want %q", cond.Field, "name")
	}
	if cond.Op != OpEqual {
		t.Errorf("op = %q, want %q", cond.Op, OpEqual)
	}
	sv, ok := cond.Value.(StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", cond.Value)
	}
	if sv.Val != "admin" {
		t.Errorf("value = %q, want %q", sv.Val, "admin")
	}
}

func TestParseImplicitAnd(t *testing.T) {
	// Two conditions without explicit operator → implicit AND
	node, err := Parse("[('a', '=', 1), ('b', '!=', 2)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	bop, ok := node.(*BoolOp)
	if !ok {
		t.Fatalf("expected *BoolOp, got %T", node)
	}
	if bop.Op != LogicAnd {
		t.Errorf("op = %q, want %q", bop.Op, LogicAnd)
	}
	if len(bop.Children) != 2 {
		t.Fatalf("children count = %d, want 2", len(bop.Children))
	}
	left := bop.Children[0].(*Condition)
	right := bop.Children[1].(*Condition)
	if left.Field != "a" || left.Op != OpEqual {
		t.Errorf("left condition = %v", left)
	}
	if right.Field != "b" || right.Op != OpNotEqual {
		t.Errorf("right condition = %v", right)
	}
}

func TestParseThreeImplicitAnd(t *testing.T) {
	// Three conditions → AND(AND(a,b), c)
	node, err := Parse("[('a', '=', 1), ('b', '=', 2), ('c', '=', 3)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	bop, ok := node.(*BoolOp)
	if !ok {
		t.Fatalf("expected *BoolOp, got %T", node)
	}
	if bop.Op != LogicAnd {
		t.Errorf("top op = %q, want AND", bop.Op)
	}
	// Left child should also be AND
	leftBop, ok := bop.Children[0].(*BoolOp)
	if !ok {
		t.Fatalf("left child type = %T, want *BoolOp", bop.Children[0])
	}
	if leftBop.Op != LogicAnd {
		t.Errorf("left op = %q, want AND", leftBop.Op)
	}
	// Right child should be condition c
	rightCond := bop.Children[1].(*Condition)
	if rightCond.Field != "c" {
		t.Errorf("right field = %q, want 'c'", rightCond.Field)
	}
}

func TestParseExplicitOr(t *testing.T) {
	node, err := Parse("['|', ('a', '=', 1), ('b', '=', 2)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	bop, ok := node.(*BoolOp)
	if !ok {
		t.Fatalf("expected *BoolOp, got %T", node)
	}
	if bop.Op != LogicOr {
		t.Errorf("op = %q, want OR", bop.Op)
	}
	if len(bop.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(bop.Children))
	}
}

func TestParseNot(t *testing.T) {
	node, err := Parse("['!', ('active', '=', False)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	bop, ok := node.(*BoolOp)
	if !ok {
		t.Fatalf("expected *BoolOp, got %T", node)
	}
	if bop.Op != LogicNot {
		t.Errorf("op = %q, want NOT", bop.Op)
	}
	if len(bop.Children) != 1 {
		t.Fatalf("children = %d, want 1", len(bop.Children))
	}
	cond := bop.Children[0].(*Condition)
	if cond.Field != "active" {
		t.Errorf("field = %q, want 'active'", cond.Field)
	}
	bv := cond.Value.(BoolValue)
	if bv.Val != false {
		t.Errorf("value = %v, want false", bv.Val)
	}
}

func TestParseNestedOrAnd(t *testing.T) {
	// ['&', '|', ('a', '=', 1), ('b', '=', 2), ('c', '=', 3)]
	// → AND(OR(a, b), c)
	node, err := Parse("['&', '|', ('a', '=', 1), ('b', '=', 2), ('c', '=', 3)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	and, ok := node.(*BoolOp)
	if !ok {
		t.Fatalf("expected *BoolOp, got %T", node)
	}
	if and.Op != LogicAnd {
		t.Errorf("top op = %q, want AND", and.Op)
	}

	or, ok := and.Children[0].(*BoolOp)
	if !ok {
		t.Fatalf("left child type = %T, want *BoolOp", and.Children[0])
	}
	if or.Op != LogicOr {
		t.Errorf("left op = %q, want OR", or.Op)
	}

	c := and.Children[1].(*Condition)
	if c.Field != "c" {
		t.Errorf("right field = %q, want 'c'", c.Field)
	}
}

func TestParseInWithList(t *testing.T) {
	node, err := Parse("[('state', 'in', ['draft', 'confirmed'])]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	if cond.Op != OpIn {
		t.Errorf("op = %q, want 'in'", cond.Op)
	}
	lv, ok := cond.Value.(ListValue)
	if !ok {
		t.Fatalf("value type = %T, want ListValue", cond.Value)
	}
	if len(lv.Items) != 2 {
		t.Fatalf("list items = %d, want 2", len(lv.Items))
	}
	if lv.Items[0].(StringValue).Val != "draft" {
		t.Errorf("list[0] = %v, want 'draft'", lv.Items[0])
	}
}

func TestParseWithNone(t *testing.T) {
	node, err := Parse("[('parent_id', '=', None)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	if _, ok := cond.Value.(NoneValue); !ok {
		t.Errorf("value type = %T, want NoneValue", cond.Value)
	}
}

func TestParseWithRef(t *testing.T) {
	node, err := Parse("[('user_id', '=', user.id)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	rv, ok := cond.Value.(RefValue)
	if !ok {
		t.Fatalf("value type = %T, want RefValue", cond.Value)
	}
	if rv.Ref != "user.id" {
		t.Errorf("ref = %q, want 'user.id'", rv.Ref)
	}
}

func TestParseWithTrue(t *testing.T) {
	node, err := Parse("[('active', '=', True)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	bv, ok := cond.Value.(BoolValue)
	if !ok {
		t.Fatalf("value type = %T, want BoolValue", cond.Value)
	}
	if bv.Val != true {
		t.Errorf("value = %v, want true", bv.Val)
	}
}

func TestParseWithIntValue(t *testing.T) {
	node, err := Parse("[('id', '>', 100)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	if cond.Op != OpGreater {
		t.Errorf("op = %q, want '>'", cond.Op)
	}
	iv := cond.Value.(IntValue)
	if iv.Val != 100 {
		t.Errorf("value = %d, want 100", iv.Val)
	}
}

func TestParseCompanyRule(t *testing.T) {
	// Real Odoo domain: company_id is False OR matches current company
	src := "['|', ('company_id', '=', False), ('company_id', 'in', company_ids)]"
	node, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	bop := node.(*BoolOp)
	if bop.Op != LogicOr {
		t.Errorf("op = %q, want OR", bop.Op)
	}
	left := bop.Children[0].(*Condition)
	if left.Field != "company_id" {
		t.Errorf("left field = %q", left.Field)
	}
	lv := left.Value.(BoolValue)
	if lv.Val != false {
		t.Errorf("left value = %v, want false", lv.Val)
	}
	right := bop.Children[1].(*Condition)
	rv := right.Value.(RefValue)
	if rv.Ref != "company_ids" {
		t.Errorf("right ref = %q, want 'company_ids'", rv.Ref)
	}
}

func TestParseComplexMultiRule(t *testing.T) {
	// ['|', '|', ('user_id', '=', user.id), ('user_id', '=', False), ('message_follower_ids', 'in', [user.partner_id.id])]
	src := "['|', '|', ('user_id', '=', user.id), ('user_id', '=', False), ('message_follower_ids', 'in', [user.partner_id.id])]"
	node, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// Should be OR(OR(user_id=user.id, user_id=False), message_follower_ids in [...])
	outer := node.(*BoolOp)
	if outer.Op != LogicOr {
		t.Errorf("outer op = %q, want OR", outer.Op)
	}
	inner := outer.Children[0].(*BoolOp)
	if inner.Op != LogicOr {
		t.Errorf("inner op = %q, want OR", inner.Op)
	}
	follower := outer.Children[1].(*Condition)
	if follower.Field != "message_follower_ids" {
		t.Errorf("follower field = %q", follower.Field)
	}
	lv := follower.Value.(ListValue)
	if len(lv.Items) != 1 {
		t.Fatalf("list items = %d, want 1", len(lv.Items))
	}
	ref := lv.Items[0].(RefValue)
	if ref.Ref != "user.partner_id.id" {
		t.Errorf("ref = %q, want 'user.partner_id.id'", ref.Ref)
	}
}

func TestParseChildOf(t *testing.T) {
	node, err := Parse("[('parent_id', 'child_of', 5)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	if cond.Op != OpChildOf {
		t.Errorf("op = %q, want 'child_of'", cond.Op)
	}
}

func TestParseILike(t *testing.T) {
	node, err := Parse("[('name', 'ilike', 'test%')]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	if cond.Op != OpILike {
		t.Errorf("op = %q, want 'ilike'", cond.Op)
	}
}

func TestParseInWithIntList(t *testing.T) {
	node, err := Parse("[('id', 'in', [1, 2, 3])]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	lv := cond.Value.(ListValue)
	if len(lv.Items) != 3 {
		t.Fatalf("list items = %d, want 3", len(lv.Items))
	}
	for i, expected := range []int64{1, 2, 3} {
		iv := lv.Items[i].(IntValue)
		if iv.Val != expected {
			t.Errorf("list[%d] = %d, want %d", i, iv.Val, expected)
		}
	}
}

func TestParseNotIn(t *testing.T) {
	node, err := Parse("[('state', 'not in', ['cancel', 'done'])]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	if cond.Op != OpNotIn {
		t.Errorf("op = %q, want 'not in'", cond.Op)
	}
}

func TestParseDoubleQuotedStrings(t *testing.T) {
	node, err := Parse(`[("name", "=", "admin")]`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	if cond.Field != "name" {
		t.Errorf("field = %q, want 'name'", cond.Field)
	}
	sv := cond.Value.(StringValue)
	if sv.Val != "admin" {
		t.Errorf("value = %q, want 'admin'", sv.Val)
	}
}

func TestParseNegativeInt(t *testing.T) {
	node, err := Parse("[('sequence', '>', -1)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	iv := cond.Value.(IntValue)
	if iv.Val != -1 {
		t.Errorf("value = %d, want -1", iv.Val)
	}
}

func TestParseFloat(t *testing.T) {
	node, err := Parse("[('amount', '>=', 99.99)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cond := node.(*Condition)
	fv := cond.Value.(FloatValue)
	if fv.Val != 99.99 {
		t.Errorf("value = %f, want 99.99", fv.Val)
	}
}

func TestParseUnknownOperator(t *testing.T) {
	_, err := Parse("[('x', 'INVALID', 1)]")
	if err == nil {
		t.Fatal("expected error for unknown operator")
	}
}

func TestParseInvalidSyntax(t *testing.T) {
	cases := []string{
		"",         // no brackets
		"(",        // not a domain
		"[('a',)]", // incomplete condition
		"[('a')]",  // missing op and value
	}
	for _, src := range cases {
		_, err := Parse(src)
		if err == nil {
			t.Errorf("expected error for %q", src)
		}
	}
}

func TestParseNotOrCombo(t *testing.T) {
	// ['!', '|', ('a', '=', 1), ('b', '=', 2)]
	// → NOT(OR(a, b))
	node, err := Parse("['!', '|', ('a', '=', 1), ('b', '=', 2)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	not := node.(*BoolOp)
	if not.Op != LogicNot {
		t.Errorf("top op = %q, want NOT", not.Op)
	}
	or := not.Children[0].(*BoolOp)
	if or.Op != LogicOr {
		t.Errorf("inner op = %q, want OR", or.Op)
	}
}

func TestParseFourConditionsImplicitAnd(t *testing.T) {
	// 4 conditions → AND(AND(AND(a,b), c), d)
	node, err := Parse("[('a','=',1),('b','=',2),('c','=',3),('d','=',4)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// Just verify it's a valid tree with the right depth
	bop := node.(*BoolOp)
	if bop.Op != LogicAnd {
		t.Errorf("top op = %q, want AND", bop.Op)
	}
	// Right-most child should be condition 'd'
	right := bop.Children[1].(*Condition)
	if right.Field != "d" {
		t.Errorf("rightmost field = %q, want 'd'", right.Field)
	}
}

func TestParseOrWithImplicitAnd(t *testing.T) {
	// ['|', ('a', '=', 1), ('b', '=', 2), ('c', '=', 3)]
	// = implicit & prepended: ['&', '|', ('a', '=', 1), ('b', '=', 2), ('c', '=', 3)]
	// → AND(OR(a, b), c)
	node, err := Parse("['|', ('a', '=', 1), ('b', '=', 2), ('c', '=', 3)]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	and := node.(*BoolOp)
	if and.Op != LogicAnd {
		t.Errorf("top op = %q, want AND", and.Op)
	}
	or := and.Children[0].(*BoolOp)
	if or.Op != LogicOr {
		t.Errorf("inner op = %q, want OR", or.Op)
	}
	c := and.Children[1].(*Condition)
	if c.Field != "c" {
		t.Errorf("right field = %q, want 'c'", c.Field)
	}
}
