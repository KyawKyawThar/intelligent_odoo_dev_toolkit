package service

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/acl"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"testing"
)

// TestBuildResponse_AllOK verifies that an all-OK pipeline returns ALLOWED.
func TestBuildResponse_AllOK(t *testing.T) {
	stages := []acl.StageResult{
		{Stage: "user", Verdict: acl.VerdictOK, Reason: "user resolved"},
		{Stage: "groups", Verdict: acl.VerdictOK, Reason: "groups resolved"},
		{Stage: "model_acl", Verdict: acl.VerdictOK, Reason: "access granted"},
		{Stage: "record_rule", Verdict: acl.VerdictOK, Reason: "rules found"},
		{Stage: "domain", Verdict: acl.VerdictOK, Reason: "domains pass"},
	}

	resp := buildResponse(stages, acl.NewSuggestionGenerator())

	if resp.Verdict != "ALLOWED" {
		t.Errorf("expected ALLOWED, got %s", resp.Verdict)
	}
	if len(resp.Stages) != 5 {
		t.Errorf("expected 5 stages, got %d", len(resp.Stages))
	}
}

// TestBuildResponse_DeniedAtModelACL verifies early denial propagates correctly.
func TestBuildResponse_DeniedAtModelACL(t *testing.T) {
	stages := []acl.StageResult{
		{Stage: "user", Verdict: acl.VerdictOK, Reason: "user resolved"},
		{Stage: "groups", Verdict: acl.VerdictOK, Reason: "groups resolved"},
		{Stage: "model_acl", Verdict: acl.VerdictDenied, Reason: "no rules grant access"},
	}

	resp := buildResponse(stages, acl.NewSuggestionGenerator())

	if resp.Verdict != "DENIED" {
		t.Errorf("expected DENIED, got %s", resp.Verdict)
	}
	if len(resp.Stages) != 3 {
		t.Errorf("expected 3 stages, got %d", len(resp.Stages))
	}
}

// TestBuildResponse_ErrorVerdict verifies that ERROR stage also yields DENIED.
func TestBuildResponse_ErrorVerdict(t *testing.T) {
	stages := []acl.StageResult{
		{Stage: "user", Verdict: acl.VerdictError, Reason: "res.users not in schema"},
	}

	resp := buildResponse(stages, acl.NewSuggestionGenerator())

	if resp.Verdict != "DENIED" {
		t.Errorf("expected DENIED, got %s", resp.Verdict)
	}
}

// TestBuildResponse_SkippedStagesAllowed verifies SKIPPED stages count as allowed.
func TestBuildResponse_SkippedStagesAllowed(t *testing.T) {
	stages := []acl.StageResult{
		{Stage: "user", Verdict: acl.VerdictOK, Reason: "user resolved"},
		{Stage: "groups", Verdict: acl.VerdictOK, Reason: "groups resolved"},
		{Stage: "model_acl", Verdict: acl.VerdictOK, Reason: "access granted"},
		{Stage: "record_rule", Verdict: acl.VerdictSkipped, Reason: "no rules"},
	}

	resp := buildResponse(stages, acl.NewSuggestionGenerator())

	if resp.Verdict != "ALLOWED" {
		t.Errorf("expected ALLOWED, got %s", resp.Verdict)
	}
}

// TestACLTraceRequest_Validation tests that the DTO struct tags are correct.
func TestACLTraceRequest_Fields(t *testing.T) {
	req := dto.ACLTraceRequest{
		UserID:    2,
		Model:     "sale.order",
		Operation: "read",
		UserData:  map[string]any{"id": 2, "login": "admin"},
		GroupData: []map[string]any{{"id": 1, "name": "base.group_user"}},
	}

	if req.UserID != 2 {
		t.Errorf("expected UserID 2, got %d", req.UserID)
	}
	if req.Model != "sale.order" {
		t.Errorf("expected sale.order, got %s", req.Model)
	}
	if req.Operation != "read" {
		t.Errorf("expected read, got %s", req.Operation)
	}
	if req.UserData["id"] != 2 {
		t.Errorf("expected UserData id 2, got %v", req.UserData["id"])
	}
	if len(req.GroupData) != 1 {
		t.Errorf("expected 1 group, got %d", len(req.GroupData))
	}
}
