package acl

import (
	"fmt"
	"math"
	"strings"

	"Intelligent_Dev_ToolKit_Odoo/internal/acl/domain"
)

const stageDomain = "domain"

// DomainEvaluator is Stage 5 of the ACL pipeline.
// It parses each applicable rule's domain string, resolves references
// (e.g. user.id), and evaluates every condition against the actual record data.
//
// Odoo domain evaluation semantics:
//   - Global rules' domains are ANDed: ALL must pass.
//   - Group rules' domains are ORed: ANY one passing is enough.
//   - Final result: global_pass AND group_pass.
//   - An empty domain "[]" or "(1,'=',1)" always passes (no filter).
//   - child_of / parent_of require hierarchy traversal and cannot be fully
//     evaluated here — they are marked as "unevaluable" with a warning.
type DomainEvaluator struct{}

// NewDomainEvaluator creates a DomainEvaluator.
func NewDomainEvaluator() *DomainEvaluator {
	return &DomainEvaluator{}
}

// ConditionResult captures the evaluation of a single leaf condition.
type ConditionResult struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Expected any    `json:"expected"` // the resolved right-hand side
	Actual   any    `json:"actual"`   // the record's field value
	Pass     bool   `json:"pass"`
	Error    string `json:"error,omitempty"` // e.g. "field not in record data"
}

// RuleDomainResult captures the evaluation of one ir.rule's domain.
type RuleDomainResult struct {
	RuleName   string            `json:"rule_name"`
	Domain     string            `json:"domain"`
	Global     bool              `json:"global"`
	Pass       bool              `json:"pass"`
	ParseError string            `json:"parse_error,omitempty"`
	Conditions []ConditionResult `json:"conditions"`
}

// DomainEvalDetail is the stage detail.
type DomainEvalDetail struct {
	Model       string             `json:"model"`
	Operation   Operation          `json:"operation"`
	GlobalPass  bool               `json:"global_pass"`
	GroupPass   bool               `json:"group_pass"`
	RuleResults []RuleDomainResult `json:"rule_results"`
}

// Evaluate evaluates all applicable rules' domains against the record data.
//
// Parameters:
//   - ruleDetail: the RecordRuleDetail from Stage 4 (record rule finder)
//   - record: the field values of the actual record
//   - evalCtx: runtime context for resolving user.id, company_ids, etc.
func (e *DomainEvaluator) Evaluate(
	ruleDetail *RecordRuleDetail,
	record RecordData,
	evalCtx *EvalContext,
) (*StageResult, error) {
	ruleResults, globalPass, groupPass := e.evaluateRules(ruleDetail, record, evalCtx)

	detail := &DomainEvalDetail{
		Model:       ruleDetail.Model,
		Operation:   ruleDetail.Operation,
		GlobalPass:  globalPass,
		GroupPass:   groupPass,
		RuleResults: ruleResults,
	}

	if globalPass && groupPass {
		return e.createSuccessResult(ruleDetail, detail, globalPass, groupPass), nil
	}

	return e.createFailureResult(ruleDetail, detail, globalPass, groupPass, ruleResults), nil
}

func (e *DomainEvaluator) evaluateRules(
	ruleDetail *RecordRuleDetail,
	record RecordData,
	evalCtx *EvalContext,
) ([]RuleDomainResult, bool, bool) {
	var ruleResults []RuleDomainResult
	globalPass := e.evaluateGlobalRules(ruleDetail.GlobalRules, record, evalCtx, &ruleResults)
	groupPass := e.evaluateGroupRules(ruleDetail.GroupRules, record, evalCtx, &ruleResults)

	return ruleResults, globalPass, groupPass
}

func (e *DomainEvaluator) evaluateGlobalRules(
	rules []RecordRuleMatch,
	record RecordData,
	evalCtx *EvalContext,
	ruleResults *[]RuleDomainResult,
) bool {
	pass := true
	for _, rule := range rules {
		if !rule.Applies {
			continue
		}
		rr := e.evalRule(rule.Name, rule.Domain, true, record, evalCtx)
		*ruleResults = append(*ruleResults, rr)
		if !rr.Pass {
			pass = false
		}
	}
	return pass
}

