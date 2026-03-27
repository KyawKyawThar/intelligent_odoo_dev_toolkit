// Package domain provides the Abstract Syntax Tree (AST) for Odoo domains,
// along with a parser to convert domain strings into this AST.
package domain

import "fmt"

// Operator is a comparison operator in an Odoo domain leaf.
type Operator string

const (
	OpEqual     Operator = "="
	OpNotEqual  Operator = "!="
	OpLess      Operator = "<"
	OpLessEq    Operator = "<="
	OpGreater   Operator = ">"
	OpGreaterEq Operator = ">="
	OpIn        Operator = "in"
	OpNotIn     Operator = "not in"
	OpLike      Operator = "like"
	OpNotLike   Operator = "not like"
	OpILike     Operator = "ilike"
	OpNotILike  Operator = "not ilike"
	OpEqLike    Operator = "=like"
	OpEqILike   Operator = "=ilike"
	OpChildOf   Operator = "child_of"
	OpParentOf  Operator = "parent_of"
)

// LogicOp is a boolean logic operator (&, |, !).
type LogicOp string

const (
	LogicAnd LogicOp = "&"
	LogicOr  LogicOp = "|"
	LogicNot LogicOp = "!"
)

// Node is the interface implemented by all AST nodes.
type Node interface {
	nodeType() string
}

// Condition is a leaf node: ('field_name', 'operator', value).
type Condition struct {
	Field string   // e.g. "user_id", "company_id.partner_id"
	Op    Operator // e.g. OpEqual, OpIn
	Value Value    // right-hand side
}

func (c *Condition) nodeType() string { return "condition" }

func (c *Condition) String() string {
	return fmt.Sprintf("(%q, %q, %v)", c.Field, c.Op, c.Value)
}

// BoolOp is a branch node combining children with a logical operator.
// AND/OR have exactly 2 children; NOT has exactly 1.
type BoolOp struct {
	Op       LogicOp
	Children []Node
}

func (b *BoolOp) nodeType() string { return "bool_op" }

func (b *BoolOp) String() string {
	return fmt.Sprintf("%s(%v)", b.Op, b.Children)
}

// MatchAllNode represents an empty domain that matches all records.
type MatchAllNode struct{}

func (n *MatchAllNode) nodeType() string { return "match_all" }

// Value is the interface for right-hand side values in a Condition.
type Value interface {
	valueType() string
}

// StringValue is a string literal.
type StringValue struct{ Val string }

func (v StringValue) valueType() string { return "string" }
func (v StringValue) String() string    { return fmt.Sprintf("%q", v.Val) }

// IntValue is an integer literal.
type IntValue struct{ Val int64 }

func (v IntValue) valueType() string { return "int" }
func (v IntValue) String() string    { return fmt.Sprintf("%d", v.Val) }

// FloatValue is a floating-point literal.
type FloatValue struct{ Val float64 }

func (v FloatValue) valueType() string { return "float" }
func (v FloatValue) String() string    { return fmt.Sprintf("%g", v.Val) }

// BoolValue is True or False.
type BoolValue struct{ Val bool }

func (v BoolValue) valueType() string { return "bool" }
func (v BoolValue) String() string    { return fmt.Sprintf("%t", v.Val) }

// NoneValue represents Python's None (null).
type NoneValue struct{}

func (v NoneValue) valueType() string { return "none" }
func (v NoneValue) String() string    { return "None" }

// ListValue is a list of values, e.g. [1, 2, 3].
type ListValue struct{ Items []Value }

func (v ListValue) valueType() string { return "list" }
func (v ListValue) String() string    { return fmt.Sprintf("%v", v.Items) }

// TupleValue is a tuple of values, e.g. (1, 2).
type TupleValue struct{ Items []Value }

func (v TupleValue) valueType() string { return "tuple" }
func (v TupleValue) String() string    { return fmt.Sprintf("(%v)", v.Items) }

// RefValue is a dotted reference like user.id or company_id.
// These appear in Odoo domains as unquoted Python expressions.
type RefValue struct{ Ref string }

func (v RefValue) valueType() string { return "ref" }
func (v RefValue) String() string    { return v.Ref }
