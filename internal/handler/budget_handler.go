package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"
)

// BudgetHandler handles the performance budget endpoints.
type BudgetHandler struct {
	*BaseHandler
	svc *service.BudgetService
}

// NewBudgetHandler creates a BudgetHandler.
func NewBudgetHandler(svc *service.BudgetService, base *BaseHandler) *BudgetHandler {
	return &BudgetHandler{BaseHandler: base, svc: svc}
}

// HandleCreate creates a new performance budget.
//
//	@Summary      Create performance budget
//	@Description  Create a new performance budget for an environment
//	@Tags         budgets
//	@Accept       json
//	@Produce      json
//	@Param        env_id  path      string                  true  "Environment ID"
//	@Param        body    body      dto.CreateBudgetRequest  true  "Budget config"
//	@Success      201     {object}  dto.BudgetResponse
//	@Failure      400     {object}  api.Error
//	@Failure      401     {object}  api.Error
//	@Failure      404     {object}  api.Error
//	@Router       /environments/{env_id}/budgets [post]
//	@Security     BearerAuth
func (h *BudgetHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	var req dto.CreateBudgetRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	result, err := h.svc.Create(r.Context(), tenantID, envID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, result)
}

// HandleGet returns a budget with its latest sample and trend.
//
//	@Summary      Get performance budget
//	@Description  Retrieve a budget with latest sample and 7-day trend
//	@Tags         budgets
//	@Produce      json
//	@Param        env_id     path  string  true  "Environment ID"
//	@Param        budget_id  path  string  true  "Budget ID"
//	@Success      200  {object}  dto.BudgetDetailResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/budgets/{budget_id} [get]
//	@Security     BearerAuth
func (h *BudgetHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, budgetID, ok := h.MustTenantEnvAndExtraID(w, r, "budget_id")
	if !ok {
		return
	}

	result, err := h.svc.GetByID(r.Context(), tenantID, envID, budgetID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleList lists budgets for an environment.
//
//	@Summary      List performance budgets
//	@Description  List performance budgets for an environment
//	@Tags         budgets
//	@Produce      json
//	@Param        env_id           path   string  true   "Environment ID"
//	@Param        include_inactive query  bool    false  "Include inactive budgets (default false)"
//	@Success      200  {object}  dto.BudgetListResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Router       /environments/{env_id}/budgets [get]
//	@Security     BearerAuth
func (h *BudgetHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	includeInactive := r.URL.Query().Get("include_inactive") == "true"

	result, err := h.svc.List(r.Context(), tenantID, envID, includeInactive)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleUpdate updates a budget's threshold and active status.
//
//	@Summary      Update performance budget
//	@Description  Update a budget's threshold percentage and active status
//	@Tags         budgets
//	@Accept       json
//	@Produce      json
//	@Param        env_id     path  string                  true  "Environment ID"
//	@Param        budget_id  path  string                  true  "Budget ID"
//	@Param        body       body  dto.UpdateBudgetRequest  true  "Updated budget config"
//	@Success      200  {object}  dto.BudgetResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/budgets/{budget_id} [patch]
//	@Security     BearerAuth
func (h *BudgetHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, budgetID, ok := h.MustTenantEnvAndExtraID(w, r, "budget_id")
	if !ok {
		return
	}

	var req dto.UpdateBudgetRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	result, err := h.svc.Update(r.Context(), tenantID, envID, budgetID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleDelete removes a budget.
//
//	@Summary      Delete performance budget
//	@Description  Delete a performance budget and its samples
//	@Tags         budgets
//	@Param        env_id     path  string  true  "Environment ID"
//	@Param        budget_id  path  string  true  "Budget ID"
//	@Success      204
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/budgets/{budget_id} [delete]
//	@Security     BearerAuth
func (h *BudgetHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, budgetID, ok := h.MustTenantEnvAndExtraID(w, r, "budget_id")
	if !ok {
		return
	}

	if err := h.svc.Delete(r.Context(), tenantID, envID, budgetID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// HandleListSamples lists performance samples for a budget.
//
//	@Summary      List budget samples
//	@Description  List performance overhead samples for a budget
//	@Tags         budgets
//	@Produce      json
//	@Param        env_id     path   string  true   "Environment ID"
//	@Param        budget_id  path   string  true   "Budget ID"
//	@Param        limit      query  int     false  "Max results (default 50)"
//	@Success      200  {object}  dto.BudgetSampleListResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/budgets/{budget_id}/samples [get]
//	@Security     BearerAuth
func (h *BudgetHandler) HandleListSamples(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, budgetID, ok := h.MustTenantEnvAndExtraID(w, r, "budget_id")
	if !ok {
		return
	}

	limit := ParseQueryInt32(r, "limit", 50)

	result, err := h.svc.ListSamples(r.Context(), tenantID, envID, budgetID, limit)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleGetTrend returns the 7-day performance trend for a budget.
//
//	@Summary      Get budget trend
//	@Description  Get the 7-day average, max, and sample count for a budget
//	@Tags         budgets
//	@Produce      json
//	@Param        env_id     path  string  true  "Environment ID"
//	@Param        budget_id  path  string  true  "Budget ID"
//	@Success      200  {object}  dto.BudgetTrendResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/budgets/{budget_id}/trend [get]
//	@Security     BearerAuth
func (h *BudgetHandler) HandleGetTrend(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, budgetID, ok := h.MustTenantEnvAndExtraID(w, r, "budget_id")
	if !ok {
		return
	}

	result, err := h.svc.GetTrend(r.Context(), tenantID, envID, budgetID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleGetBreakdown returns the function-level breakdown for a budget sample.
//
//	@Summary      Get function-level breakdown
//	@Description  Get per-function performance breakdown for a specific budget sample
//	@Tags         budgets
//	@Produce      json
//	@Param        env_id     path  string  true  "Environment ID"
//	@Param        budget_id  path  string  true  "Budget ID"
//	@Param        sample_id  path  string  true  "Sample ID"
//	@Success      200  {object}  dto.FunctionBreakdownResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/budgets/{budget_id}/samples/{sample_id}/breakdown [get]
//	@Security     BearerAuth
func (h *BudgetHandler) HandleGetBreakdown(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, budgetID, ok := h.MustTenantEnvAndExtraID(w, r, "budget_id")
	if !ok {
		return
	}

	sampleID, ok := h.MustUUIDParam(w, r, "sample_id")
	if !ok {
		return
	}

	result, err := h.svc.GetBreakdown(r.Context(), tenantID, envID, budgetID, sampleID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}
