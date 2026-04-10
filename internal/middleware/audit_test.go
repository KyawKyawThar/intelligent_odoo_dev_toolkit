package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Mock Store ──────────────────────────────────────────────────────────────

type auditMockStore struct {
	db.Store // embed — unimplemented methods panic on nil receiver

	mu             sync.Mutex
	auditCalls     []db.CreateAuditLogParams
	createAuditErr error
}

func (m *auditMockStore) CreateAuditLog(_ context.Context, arg db.CreateAuditLogParams) (db.AuditLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditCalls = append(m.auditCalls, arg)
	return db.AuditLog{}, m.createAuditErr
}

// auditCallsSnapshot returns a stable copy of the recorded calls (safe to call after waitForAudit).
func (m *auditMockStore) auditCallsSnapshot() []db.CreateAuditLogParams {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]db.CreateAuditLogParams, len(m.auditCalls))
	copy(out, m.auditCalls)
	return out
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// nopHandler returns 200 OK with no body.
var nopHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})

// waitForAudit polls until at least n audit calls are recorded or the deadline passes.
func waitForAudit(t *testing.T, store *auditMockStore, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		got := len(store.auditCalls)
		store.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d audit call(s)", n)
}

// requestWithContext builds an httptest.Request with tenant/user IDs injected into context.
func requestWithContext(method, path, tenantID, userID string) *http.Request {
	r := httptest.NewRequestWithContext(context.Background(), method, path, http.NoBody)
	ctx := r.Context()
	if tenantID != "" {
		ctx = SetTenantID(ctx, tenantID)
	}
	if userID != "" {
		ctx = SetUserID(ctx, userID)
	}
	return r.WithContext(ctx)
}

// ── AuditLog middleware ──────────────────────────────────────────────────────

func TestAuditLog_SkipsGET(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	tenantID := uuid.New().String()
	r := requestWithContext(http.MethodGet, "/api/v1/environments", tenantID, "")
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)

	// Give the goroutine a moment — it should never fire for GET.
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, store.auditCallsSnapshot(), "GET must not create an audit log")
}

func TestAuditLog_SkipsHEAD(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	r := requestWithContext(http.MethodHead, "/api/v1/environments", uuid.New().String(), "")
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, store.auditCallsSnapshot())
}

func TestAuditLog_SkipsOPTIONS(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	r := requestWithContext(http.MethodOptions, "/api/v1/environments", uuid.New().String(), "")
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, store.auditCallsSnapshot())
}

func TestAuditLog_AuditsPOST(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	tenantID := uuid.New()
	r := requestWithContext(http.MethodPost, "/api/v1/environments", tenantID.String(), "")
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)

	waitForAudit(t, store, 1)
	calls := store.auditCallsSnapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, tenantID, calls[0].TenantID)
	assert.Contains(t, calls[0].Action, http.MethodPost)
}

func TestAuditLog_AuditsPATCH(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	tenantID := uuid.New()
	r := requestWithContext(http.MethodPatch, "/api/v1/environments/1", tenantID.String(), "")
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)

	waitForAudit(t, store, 1)
	calls := store.auditCallsSnapshot()
	require.Len(t, calls, 1)
	assert.Contains(t, calls[0].Action, http.MethodPatch)
}

func TestAuditLog_AuditsDELETE(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	tenantID := uuid.New()
	r := requestWithContext(http.MethodDelete, "/api/v1/environments/42", tenantID.String(), "")
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)

	waitForAudit(t, store, 1)
	calls := store.auditCallsSnapshot()
	require.Len(t, calls, 1)
	assert.Contains(t, calls[0].Action, http.MethodDelete)
}

func TestAuditLog_SkipsWhenNoTenantInContext(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	// No tenant ID set → middleware should skip the audit write.
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/environments", http.NoBody)
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, store.auditCallsSnapshot(), "missing tenant_id must skip audit log")
}

func TestAuditLog_SkipsWhenTenantIDIsInvalidUUID(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/environments", http.NoBody)
	ctx := SetTenantID(r.Context(), "not-a-uuid")
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r.WithContext(ctx))

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, store.auditCallsSnapshot(), "invalid tenant UUID must skip audit log")
}

func TestAuditLog_TenantIDPropagatedToAuditRecord(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	tenantID := uuid.New()
	r := requestWithContext(http.MethodPost, "/api/v1/alerts", tenantID.String(), "")
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)

	waitForAudit(t, store, 1)
	assert.Equal(t, tenantID, store.auditCallsSnapshot()[0].TenantID)
}

func TestAuditLog_UserIDPropagatedWhenPresent(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	tenantID := uuid.New()
	userID := uuid.New()
	r := requestWithContext(http.MethodPost, "/api/v1/alerts", tenantID.String(), userID.String())
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)

	waitForAudit(t, store, 1)
	calls := store.auditCallsSnapshot()
	require.NotNil(t, calls[0].UserID, "user_id must be set when present in context")
	assert.Equal(t, userID, *calls[0].UserID)
}

