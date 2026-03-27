package acl

import "testing"

func defaultEvalCtx() *EvalContext {
	return &EvalContext{
		UserID:     42,
		CompanyID:  1,
		CompanyIDs: []int{1, 2},
	}
}

// ─── Basic condition evaluation ─────────────────────────────────────────────

func TestDomainEvalSimpleEqualPass(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "active_rule", Domain: "[('active','=',True)]", Global: true, Applies: true},
		},
	}
	record := RecordData{"active": true}

	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (reason: %s)", result.Verdict, result.Reason)
	}
}

func TestDomainEvalSimpleEqualFail(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "active_rule", Domain: "[('active','=',True)]", Global: true, Applies: true},
		},
	}
	record := RecordData{"active": false}

	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestDomainEvalNotEqual(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "state_rule", Domain: "[('state','!=','cancel')]", Global: true, Applies: true},
		},
	}
	record := RecordData{"state": "draft"}

	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
}

// ─── Numeric comparisons ────────────────────────────────────────────────────

func TestDomainEvalGreaterThan(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "amount_rule", Domain: "[('amount_total','>',100)]", Global: true, Applies: true},
		},
	}

	// Pass: 200 > 100
	record := RecordData{"amount_total": float64(200)}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	// Fail: 50 > 100
	record = RecordData{"amount_total": float64(50)}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestDomainEvalLessEqual(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "seq_rule", Domain: "[('sequence','<=',10)]", Global: true, Applies: true},
		},
	}
	record := RecordData{"sequence": float64(10)}

	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
}

// ─── User reference resolution ──────────────────────────────────────────────

func TestDomainEvalUserIdRef(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "owner_rule", Domain: "[('user_id','=',user.id)]", Global: true, Applies: true},
		},
	}

	// Pass: record user_id matches eval context user.id (42)
	record := RecordData{"user_id": float64(42)}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	// Fail: different user
	record = RecordData{"user_id": float64(99)}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestDomainEvalCompanyIdsRef(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "company_rule", Domain: "[('company_id','in',company_ids)]", Global: true, Applies: true},
		},
	}

	// Pass: company_id=1, company_ids=[1,2]
	record := RecordData{"company_id": float64(1)}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	record = RecordData{"company_id": float64(99)}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

// ─── In / Not In ────────────────────────────────────────────────────────────

func TestDomainEvalInList(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "state_rule", Domain: "[('state','in',['draft','confirmed'])]", Global: true, Applies: true},
		},
	}

	record := RecordData{"state": "draft"}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	record = RecordData{"state": "cancel"}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestDomainEvalNotIn(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "state_rule", Domain: "[('state','not in',['cancel','done'])]", Global: true, Applies: true},
		},
	}

	record := RecordData{"state": "draft"}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
}

// ─── Boolean logic (OR, AND, NOT) ───────────────────────────────────────────

func TestDomainEvalOrCondition(t *testing.T) {
	// ['|', ('user_id','=',user.id), ('user_id','=',False)]
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{
				Name:    "owner_or_unassigned",
				Domain:  "['|',('user_id','=',user.id),('user_id','=',False)]",
				Global:  true,
				Applies: true,
			},
		},
	}

	// Pass: user_id is False (nil) → second branch passes
	record := RecordData{"user_id": false}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (user_id=False matches second branch)", result.Verdict)
	}

	// Pass: user_id matches user.id
	record = RecordData{"user_id": float64(42)}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	// Fail: user_id is another user
	record = RecordData{"user_id": float64(99)}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestDomainEvalNotCondition(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{
				Name:    "not_canceled",
				Domain:  "['!',('state','=','cancel')]",
				Global:  true,
				Applies: true,
			},
		},
	}

	record := RecordData{"state": "draft"}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	record = RecordData{"state": "cancel"}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

// ─── Global AND Group rule combination ──────────────────────────────────────

func TestDomainEvalGlobalAndGroupRules(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "active_rule", Domain: "[('active','=',True)]", Global: true, Applies: true},
		},
		GroupRules: []RecordRuleMatch{
			{Name: "owner_rule", Domain: "[('user_id','=',user.id)]", Global: false, Applies: true},
		},
	}

	// Both pass
	record := RecordData{"active": true, "user_id": float64(42)}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	// Global passes, group fails → DENIED
	record = RecordData{"active": true, "user_id": float64(99)}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED (group rule failed)", result.Verdict)
	}

	// Global fails, group passes → DENIED
	record = RecordData{"active": false, "user_id": float64(42)}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED (global rule failed)", result.Verdict)
	}
}

