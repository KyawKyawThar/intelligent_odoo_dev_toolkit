package odoo

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ─── fixture responses ────────────────────────────────────────────────────────

const authSuccessResp = `<?xml version='1.0'?>
<methodResponse>
  <params>
    <param><value><int>2</int></value></param>
  </params>
</methodResponse>`

const authSuccessI4Resp = `<?xml version='1.0'?>
<methodResponse>
  <params>
    <param><value><i4>7</i4></value></param>
  </params>
</methodResponse>`

const authFailureResp = `<?xml version='1.0'?>
<methodResponse>
  <params>
    <param><value><boolean>0</boolean></value></param>
  </params>
</methodResponse>`

const authFaultResp = `<?xml version='1.0'?>
<methodResponse>
  <fault>
    <value>
      <struct>
        <member><name>faultCode</name><value><int>3</int></value></member>
        <member><name>faultString</name><value><string>Access Denied</string></value></member>
      </struct>
    </value>
  </fault>
</methodResponse>`

const executeKwResp = `<?xml version='1.0'?>
<methodResponse>
  <params>
    <param>
      <value>
        <array><data>
          <value><struct>
            <member><name>id</name><value><int>1</int></value></member>
            <member><name>model</name><value><string>res.partner</string></value></member>
          </struct></value>
        </data></array>
      </value>
    </param>
  </params>
</methodResponse>`

// ─── helpers ──────────────────────────────────────────────────────────────────

// newMockServer returns a test server that dispatches by URL path.
// pathResp maps path → response body.
func newMockServer(t *testing.T, pathResp map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pathResp[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request to %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body)) //nolint:errcheck
	}))
}

func authenticatedClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := NewClient(srv.URL, "testdb", "admin", "apikey123")
	require.NoError(t, err)
	require.NoError(t, c.Authenticate())
	require.Equal(t, 2, c.UID)
	return c
}

// ─── parseUID unit tests ──────────────────────────────────────────────────────

func TestParseUID_Int(t *testing.T) {
	uid, err := parseUID([]byte(authSuccessResp))
	require.NoError(t, err)
	require.Equal(t, 2, uid)
}

func TestParseUID_I4(t *testing.T) {
	uid, err := parseUID([]byte(authSuccessI4Resp))
	require.NoError(t, err)
	require.Equal(t, 7, uid)
}

func TestParseUID_BooleanZero(t *testing.T) {
	_, err := parseUID([]byte(authFailureResp))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid credentials")
}

func TestParseUID_Fault(t *testing.T) {
	_, err := parseUID([]byte(authFaultResp))
	require.Error(t, err)
	require.Contains(t, err.Error(), "Access Denied")
}

func TestParseUID_Garbage(t *testing.T) {
	_, err := parseUID([]byte(`not xml at all`))
	require.Error(t, err)
}

// ─── buildRequest unit tests ──────────────────────────────────────────────────

func TestBuildRequest_ContainsMethodName(t *testing.T) {
	data, err := buildRequest("authenticate", []any{"mydb", "admin", "secret", map[string]any{}})
	require.NoError(t, err)
	require.Contains(t, string(data), "<methodName>authenticate</methodName>")
	require.Contains(t, string(data), "<string>mydb</string>")
	require.Contains(t, string(data), "<string>admin</string>")
}

func TestBuildRequest_XMLEscapesStrings(t *testing.T) {
	data, err := buildRequest("test", []any{"<hello & world>"})
	require.NoError(t, err)
	require.Contains(t, string(data), "&lt;hello &amp; world&gt;")
}

func TestBuildRequest_NestedArray(t *testing.T) {
	// Typical domain arg: [[]]
	data, err := buildRequest("execute_kw", []any{
		[]any{
			[]any{},
		},
	})
	require.NoError(t, err)
	xml := string(data)
	require.Equal(t, 2, strings.Count(xml, "<array>"), "expected two nested <array> elements")
}

