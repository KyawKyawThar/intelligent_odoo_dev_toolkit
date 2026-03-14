package collector

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"

	"github.com/stretchr/testify/require"
)

const irModelAccessRespCorrect = `<?xml version='1.0'?>
<methodResponse>
  <params>
    <param>
      <value>
        <array><data>
          <value><struct>
            <member><name>id</name><value><int>1</int></value></member>
            <member><name>name</name><value><string>res.partner access</string></value></member>
            <member><name>model_id</name><value><array><data><value><int>10</int></value><value><string>res.partner</string></value></data></array></value></member>
            <member><name>group_id</name><value><array><data><value><int>5</int></value><value><string>Sales</string></value></data></array></value></member>
            <member><name>perm_read</name><value><boolean>1</boolean></value></member>
            <member><name>perm_write</name><value><boolean>0</boolean></value></member>
            <member><name>perm_create</name><value><boolean>0</boolean></value></member>
            <member><name>perm_unlink</name><value><boolean>0</boolean></value></member>
          </struct></value>
        </data></array>
      </value>
    </param>
  </params>
</methodResponse>`

const irRuleRespCorrect = `<?xml version='1.0'?>
<methodResponse>
  <params>
    <param>
      <value>
        <array><data>
          <value><struct>
            <member><name>id</name><value><int>100</int></value></member>
            <member><name>name</name><value><string>res.partner rule</string></value></member>
            <member><name>model_id</name><value><array><data><value><int>10</int></value><value><string>res.partner</string></value></data></array></value></member>
            <member><name>groups</name><value><array><data><value><int>5</int></value></data></array></value></member>
            <member><name>domain_force</name><value><string>[('is_company', '=', True)]</string></value></member>
            <member><name>perm_read</name><value><boolean>1</boolean></value></member>
            <member><name>perm_write</name><value><boolean>1</boolean></value></member>
            <member><name>perm_create</name><value><boolean>1</boolean></value></member>
            <member><name>perm_unlink</name><value><boolean>1</boolean></value></member>
			<member><name>global</name><value><boolean>0</boolean></value></member>
			<member><name>active</name><value><boolean>1</boolean></value></member>
          </struct></value>
        </data></array>
      </value>
    </param>
  </params>
</methodResponse>`

func newMockACLServerCorrect(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore the body

		body := string(bodyBytes)

		switch r.URL.Path {
		case "/xmlrpc/2/common":
			w.Write([]byte(authSuccessResp))
		case "/xmlrpc/2/object":
			switch {
			case strings.Contains(body, "ir.model.access"):
				w.Write([]byte(irModelAccessRespCorrect))
			case strings.Contains(body, "ir.rule"):
				w.Write([]byte(irRuleRespCorrect))
			default:
				http.Error(w, "unexpected model request for ACL", http.StatusBadRequest)
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestCollectACLAndRules(t *testing.T) {
	srv := newMockACLServerCorrect(t)
	defer srv.Close()

	client, err := odoo.NewClient(srv.URL, "testdb", "admin", "password")
	require.NoError(t, err)
	err = client.Authenticate(context.Background())
	require.NoError(t, err)

	accessList, ruleList, err := CollectACLAndRules(context.Background(), client)
	require.NoError(t, err)

	require.Len(t, accessList, 1)
	access := accessList[0]
	require.Equal(t, 1, access.ID)
	require.Equal(t, "res.partner access", access.Name)
	require.Equal(t, 10, access.ModelID)
	require.Equal(t, 5, access.GroupID)
	require.True(t, access.PermRead)
	require.False(t, access.PermWrite)

	require.Len(t, ruleList, 1)
	rule := ruleList[0]
	require.Equal(t, 100, rule.ID)
	require.Equal(t, "res.partner rule", rule.Name)
	require.Equal(t, 10, rule.ModelID)
	require.Equal(t, []int{5}, rule.Groups)
	require.Equal(t, "[('is_company', '=', True)]", rule.Domain)
	require.True(t, rule.PermRead)
	require.True(t, rule.Active)
	require.False(t, rule.Global)
}
