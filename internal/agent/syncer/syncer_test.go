package syncer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// --- Test Data ---
const authSuccessResp = `<?xml version='1.0'?>
<methodResponse>
  <params>
    <param><value><int>2</int></value></param>
  </params>
</methodResponse>`

const irModelResp = `<?xml version='1.0'?>
<methodResponse>
  <params>
    <param>
      <value>
        <array><data>
          <value><struct>
            <member><name>id</name><value><int>1</int></value></member>
            <member><name>model</name><value><string>res.partner</string></value></member>
			<member><name>name</name><value><string>Partner</string></value></member>
          </struct></value>
        </data></array>
      </value>
    </param>
  </params>
</methodResponse>`

const irModelFieldsResp = `<?xml version='1.0'?>
<methodResponse>
  <params>
    <param>
      <value>
        <array><data>
          <value><struct>
            <member><name>id</name><value><int>101</int></value></member>
            <member><name>model</name><value><string>res.partner</string></value></member>
            <member><name>name</name><value><string>name</string></value></member>
            <member><name>ttype</name><value><string>char</string></value></member>
          </struct></value>
        </data></array>
      </value>
    </param>
  </params>
</methodResponse>`

const emptyACLResp = `<?xml version='1.0'?>
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

func TestDeltaDetection(t *testing.T) {
	// ── 1. Setup mock server to receive pushes ──────────────────────
	var serverRequests int
	var mu sync.Mutex
	var receivedPayload schemaPayload

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		serverRequests++
		mu.Unlock()

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &receivedPayload)
		require.NoError(t, err)

		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	// ── 2. Setup mock Odoo server ───────────────────────────────────
	odooSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		body := string(bodyBytes)
		switch r.URL.Path {
		case "/xmlrpc/2/object":
			switch {
			case strings.Contains(body, "ir.model.fields"):
				w.Write([]byte(irModelFieldsResp))
			case strings.Contains(body, "ir.model"):
				w.Write([]byte(irModelResp))
			case strings.Contains(body, "ir.rule") || strings.Contains(body, "ir.model.access"):
				w.Write([]byte(emptyACLResp))
			}
		case "/xmlrpc/2/common":
			w.Write([]byte(authSuccessResp))
		}
	}))
	defer odooSrv.Close()

	client, err := odoo.NewClient(odooSrv.URL, "test", "admin", "pw")
	require.NoError(t, err)
	err = client.Authenticate(context.Background())
	require.NoError(t, err)

	// ── 3. Create Syncer and run checks ──────────────────────────────
	logger := zerolog.Nop()
	syncer := New(client, mockServer.URL, "test-api-key", "test-env-id", logger)

	// First run: Should collect and push
	err = syncer.RunOnce(context.Background())
	require.NoError(t, err)
	mu.Lock()
	require.Equal(t, 1, serverRequests, "Server should have received exactly one request")
	require.Equal(t, "test-env-id", receivedPayload.EnvID)
	require.Equal(t, 1, receivedPayload.ModelCount)
	mu.Unlock()

	// Second run: Schema is identical, should NOT push
	err = syncer.RunOnce(context.Background())
	require.NoError(t, err)
	mu.Lock()
	require.Equal(t, 1, serverRequests, "Server should not have received a second request")
	mu.Unlock()
}