func (e *DomainEvaluator) evaluateGroupRules(
	rules []RecordRuleMatch,
	record RecordData,
	evalCtx *EvalContext,
	ruleResults *[]RuleDomainResult,
) bool {
	pass := true
	hasApplicableRule := false
	for _, rule := range rules {
		if !rule.Applies {
			*ruleResults = append(*ruleResults, RuleDomainResult{
				RuleName: rule.Name, Domain: rule.Domain, Global: false, Pass: false,
				Conditions: []ConditionResult{{Error: "rule does not apply to user's groups"}},
			})
			continue
		}
		hasApplicableRule = true
		rr := e.evalRule(rule.Name, rule.Domain, false, record, evalCtx)
		*ruleResults = append(*ruleResults, rr)
	}

	if hasApplicableRule {
		pass = false
		for _, rr := range *ruleResults {
			if !rr.Global && rr.Pass {
				pass = true
				break
			}
		}
	}
	return pass
}

func (e *DomainEvaluator) createSuccessResult(
	ruleDetail *RecordRuleDetail,
	detail *DomainEvalDetail,
	globalPass, groupPass bool,
) *StageResult {
	return &StageResult{
		Stage:   stageDomain,
		Verdict: VerdictOK,
		Reason: fmt.Sprintf(
			"model %q %s: all domain conditions satisfied (global: %s, group: %s)",
			ruleDetail.Model, ruleDetail.Operation,
			passStr(globalPass), passStr(groupPass),
		),
		Detail: detail,
	}
}

func (e *DomainEvaluator) createFailureResult(
	ruleDetail *RecordRuleDetail,
	detail *DomainEvalDetail,
	globalPass, groupPass bool,
	ruleResults []RuleDomainResult,
) *StageResult {
	var parts []string
	if !globalPass {
		failed := 0
		for _, rr := range ruleResults {
			if rr.Global && !rr.Pass {
				failed++
			}
		}
		parts = append(parts, fmt.Sprintf("%d global rule(s) failed", failed))
	}
	if !groupPass {
		parts = append(parts, "no group rule domain passed")
	}

	return &StageResult{
		Stage:   stageDomain,
		Verdict: VerdictDenied,
		Reason: fmt.Sprintf(
			"model %q %s: DENIED — %s",
			ruleDetail.Model, ruleDetail.Operation, strings.Join(parts, "; "),
		),
		Detail: detail,
	}
}

// evalRule parses a single rule's domain and evaluates it against the record.
func (e *DomainEvaluator) evalRule(
	name, domainStr string,
	global bool,
	record RecordData,
	evalCtx *EvalContext,
) RuleDomainResult {
	result := RuleDomainResult{
		RuleName: name,
		Domain:   domainStr,
		Global:   global,
	}

	ast, err := domain.Parse(domainStr)
	if err != nil {
		result.ParseError = err.Error()
		result.Pass = false
		return result
	}

	// Empty domain (e.g. "[]") → always passes.
	if _, ok := ast.(*domain.MatchAllNode); ok {
		result.Pass = true
		return result
	}

	pass, conditions := e.evalNode(ast, record, evalCtx)
	result.Pass = pass
	result.Conditions = conditions
	return result
}

// evalNode recursively evaluates an AST node, returning pass/fail and
// the leaf conditions encountered.
func (e *DomainEvaluator) evalNode(
	node domain.Node,
	record RecordData,
	evalCtx *EvalContext,
) (bool, []ConditionResult) {
	switch n := node.(type) {
	case *domain.MatchAllNode:
		return true, nil
	case *domain.Condition:
		cr := e.evalCondition(n, record, evalCtx)
		return cr.Pass, []ConditionResult{cr}

	case *domain.BoolOp:
		switch n.Op {
		case domain.LogicAnd:
			allPass := true
			var allConds []ConditionResult
			for _, child := range n.Children {
				pass, conds := e.evalNode(child, record, evalCtx)
				allConds = append(allConds, conds...)
				if !pass {
					allPass = false
				}
			}
			return allPass, allConds

		case domain.LogicOr:
			anyPass := false
			var allConds []ConditionResult
			for _, child := range n.Children {
				pass, conds := e.evalNode(child, record, evalCtx)
				allConds = append(allConds, conds...)
				if pass {
					anyPass = true
				}
			}
			return anyPass, allConds

		case domain.LogicNot:
			if len(n.Children) != 1 {
				return false, []ConditionResult{{Error: "NOT node must have exactly 1 child"}}
			}
			pass, conds := e.evalNode(n.Children[0], record, evalCtx)
			return !pass, conds
		}
	}

	return false, []ConditionResult{{Error: fmt.Sprintf("unknown node type: %T", node)}}
}

