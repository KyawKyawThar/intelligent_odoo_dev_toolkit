package collector

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

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
          <value><struct>
            <member><name>id</name><value><int>2</int></value></member>
            <member><name>model</name><value><string>res.users</string></value></member>
			<member><name>name</name><value><string>Users</string></value></member>
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
          <value><struct>
            <member><name>id</name><value><int>102</int></value></member>
            <member><name>model</name><value><string>res.partner</string></value></member>
            <member><name>name</name><value><string>email</string></value></member>
            <member><name>ttype</name><value><string>char</string></value></member>
          </struct></value>
          <value><struct>
            <member><name>id</name><value><int>201</int></value></member>
            <member><name>model</name><value><string>res.users</string></value></member>
            <member><name>name</name><value><string>login</string></value></member>
            <member><name>ttype</name><value><string>char</string></value></member>
          </struct></value>
        </data></array>
      </value>
    </param>
  </params>
</methodResponse>`

func newMockCollectorServer(t *testing.T) *httptest.Server {
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
			if strings.Contains(body, "ir.model.fields") {
				w.Write([]byte(irModelFieldsResp))
			} else if strings.Contains(body, "ir.model") {
				w.Write([]byte(irModelResp))
			} else {
				http.Error(w, "unexpected model request", http.StatusBadRequest)
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestCollectModels(t *testing.T) {
	srv := newMockCollectorServer(t)
	defer srv.Close()

	client, err := odoo.NewClient(srv.URL, "testdb", "admin", "password")
	require.NoError(t, err)
	err = client.Authenticate()
	require.NoError(t, err)

	models, err := CollectModels(client)
	require.NoError(t, err)
	require.Len(t, models, 2)

	// Check model res.partner
	var partner map[string]any
	for _, m := range models {
		if m["model"] == "res.partner" {
			partner = m
			break
		}
	}
	require.NotNil(t, partner, "model res.partner not found")

	require.Equal(t, "res.partner", partner["model"])
	require.Equal(t, "Partner", partner["name"])
	fields, ok := partner["fields"].([]map[string]any)
	require.True(t, ok, "fields are not of the expected type")
	require.Len(t, fields, 2)
	require.Equal(t, "name", fields[0]["name"])
	require.Equal(t, "email", fields[1]["name"])

	// Check model res.users
	var users map[string]any
	for _, m := range models {
		if m["model"] == "res.users" {
			users = m
			break
		}
	}
	require.NotNil(t, users, "model res.users not found")

	require.Equal(t, "res.users", users["model"])
	require.Equal(t, "Users", users["name"])
	fields, ok = users["fields"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, fields, 1)
	require.Equal(t, "login", fields[0]["name"])
}

func TestCollectAndSendWebSocket(t *testing.T) {
	// ── 1. Mock WebSocket server ──────────────────────────────────────────
	var upgrader = websocket.Upgrader{}
	var receivedData []map[string]any
	var wg sync.WaitGroup
	wg.Add(1)

	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer c.Close()
		defer wg.Done()

		mt, message, err := c.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocket.BinaryMessage, mt)

		// Decompress
		gz, err := gzip.NewReader(bytes.NewReader(message))
		require.NoError(t, err)
		uncompressed, err := io.ReadAll(gz)
		require.NoError(t, err)

		// Deserialize
		err = json.Unmarshal(uncompressed, &receivedData)
		require.NoError(t, err)
	}))
	defer wsSrv.Close()

	// ── 2. Collect data from mock Odoo ──────────────────────────────────────
	odooSrv := newMockCollectorServer(t)
	defer odooSrv.Close()

	client, err := odoo.NewClient(odooSrv.URL, "testdb", "admin", "password")
	require.NoError(t, err)
	err = client.Authenticate()
	require.NoError(t, err)
	models, err := CollectModels(client)
	require.NoError(t, err)

	// ── 3. Serialize + Gzip ─────────────────────────────────────────────────
	jsonData, err := json.Marshal(models)
	require.NoError(t, err)

	var compressedData bytes.Buffer
	gz := gzip.NewWriter(&compressedData)
	_, err = gz.Write(jsonData)
	require.NoError(t, err)
	err = gz.Close()
	require.NoError(t, err)

	// ── 4. Send via WebSocket client ────────────────────────────────────────
	wsURL := "ws" + strings.TrimPrefix(wsSrv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer ws.Close()

	err = ws.WriteMessage(websocket.BinaryMessage, compressedData.Bytes())
	require.NoError(t, err)

	// ── 5. Wait and Assert ──────────────────────────────────────────────────
	// Wait for the server to process the message, with a timeout.
	waitChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitChan)
	}()
	select {
	case <-waitChan:
		// all good
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for websocket server to receive message")
	}

	require.Len(t, receivedData, 2)
	require.Equal(t, "res.partner", receivedData[0]["model"])
	require.Equal(t, "res.users", receivedData[1]["model"])

	fields, ok := receivedData[0]["fields"].([]any)
	require.True(t, ok)
	require.Len(t, fields, 2)
}