func TestBuildRequest_UnsupportedTypeErrors(t *testing.T) {
	_, err := buildRequest("test", []any{struct{ X int }{X: 1}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported type")
}

// ─── writeValue unit tests ────────────────────────────────────────────────────

func TestWriteValue_Types(t *testing.T) {
	cases := []struct {
		name     string
		input    any
		contains string
	}{
		{"nil", nil, "<nil/>"},
		{"bool true", true, "<boolean>1</boolean>"},
		{"bool false", false, "<boolean>0</boolean>"},
		{"int", 42, "<int>42</int>"},
		{"int64", int64(99), "<int>99</int>"},
		{"float64", 3.14, "<double>3.14</double>"},
		{"string", "hello", "<string>hello</string>"},
		{"empty struct", map[string]any{}, "<struct></struct>"},
		{"[]string", []string{"a", "b"}, "<string>a</string>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, writeValue(&buf, tc.input))
			require.Contains(t, buf.String(), tc.contains)
		})
	}
}

// ─── Authenticate integration tests ──────────────────────────────────────────

func TestAuthenticate_Success(t *testing.T) {
	srv := newMockServer(t, map[string]string{
		"/xmlrpc/2/common": authSuccessResp,
	})
	defer srv.Close()

	c, err := NewClient(srv.URL, "testdb", "admin", "apikey123")
	require.NoError(t, err)

	require.NoError(t, c.Authenticate())
	require.Equal(t, 2, c.UID)
}

func TestAuthenticate_InvalidCredentials(t *testing.T) {
	srv := newMockServer(t, map[string]string{
		"/xmlrpc/2/common": authFailureResp,
	})
	defer srv.Close()

	c, err := NewClient(srv.URL, "testdb", "admin", "wrong-key")
	require.NoError(t, err)

	err = c.Authenticate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid credentials")
	require.Equal(t, 0, c.UID, "UID must remain 0 on auth failure")
}

func TestAuthenticate_Fault(t *testing.T) {
	srv := newMockServer(t, map[string]string{
		"/xmlrpc/2/common": authFaultResp,
	})
	defer srv.Close()

	c, err := NewClient(srv.URL, "testdb", "admin", "apikey123")
	require.NoError(t, err)

	err = c.Authenticate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Access Denied")
}

func TestAuthenticate_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "testdb", "admin", "apikey123")
	require.NoError(t, err)

	err = c.Authenticate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}

// ─── ExecuteKw integration tests ─────────────────────────────────────────────

func TestExecuteKw_RequiresAuthentication(t *testing.T) {
	c, err := NewClient("http://localhost:8069", "testdb", "admin", "apikey123")
	require.NoError(t, err)

	_, err = c.ExecuteKw("ir.model", "search_read", []any{[]any{}}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not authenticated")
}

func TestExecuteKw_Success(t *testing.T) {
	srv := newMockServer(t, map[string]string{
		"/xmlrpc/2/common": authSuccessResp,
		"/xmlrpc/2/object": executeKwResp,
	})
	defer srv.Close()

	c := authenticatedClient(t, srv)

	result, err := c.ExecuteKw(
		"ir.model", "search_read",
		[]any{[]any{}}, // empty domain
		map[string]any{
			"fields": []any{"id", "model"},
			"limit":  10,
		},
	)
	require.NoError(t, err)
	require.Contains(t, string(result), "res.partner")
}

func TestExecuteKw_NilKwargsUsesEmptyStruct(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/xmlrpc/2/common" {
			w.Write([]byte(authSuccessResp)) //nolint:errcheck
			return
		}
		capturedBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(executeKwResp)) //nolint:errcheck
	}))
	defer srv.Close()

	c := authenticatedClient(t, srv)
	_, err := c.ExecuteKw("res.partner", "search_read", []any{[]any{}}, nil)
	require.NoError(t, err)
	require.Contains(t, string(capturedBody), "<struct></struct>", "nil kwargs must send empty struct")
}
