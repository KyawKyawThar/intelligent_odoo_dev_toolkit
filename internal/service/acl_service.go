package service

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/acl"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const (
	verdictAllowed = "ALLOWED"
	verdictDenied  = "DENIED"
)

// ACLService orchestrates the 5-stage ACL pipeline.
type ACLService struct {
	store         db.Store
	userResolver  *acl.UserResolver
	groupResolver *acl.GroupResolver
	modelChecker  *acl.ModelACLChecker
	ruleFinder    *acl.RecordRuleFinder
	domainEval    *acl.DomainEvaluator
	suggestionGen *acl.SuggestionGenerator
}

// NewACLService creates an ACLService with all pipeline stages.
func NewACLService(store db.Store) *ACLService {
	return &ACLService{
		store:         store,
		userResolver:  acl.NewUserResolver(),
		groupResolver: acl.NewGroupResolver(),
		modelChecker:  acl.NewModelACLChecker(),
		ruleFinder:    acl.NewRecordRuleFinder(),
		domainEval:    acl.NewDomainEvaluator(),
		suggestionGen: acl.NewSuggestionGenerator(),
	}
}

// TraceAccess runs the full 5-stage ACL pipeline and returns the result.
//
// Stages:
//  1. User Resolver    — validate and extract user info
//  2. Group Resolver   — expand groups through implied_ids
//  3. Model ACL Check  — evaluate ir.model.access rules
//  4. Record Rule Find — find and filter ir.rule entries
//  5. Domain Evaluator — evaluate rule domains against record data
func (s *ACLService) TraceAccess(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	req *dto.ACLTraceRequest,
) (*dto.ACLTraceResponse, error) {
	// ── Load schema snapshot ────────────────────────────────────────────
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	snapshot, err := s.store.GetLatestSchema(ctx, envID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	var schema acl.SchemaModels
	if unmarshalErr := json.Unmarshal(snapshot.Models, &schema); unmarshalErr != nil {
		return nil, api.ErrInternal(fmt.Errorf("unmarshal schema models: %w", unmarshalErr))
	}

	p := &aclPipeline{
		s:      s,
		ctx:    ctx,
		req:    req,
		schema: schema,
		stages: make([]acl.StageResult, 0, 5),
		op:     acl.Operation(req.Operation),
	}

	return p.run()
}

// buildResponse constructs the final ACLTraceResponse from the accumulated stages.
// The overall verdict is ALLOWED only if no stage returned DENIED.
// When the verdict is DENIED, actionable fix suggestions are generated.
func buildResponse(stages []acl.StageResult, sg *acl.SuggestionGenerator) *dto.ACLTraceResponse {
	verdict := verdictAllowed
	for _, s := range stages {
		if s.Verdict == acl.VerdictDenied || s.Verdict == acl.VerdictError {
			verdict = verdictDenied
			break
		}
	}

	resp := &dto.ACLTraceResponse{
		Verdict: verdict,
		Stages:  stages,
	}

	if verdict == verdictDenied && sg != nil {
		resp.Suggestions = sg.Generate(stages)
	}

	return resp
}

type aclPipeline struct {
	s      *ACLService
	ctx    context.Context
	req    *dto.ACLTraceRequest
	schema acl.SchemaModels
	stages []acl.StageResult
	user   *acl.ResolvedUser
	groups *acl.ResolvedGroups
	op     acl.Operation
}

func (p *aclPipeline) run() (*dto.ACLTraceResponse, error) {
	if stop, err := p.resolveUser(); err != nil {
		return nil, api.ErrInternal(fmt.Errorf("stage 1 (user): %w", err))
	} else if stop {
		return p.buildResponse(), nil
	}

	if stop, err := p.resolveGroups(); err != nil {
		return nil, api.ErrInternal(fmt.Errorf("stage 2 (groups): %w", err))
	} else if stop {
		return p.buildResponse(), nil
	}

	if stop, err := p.checkModelACL(); err != nil {
		return nil, api.ErrInternal(fmt.Errorf("stage 3 (model_acl): %w", err))
	} else if stop {
		return p.buildResponse(), nil
	}

	if stop, err := p.findRecordRules(); err != nil {
		return nil, api.ErrInternal(fmt.Errorf("stage 4 (record_rule): %w", err))
	} else if stop {
		return p.buildResponse(), nil
	}

	if _, err := p.evaluateDomain(); err != nil {
		return nil, api.ErrInternal(fmt.Errorf("stage 5 (domain): %w", err))
	}

	return p.buildResponse(), nil
}

func (p *aclPipeline) resolveUser() (stop bool, err error) {
	user, userStage, err := p.s.userResolver.Resolve(p.schema, p.req.UserData)
	if err != nil {
		return true, err
	}
	p.stages = append(p.stages, *userStage)
	p.user = user
	return userStage.Verdict == acl.VerdictDenied || userStage.Verdict == acl.VerdictError || user.IsSuperUser(), nil
}

func (p *aclPipeline) resolveGroups() (stop bool, err error) {
	groups, groupStage, err := p.s.groupResolver.Resolve(p.schema, p.user, p.req.GroupData)
	if err != nil {
		return true, err
	}
	p.stages = append(p.stages, *groupStage)
	p.groups = groups
	return groupStage.Verdict == acl.VerdictDenied || groupStage.Verdict == acl.VerdictError, nil
}

func (p *aclPipeline) checkModelACL() (stop bool, err error) {
	aclStage, err := p.s.modelChecker.Check(p.schema, p.user, p.groups, p.req.Model, p.op)
	if err != nil {
		return true, err
	}
	p.stages = append(p.stages, *aclStage)
	return aclStage.Verdict == acl.VerdictDenied || aclStage.Verdict == acl.VerdictError, nil
}

func (p *aclPipeline) findRecordRules() (stop bool, err error) {
	ruleStage, err := p.s.ruleFinder.Find(p.schema, p.user, p.groups, p.req.Model, p.op)
	if err != nil {
		return true, err
	}
	p.stages = append(p.stages, *ruleStage)
	return ruleStage.Verdict == acl.VerdictDenied || ruleStage.Verdict == acl.VerdictError || ruleStage.Verdict == acl.VerdictSkipped, nil
}

func (p *aclPipeline) evaluateDomain() (stop bool, err error) {
	ruleDetail, ok := p.stages[len(p.stages)-1].Detail.(*acl.RecordRuleDetail)
	if !ok {
		return true, nil
	}

	if len(p.req.RecordData) == 0 {
		p.stages = append(p.stages, acl.StageResult{
			Stage:   "domain",
			Verdict: acl.VerdictSkipped,
			Reason:  "no record_data provided — domain evaluation skipped (provide record_data to evaluate ir.rule domains)",
		})
		return true, nil
	}

	evalCtx := &acl.EvalContext{
		UserID:     p.user.UID,
		CompanyID:  p.user.CompanyID,
		CompanyIDs: p.user.CompanyIDs,
	}

	domainStage, err := p.s.domainEval.Evaluate(ruleDetail, acl.RecordData(p.req.RecordData), evalCtx)
	if err != nil {
		return true, err
	}
	p.stages = append(p.stages, *domainStage)
	return false, nil
}

func (p *aclPipeline) buildResponse() *dto.ACLTraceResponse {
	return buildResponse(p.stages, p.s.suggestionGen)
}