// evalCondition evaluates a single leaf condition against the record.
func (e *DomainEvaluator) evalCondition(
	cond *domain.Condition,
	record RecordData,
	evalCtx *EvalContext,
) ConditionResult {
	cr := ConditionResult{
		Field:    cond.Field,
		Operator: string(cond.Op),
	}

	// Resolve the expected value (right-hand side).
	expected, err := resolveValue(cond.Value, evalCtx)
	if err != nil {
		cr.Error = err.Error()
		cr.Pass = false
		return cr
	}
	cr.Expected = expected

	// Get the actual value from the record.
	actual, fieldExists := record[cond.Field]
	if !fieldExists {
		cr.Error = fmt.Sprintf("field %q not found in record data", cond.Field)
		cr.Pass = false
		return cr
	}
	cr.Actual = actual

	// Compare.
	cr.Pass = compare(cond.Op, actual, expected)
	return cr
}

// resolveValue converts a domain AST Value into a Go value, resolving
// references like user.id from the EvalContext.
func resolveValue(v domain.Value, ctx *EvalContext) (any, error) {
	switch val := v.(type) {
	case domain.StringValue:
		return val.Val, nil
	case domain.IntValue:
		return val.Val, nil
	case domain.FloatValue:
		return val.Val, nil
	case domain.BoolValue:
		return val.Val, nil
	case domain.NoneValue:
		return nil, nil //nolint:nilnil // NoneValue successfully resolves to a nil interface value.
	case domain.RefValue:
		return resolveRef(val.Ref, ctx)
	case domain.ListValue:
		items := make([]any, len(val.Items))
		for i, item := range val.Items {
			resolved, err := resolveValue(item, ctx)
			if err != nil {
				return nil, err
			}
			items[i] = resolved
		}
		return items, nil
	case domain.TupleValue:
		items := make([]any, len(val.Items))
		for i, item := range val.Items {
			resolved, err := resolveValue(item, ctx)
			if err != nil {
				return nil, err
			}
			items[i] = resolved
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unsupported value type: %T", v)
	}
}

// resolveRef resolves a dotted reference (e.g. "user.id", "company_ids") to
// a concrete value from the EvalContext.
func resolveRef(ref string, ctx *EvalContext) (any, error) {
	switch ref {
	case "user.id":
		return int64(ctx.UserID), nil
	case "user.company_id", "user.company_id.id":
		return int64(ctx.CompanyID), nil
	case "company_ids", "user.company_ids":
		result := make([]any, len(ctx.CompanyIDs))
		for i, id := range ctx.CompanyIDs {
			result[i] = int64(id)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unresolvable reference %q — requires runtime data from Odoo", ref)
	}
}

var operatorFuncs = map[domain.Operator]func(actual, expected any) bool{
	domain.OpEqual:     valuesEqual,
	domain.OpNotEqual:  func(a, e any) bool { return !valuesEqual(a, e) },
	domain.OpLess:      func(a, e any) bool { return numericCompare(a, e) < 0 },
	domain.OpLessEq:    func(a, e any) bool { return numericCompare(a, e) <= 0 },
	domain.OpGreater:   func(a, e any) bool { return numericCompare(a, e) > 0 },
	domain.OpGreaterEq: func(a, e any) bool { return numericCompare(a, e) >= 0 },
	domain.OpIn:        valueIn,
	domain.OpNotIn:     func(a, e any) bool { return !valueIn(a, e) },
	domain.OpLike:      func(a, e any) bool { return stringLike(a, e, true) },
	domain.OpNotLike:   func(a, e any) bool { return !stringLike(a, e, true) },
	domain.OpILike:     func(a, e any) bool { return stringLike(a, e, false) },
	domain.OpNotILike:  func(a, e any) bool { return !stringLike(a, e, false) },
	domain.OpEqLike:    func(a, e any) bool { return stringLike(a, e, true) },
	domain.OpEqILike:   func(a, e any) bool { return stringLike(a, e, false) },
	domain.OpChildOf:   func(a, e any) bool { return true }, // optimistic
	domain.OpParentOf:  func(a, e any) bool { return true }, // optimistic
}

// compare applies the Odoo domain operator to actual and expected values.
func compare(op domain.Operator, actual, expected any) bool {
	if fn, ok := operatorFuncs[op]; ok {
		return fn(actual, expected)
	}
	return false
}

// valuesEqual compares two values with type coercion for numeric types.
// Odoo's False is equivalent to nil/0/false depending on context.
func valuesEqual(a, b any) bool {
	// Handle nil / false / 0 equivalence (Odoo treats False as nil).
	if isNilOrFalse(a) && isNilOrFalse(b) {
		return true
	}
	if isNilOrFalse(a) != isNilOrFalse(b) {
		return false
	}

	// Numeric comparison with coercion.
	af, aOk := toFloat64(a)
	bf, bOk := toFloat64(b)
	if aOk && bOk {
		return af == bf
	}

	// String comparison.
	as, aStr := a.(string)
	bs, bStr := b.(string)
	if aStr && bStr {
		return as == bs
	}

	// Bool comparison.
	ab, aBool := a.(bool)
	bb, bBool := b.(bool)
	if aBool && bBool {
		return ab == bb
	}

	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// isNilOrFalse returns true if the value represents Odoo's "False" concept.
func isNilOrFalse(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case bool:
		return !val
	case int:
		return val == 0
	case int64:
		return val == 0
	case float64:
		return val == 0
	case string:
		return false // empty string is NOT considered False in Odoo domain comparison
	}
	return false
}

// toFloat64 coerces numeric types to float64.
func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case float64:
		return val, true
	case float32:
		return float64(val), true
	}
	return 0, false
}