func TestDomainEvalMultipleGlobalRulesANDed(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "active_rule", Domain: "[('active','=',True)]", Global: true, Applies: true},
			{Name: "company_rule", Domain: "[('company_id','=',1)]", Global: true, Applies: true},
		},
	}

	// Both pass
	record := RecordData{"active": true, "company_id": float64(1)}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	// One fails → DENIED (ANDed)
	record = RecordData{"active": true, "company_id": float64(99)}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestDomainEvalMultipleGroupRulesORed(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GroupRules: []RecordRuleMatch{
			{Name: "owner_rule", Domain: "[('user_id','=',user.id)]", Global: false, Applies: true},
			{Name: "public_rule", Domain: "[('is_public','=',True)]", Global: false, Applies: true},
		},
	}

	// First passes, second fails → OK (ORed)
	record := RecordData{"user_id": float64(42), "is_public": false}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (one group rule passes = enough)", result.Verdict)
	}

	// Both fail → DENIED
	record = RecordData{"user_id": float64(99), "is_public": false}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

// ─── Empty domain ───────────────────────────────────────────────────────────

func TestDomainEvalEmptyDomainPasses(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "empty_rule", Domain: "[]", Global: true, Applies: true},
		},
	}

	record := RecordData{}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (empty domain always passes)", result.Verdict)
	}
}

// ─── No rules → all pass ───────────────────────────────────────────────────

func TestDomainEvalNoRulesPasses(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
	}

	result, err := eval.Evaluate(detail, RecordData{}, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (no rules = pass)", result.Verdict)
	}
}

// ─── Parse error handling ───────────────────────────────────────────────────

func TestDomainEvalParseError(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "bad_rule", Domain: "NOT_VALID_DOMAIN", Global: true, Applies: true},
		},
	}

	result, err := eval.Evaluate(detail, RecordData{}, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED (parse error = fail)", result.Verdict)
	}
	evalDetail := result.Detail.(*DomainEvalDetail)
	if evalDetail.RuleResults[0].ParseError == "" {
		t.Error("expected ParseError to be set")
	}
}

// ─── Field not in record ────────────────────────────────────────────────────

func TestDomainEvalMissingField(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "field_rule", Domain: "[('nonexistent','=',True)]", Global: true, Applies: true},
		},
	}

	result, err := eval.Evaluate(detail, RecordData{}, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED (missing field)", result.Verdict)
	}
	evalDetail := result.Detail.(*DomainEvalDetail)
	if len(evalDetail.RuleResults[0].Conditions) == 0 {
		t.Fatal("expected conditions in result")
	}
	if evalDetail.RuleResults[0].Conditions[0].Error == "" {
		t.Error("expected error for missing field")
	}
}

// ─── None/False equivalence ─────────────────────────────────────────────────

func TestDomainEvalNoneFalseEquivalence(t *testing.T) {
	eval := NewDomainEvaluator()

	// ('parent_id', '=', False) should match nil
	detail := &RecordRuleDetail{
		Model:     "res.partner",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "no_parent", Domain: "[('parent_id','=',False)]", Global: true, Applies: true},
		},
	}

	record := RecordData{"parent_id": nil}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (nil == False in Odoo)", result.Verdict)
	}
}

// ─── Like / ILike ───────────────────────────────────────────────────────────

func TestDomainEvalILike(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "res.partner",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "name_rule", Domain: "[('name','ilike','%admin%')]", Global: true, Applies: true},
		},
	}

	record := RecordData{"name": "System Administrator"}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (ilike %%admin%% matches)", result.Verdict)
	}

	record = RecordData{"name": "Demo User"}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

// ─── child_of / parent_of (optimistic) ─────────────────────────────────────

func TestDomainEvalChildOfOptimistic(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "res.partner",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "child_rule", Domain: "[('parent_id','child_of',5)]", Global: true, Applies: true},
		},
	}

	record := RecordData{"parent_id": float64(5)}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// child_of can't be fully evaluated, defaults to true (optimistic)
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (child_of defaults to pass)", result.Verdict)
	}
}

// ─── Stage field ────────────────────────────────────────────────────────────

func TestDomainEvalStageField(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "test", Domain: "[]", Global: true, Applies: true},
		},
	}

	result, err := eval.Evaluate(detail, RecordData{}, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stage != "domain" {
		t.Errorf("Stage = %q, want 'domain'", result.Stage)
	}
}

// ─── Detail structure ───────────────────────────────────────────────────────

