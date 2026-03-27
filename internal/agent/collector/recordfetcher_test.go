package collector

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Intelligent_Dev_ToolKit_Odoo/internal/acl"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"

	"github.com/stretchr/testify/require"
)

// ─── XML-RPC mock responses ─────────────────────────────────────────────────

const saleOrderRecordResp = `<?xml version='1.0'?>
<methodResponse>
  <params>
    <param>
      <value>
        <array><data>
          <value><struct>
            <member><name>id</name><value><int>42</int></value></member>
            <member><name>active</name><value><boolean>1</boolean></value></member>
            <member><name>user_id</name><value><array><data><value><int>7</int></value><value><string>Demo User</string></value></data></array></value></member>
            <member><name>company_id</name><value><array><data><value><int>1</int></value><value><string>My Company</string></value></data></array></value></member>
            <member><name>state</name><value><string>draft</string></value></member>
            <member><name>amount_total</name><value><double>1500.50</double></value></member>
          </struct></value>
        </data></array>
      </value>
    </param>
  </params>
</methodResponse>`

const emptyRecordResp = `<?xml version='1.0'?>
<methodResponse>
  <params>
    <param>
      <value>
        <array><data>
        </data></array>
      </value>
    </param>
  </params>
</methodResponse>`

func newMockRecordServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		body := string(bodyBytes)

		switch r.URL.Path {
		case "/xmlrpc/2/common":
			w.Write([]byte(authSuccessResp))
		case "/xmlrpc/2/object":
			switch {
			case strings.Contains(body, "sale.order"):
				w.Write([]byte(saleOrderRecordResp))
			case strings.Contains(body, "nonexistent.model"):
				w.Write([]byte(emptyRecordResp))
			default:
				w.Write([]byte(emptyRecordResp))
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

// ─── FetchRecord tests ──────────────────────────────────────────────────────

func TestFetchRecordSuccess(t *testing.T) {
	srv := newMockRecordServer(t)
	defer srv.Close()

	client, err := odoo.NewClient(srv.URL, "testdb", "admin", "password")
	require.NoError(t, err)
	err = client.Authenticate(context.Background())
	require.NoError(t, err)

	fetcher := NewRecordFetcher(client)
	record, err := fetcher.FetchRecord(
		context.Background(),
		"sale.order",
		42,
		[]string{"active", "user_id", "company_id", "state", "amount_total"},
	)
	require.NoError(t, err)
	require.NotNil(t, record)

	// active is boolean
	require.Equal(t, true, record["active"])

	// state is string
	require.Equal(t, "draft", record["state"])

	// amount_total is float
	require.Equal(t, 1500.50, record["amount_total"])

	// user_id is many2one [id, "name"]
	userID, ok := record["user_id"].([]any)
	require.True(t, ok, "user_id should be []any (many2one)")
	require.Len(t, userID, 2)
	require.Equal(t, 7, userID[0])

	// company_id is many2one [id, "name"]
	companyID, ok := record["company_id"].([]any)
	require.True(t, ok, "company_id should be []any (many2one)")
	require.Equal(t, 1, companyID[0])
}

func TestFetchRecordNotFound(t *testing.T) {
	srv := newMockRecordServer(t)
	defer srv.Close()

	client, err := odoo.NewClient(srv.URL, "testdb", "admin", "password")
	require.NoError(t, err)
	err = client.Authenticate(context.Background())
	require.NoError(t, err)

	fetcher := NewRecordFetcher(client)
	_, err = fetcher.FetchRecord(
		context.Background(),
		"nonexistent.model",
		999,
		[]string{"active"},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "record not found")
}

func TestFetchRecordEmptyFields(t *testing.T) {
	srv := newMockRecordServer(t)
	defer srv.Close()

	client, err := odoo.NewClient(srv.URL, "testdb", "admin", "password")
	require.NoError(t, err)
	err = client.Authenticate(context.Background())
	require.NoError(t, err)

	fetcher := NewRecordFetcher(client)
	record, err := fetcher.FetchRecord(
		context.Background(),
		"sale.order",
		42,
		[]string{}, // no fields
	)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Empty(t, record)
}

// ─── FetchRecordForRules tests ──────────────────────────────────────────────

func TestFetchRecordForRulesExtractsFields(t *testing.T) {
	srv := newMockRecordServer(t)
	defer srv.Close()

	client, err := odoo.NewClient(srv.URL, "testdb", "admin", "password")
	require.NoError(t, err)
	err = client.Authenticate(context.Background())
	require.NoError(t, err)

	fetcher := NewRecordFetcher(client)

	ruleDetail := &acl.RecordRuleDetail{
		Model:     "sale.order",
		Operation: acl.OpRead,
		GlobalRules: []acl.RecordRuleMatch{
			{Name: "active_rule", Domain: "[('active','=',True)]", Global: true, Applies: true},
		},
		GroupRules: []acl.RecordRuleMatch{
			{Name: "owner_rule", Domain: "[('user_id','=',user.id)]", Global: false, Applies: true},
			{Name: "skipped_rule", Domain: "[('state','=','draft')]", Global: false, Applies: false},
		},
	}

	record, err := fetcher.FetchRecordForRules(
		context.Background(),
		"sale.order",
		42,
		ruleDetail,
	)
	require.NoError(t, err)
	require.NotNil(t, record)

	// Should have fetched fields from applicable rules (active, user_id)
	// Not state (from skipped_rule which Applies=false)
	_, hasActive := record["active"]
	require.True(t, hasActive, "should have fetched 'active' field")
}

func TestFetchRecordForRulesNoApplicableRules(t *testing.T) {
	srv := newMockRecordServer(t)
	defer srv.Close()

	client, err := odoo.NewClient(srv.URL, "testdb", "admin", "password")
	require.NoError(t, err)
	err = client.Authenticate(context.Background())
	require.NoError(t, err)

	fetcher := NewRecordFetcher(client)

	ruleDetail := &acl.RecordRuleDetail{
		Model:     "sale.order",
		Operation: acl.OpRead,
	}

	record, err := fetcher.FetchRecordForRules(
		context.Background(),
		"sale.order",
		42,
		ruleDetail,
	)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Empty(t, record)
}

// ─── ExtractFieldsFromDomains tests ─────────────────────────────────────────

func TestExtractFieldsFromDomainsSingle(t *testing.T) {
	fields := ExtractFieldsFromDomains([]string{
		"[('active','=',True)]",
	})
	require.Equal(t, []string{"active"}, fields)
}

func TestExtractFieldsFromDomainsMultiple(t *testing.T) {
	fields := ExtractFieldsFromDomains([]string{
		"[('active','=',True)]",
		"[('user_id','=',user.id)]",
		"['|',('company_id','=',False),('company_id','in',company_ids)]",
	})
	require.Len(t, fields, 3)

	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[f] = true
	}
	require.True(t, fieldSet["active"])
	require.True(t, fieldSet["user_id"])
	require.True(t, fieldSet["company_id"])
}

func TestExtractFieldsFromDomainsDedup(t *testing.T) {
	// company_id appears in both domains — should only appear once.
	fields := ExtractFieldsFromDomains([]string{
		"[('company_id','=',1)]",
		"[('company_id','in',company_ids)]",
	})
	require.Len(t, fields, 1)
	require.Equal(t, "company_id", fields[0])
}

func TestExtractFieldsFromDomainsEmpty(t *testing.T) {
	fields := ExtractFieldsFromDomains([]string{"[]"})
	require.Empty(t, fields)
}

func TestExtractFieldsFromDomainsInvalidDomain(t *testing.T) {
	fields := ExtractFieldsFromDomains([]string{"NOT_VALID"})
	require.Empty(t, fields)
}

func TestExtractFieldsFromDomainsComplex(t *testing.T) {
	// Real-world Odoo domain with OR and multiple fields.
	fields := ExtractFieldsFromDomains([]string{
		"['|','|',('user_id','=',user.id),('user_id','=',False),('message_follower_ids','in',[user.partner_id.id])]",
	})

	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[f] = true
	}
	require.True(t, fieldSet["user_id"])
	require.True(t, fieldSet["message_follower_ids"])
	require.Len(t, fields, 2) // user_id deduped
}