// numericCompare returns -1, 0, or 1 comparing a and b numerically.
// Returns 0 if either value is non-numeric.
func numericCompare(a, b any) int {
	af, aOk := toFloat64(a)
	bf, bOk := toFloat64(b)
	if !aOk || !bOk {
		return 0
	}
	diff := af - bf
	if math.Abs(diff) < 1e-9 {
		return 0
	}
	if diff < 0 {
		return -1
	}
	return 1
}

// valueIn checks if actual is contained in expected (which should be a slice).
// Also handles the case where actual is a scalar and expected is a list.
func valueIn(actual, expected any) bool {
	list, ok := toSlice(expected)
	if !ok {
		// If expected is not a list, treat as single-element comparison.
		return valuesEqual(actual, expected)
	}
	for _, item := range list {
		if valuesEqual(actual, item) {
			return true
		}
	}
	return false
}

// toSlice converts a value to []any if it's a slice type.
func toSlice(v any) ([]any, bool) {
	switch val := v.(type) {
	case []any:
		return val, true
	case []int:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = item
		}
		return result, true
	case []int64:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = item
		}
		return result, true
	case []float64:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = item
		}
		return result, true
	case []string:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = item
		}
		return result, true
	}
	return nil, false
}

// stringLike implements SQL LIKE-style matching.
// Pattern uses % as wildcard. caseSensitive=false for ilike.
func stringLike(actual, expected any, caseSensitive bool) bool {
	aStr, ok := actual.(string)
	if !ok {
		return false
	}
	pattern, ok := expected.(string)
	if !ok {
		return false
	}

	if !caseSensitive {
		aStr = strings.ToLower(aStr)
		pattern = strings.ToLower(pattern)
	}

	// Odoo's like/ilike wraps the pattern with % by default if not present.
	// But here we evaluate the pattern as-is since the domain already includes %.
	return matchLike(aStr, pattern)
}

// matchLike implements SQL LIKE pattern matching with % and _ wildcards.
func matchLike(s, pattern string) bool {
	// Dynamic programming approach for LIKE matching.
	sLen := len(s)
	pLen := len(pattern)

	// dp[i][j] = true if s[:i] matches pattern[:j]
	dp := make([][]bool, sLen+1)
	for i := range dp {
		dp[i] = make([]bool, pLen+1)
	}
	dp[0][0] = true

	// Handle leading % in pattern.
	for j := 1; j <= pLen; j++ {
		if pattern[j-1] == '%' {
			dp[0][j] = dp[0][j-1]
		} else {
			break
		}
	}

	for i := 1; i <= sLen; i++ {
		for j := 1; j <= pLen; j++ {
			switch pattern[j-1] {
			case '%':
				dp[i][j] = dp[i-1][j] || dp[i][j-1]
			case '_':
				dp[i][j] = dp[i-1][j-1]
			default:
				dp[i][j] = dp[i-1][j-1] && s[i-1] == pattern[j-1]
			}
		}
	}
	return dp[sLen][pLen]
}

func passStr(pass bool) string {
	if pass {
		return "PASS"
	}
	return "FAIL"
}