func TestDomainEvalDetailStructure(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "active_rule", Domain: "[('active','=',True)]", Global: true, Applies: true},
		},
		GroupRules: []RecordRuleMatch{
			{Name: "owner_rule", Domain: "[('user_id','=',user.id)]", Global: false, Applies: true},
		},
	}

	record := RecordData{"active": true, "user_id": float64(42)}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	evalDetail := result.Detail.(*DomainEvalDetail)
	if evalDetail.Model != "sale.order" {
		t.Errorf("Model = %q, want 'sale.order'", evalDetail.Model)
	}
	if !evalDetail.GlobalPass {
		t.Error("GlobalPass should be true")
	}
	if !evalDetail.GroupPass {
		t.Error("GroupPass should be true")
	}
	if len(evalDetail.RuleResults) != 2 {
		t.Fatalf("RuleResults count = %d, want 2", len(evalDetail.RuleResults))
	}

	// Check condition details
	globalResult := evalDetail.RuleResults[0]
	if globalResult.RuleName != "active_rule" {
		t.Errorf("RuleName = %q, want 'active_rule'", globalResult.RuleName)
	}
	if !globalResult.Pass {
		t.Error("global rule should pass")
	}
	if len(globalResult.Conditions) != 1 {
		t.Fatalf("Conditions count = %d, want 1", len(globalResult.Conditions))
	}
	cond := globalResult.Conditions[0]
	if cond.Field != "active" {
		t.Errorf("Field = %q, want 'active'", cond.Field)
	}
	if !cond.Pass {
		t.Error("condition should pass")
	}
}

// ─── Non-applicable group rules in output ───────────────────────────────────

func TestDomainEvalNonApplicableGroupRuleInOutput(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GroupRules: []RecordRuleMatch{
			{Name: "applies_rule", Domain: "[('active','=',True)]", Global: false, Applies: true},
			{Name: "skipped_rule", Domain: "[('state','=','draft')]", Global: false, Applies: false},
		},
	}

	record := RecordData{"active": true}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	evalDetail := result.Detail.(*DomainEvalDetail)
	// Both rules should appear in output
	if len(evalDetail.RuleResults) != 2 {
		t.Fatalf("RuleResults count = %d, want 2", len(evalDetail.RuleResults))
	}
}

// ─── Real-world Odoo domain ─────────────────────────────────────────────────

func TestDomainEvalRealWorldCompanyRule(t *testing.T) {
	// Real Odoo domain: ['|', ('company_id', '=', False), ('company_id', 'in', company_ids)]
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{
				Name:    "company_rule",
				Domain:  "['|',('company_id','=',False),('company_id','in',company_ids)]",
				Global:  true,
				Applies: true,
			},
		},
	}

	// company_id=1, user's company_ids=[1,2] → passes via 'in' branch
	record := RecordData{"company_id": float64(1)}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	// company_id=False → passes via '=' branch
	record = RecordData{"company_id": false}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	// company_id=99 → fails both branches
	record = RecordData{"company_id": float64(99)}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestDomainEvalRealWorldPrivateEmployeeRule(t *testing.T) {
	// Real Odoo domain: ['|', ('user_id', '=', user.id), ('user_id', '=', False)]
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "res.partner",
		Operation: OpRead,
		GroupRules: []RecordRuleMatch{
			{
				Name:    "res.partner.rule.private.employee",
				Domain:  "['|',('user_id','=',user.id),('user_id','=',False)]",
				Global:  false,
				Applies: true,
			},
		},
	}

	// user_id=42 → matches user.id
	record := RecordData{"user_id": float64(42)}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	// user_id=12 → mismatch → DENIED
	record = RecordData{"user_id": float64(12)}
	result, err = eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED (user_id=12 != user.id=42)", result.Verdict)
	}
}

// ─── Numeric type coercion ──────────────────────────────────────────────────

func TestDomainEvalNumericCoercion(t *testing.T) {
	eval := NewDomainEvaluator()
	detail := &RecordRuleDetail{
		Model:     "sale.order",
		Operation: OpRead,
		GlobalRules: []RecordRuleMatch{
			{Name: "id_rule", Domain: "[('id','=',42)]", Global: true, Applies: true},
		},
	}

	// JSON numbers come as float64 from the agent
	record := RecordData{"id": float64(42)}
	result, err := eval.Evaluate(detail, record, defaultEvalCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (float64(42) == int64(42))", result.Verdict)
	}
}

// ─── Like pattern matching ──────────────────────────────────────────────────

func TestMatchLike(t *testing.T) {
	tests := []struct {
		s, pattern string
		want       bool
	}{
		{"hello", "%llo", true},
		{"hello", "hel%", true},
		{"hello", "%ell%", true},
		{"hello", "hello", true},
		{"hello", "world", false},
		{"hello", "h_llo", true},
		{"hello", "h_lo", false},
		{"", "%", true},
		{"", "", true},
		{"abc", "___", true},
		{"ab", "___", false},
	}
	for _, tt := range tests {
		got := matchLike(tt.s, tt.pattern)
		if got != tt.want {
			t.Errorf("matchLike(%q, %q) = %v, want %v", tt.s, tt.pattern, got, tt.want)
		}
	}
}