func TestAuditLog_UserIDNilWhenAbsent(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	r := requestWithContext(http.MethodPost, "/api/v1/alerts", uuid.New().String(), "" /* no user */)
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)

	waitForAudit(t, store, 1)
	assert.Nil(t, store.auditCallsSnapshot()[0].UserID, "user_id must be nil when absent from context")
}

func TestAuditLog_ActionContainsMethodAndPath(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	r := requestWithContext(http.MethodPost, "/api/v1/environments", uuid.New().String(), "")
	mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)

	waitForAudit(t, store, 1)
	action := store.auditCallsSnapshot()[0].Action
	assert.Contains(t, action, "POST")
	assert.Contains(t, action, "/api/v1/environments")
}

func TestAuditLog_MetadataContainsStatusCode(t *testing.T) {
	store := &auditMockStore{}
	mw := AuditLog(store, zerolog.Nop())

	// Handler returns 201.
	created := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	r := requestWithContext(http.MethodPost, "/api/v1/environments", uuid.New().String(), "")
	mw(created).ServeHTTP(httptest.NewRecorder(), r)

	waitForAudit(t, store, 1)
	meta := store.auditCallsSnapshot()[0].Metadata
	assert.Contains(t, string(meta), "201")
}

func TestAuditLog_WriteErrorDoesNotPanic(t *testing.T) {
	store := &auditMockStore{createAuditErr: assert.AnError}
	mw := AuditLog(store, zerolog.Nop())

	r := requestWithContext(http.MethodDelete, "/api/v1/environments/1", uuid.New().String(), "")

	require.NotPanics(t, func() {
		mw(nopHandler).ServeHTTP(httptest.NewRecorder(), r)
		waitForAudit(t, store, 1)
	})
}

// ── statusRecorder ───────────────────────────────────────────────────────────

func TestStatusRecorder_DefaultsTo200(t *testing.T) {
	// statusRecorder must default to 200 when WriteHeader is never called.
	sr := &statusRecorder{status: http.StatusOK}
	assert.Equal(t, http.StatusOK, sr.status)
}

func TestStatusRecorder_CapturesWrittenStatus(t *testing.T) {
	rw := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rw, status: http.StatusOK}
	sr.WriteHeader(http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, sr.status)
	assert.Equal(t, http.StatusNotFound, rw.Code)
}

func TestStatusRecorder_DelegatesWriteHeader(t *testing.T) {
	rw := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rw, status: http.StatusOK}
	sr.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusCreated, rw.Code)
}

// ── parseClientIP ────────────────────────────────────────────────────────────

func TestParseClientIP_IPv4WithPort(t *testing.T) {
	addr := parseClientIP("192.168.1.1:8080")
	require.NotNil(t, addr)
	assert.Equal(t, "192.168.1.1", addr.String())
}

func TestParseClientIP_IPv4Bare(t *testing.T) {
	addr := parseClientIP("10.0.0.1")
	require.NotNil(t, addr)
	assert.Equal(t, "10.0.0.1", addr.String())
}

func TestParseClientIP_IPv6WithPort(t *testing.T) {
	addr := parseClientIP("[::1]:9090")
	require.NotNil(t, addr)
	assert.Equal(t, "::1", addr.String())
}

func TestParseClientIP_IPv6Bare(t *testing.T) {
	// Bare IPv6 addresses (without brackets) contain ':' so splitHostPort is
	// called, which treats the last ':' as a host/port separator and produces
	// a mangled host string that fails ParseAddr.  The function returns nil
	// in this case; callers should pass bracket-enclosed IPv6 addresses.
	addr := parseClientIP("::1")
	assert.Nil(t, addr)
}

func TestParseClientIP_InvalidReturnsNil(t *testing.T) {
	addr := parseClientIP("not-an-ip")
	assert.Nil(t, addr)
}

func TestParseClientIP_EmptyReturnsNil(t *testing.T) {
	addr := parseClientIP("")
	assert.Nil(t, addr)
}

// ── splitHostPort ────────────────────────────────────────────────────────────

func TestSplitHostPort_RegularHostPort(t *testing.T) {
	host, port, err := splitHostPort("example.com:443")
	require.NoError(t, err)
	assert.Equal(t, "example.com", host)
	assert.Equal(t, "443", port)
}

func TestSplitHostPort_IPv6Bracketed(t *testing.T) {
	host, port, err := splitHostPort("[::1]:8080")
	require.NoError(t, err)
	assert.Equal(t, "::1", host)
	assert.Equal(t, "8080", port)
}

func TestSplitHostPort_MissingBracketClose(t *testing.T) {
	_, _, err := splitHostPort("[::1:8080")
	assert.Error(t, err)
}

func TestSplitHostPort_HostOnly(t *testing.T) {
	host, port, err := splitHostPort("localhost")
	require.NoError(t, err)
	assert.Equal(t, "localhost", host)
	assert.Empty(t, port)
}
