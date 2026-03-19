package odoo

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// ─── request building ─────────────────────────────────────────────────────────

// call sends an XML-RPC request to endpoint and returns the raw response body.
func (c *Client) call(ctx context.Context, endpoint, method string, params []any) ([]byte, error) {
	body, err := buildRequest(method, params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// buildRequest serializes an XML-RPC method call envelope.
func buildRequest(method string, params []any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0"?><methodCall><methodName>`)
	if err := xml.EscapeText(&buf, []byte(method)); err != nil {
		return nil, fmt.Errorf("escape method name: %w", err)
	}
	buf.WriteString(`</methodName><params>`)
	for _, p := range params {
		buf.WriteString(`<param>`)
		if err := writeValue(&buf, p); err != nil {
			return nil, fmt.Errorf("encode param: %w", err)
		}
		buf.WriteString(`</param>`)
	}
	buf.WriteString(`</params></methodCall>`)
	return buf.Bytes(), nil
}

// writeValue encodes a Go value as an XML-RPC <value> element.
// Supported Go types → XML-RPC types:
//
//	nil              → <nil/>
//	bool             → <boolean>
//	int / int64      → <int>
//	float64          → <double>
//	string           → <string>
//	[]any    → <array>
//	[]string         → <array> of <string>
//	map[string]any   → <struct>
func writeValue(buf *bytes.Buffer, v any) error {
	buf.WriteString(`<value>`)
	if err := writeInner(buf, v); err != nil {
		return err
	}
	buf.WriteString(`</value>`)
	return nil
}

func writeArray[T any](buf *bytes.Buffer, v []T) error {
	buf.WriteString(`<array><data>`)
	for _, elem := range v {
		if err := writeValue(buf, elem); err != nil {
			return err
		}
	}
	buf.WriteString(`</data></array>`)
	return nil
}

func writeStruct(buf *bytes.Buffer, v map[string]any) error {
	buf.WriteString(`<struct>`)
	for k, val := range v {
		buf.WriteString(`<member><name>`)
		xml.EscapeText(buf, []byte(k)) //nolint:errcheck // writing to bytes.Buffer never fails
		buf.WriteString(`</name>`)
		if err := writeValue(buf, val); err != nil {
			return err
		}
		buf.WriteString(`</member>`)
	}
	buf.WriteString(`</struct>`)
	return nil
}

func writeInner(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString(`<nil/>`)

	case bool:
		if t {
			buf.WriteString(`<boolean>1</boolean>`)
		} else {
			buf.WriteString(`<boolean>0</boolean>`)
		}

	case int:
		fmt.Fprintf(buf, `<int>%d</int>`, t)

	case int64:
		fmt.Fprintf(buf, `<int>%d</int>`, t)

	case float64:
		fmt.Fprintf(buf, `<double>%g</double>`, t)

	case string:
		buf.WriteString(`<string>`)
		xml.EscapeText(buf, []byte(t)) //nolint:errcheck // writing to bytes.Buffer never fails
		buf.WriteString(`</string>`)

	case []any:
		return writeArray(buf, t)

	case []string:
		return writeArray(buf, t)

	case map[string]any:
		return writeStruct(buf, t)

	default:
		return fmt.Errorf("xmlrpc: unsupported type %T", v)
	}
	return nil
}

// ─── response parsing ─────────────────────────────────────────────────────────

// parseUID extracts the authenticated user ID from an XML-RPC /common authenticate response.
//
// Odoo returns:
//   - <int>N</int>   on success  (N = UID, always > 0)
//   - <boolean>0</boolean>  on wrong credentials
//   - <fault>…</fault>      on server errors
func parseUID(data []byte) (int, error) {
	if bytes.Contains(data, []byte("<fault>")) {
		return 0, fmt.Errorf("odoo fault: %s", extractFaultString(data))
	}

	type respValue struct {
		Int     string `xml:"int"`
		I4      string `xml:"i4"`
		Boolean string `xml:"boolean"`
	}
	type methodResp struct {
		XMLName xml.Name  `xml:"methodResponse"`
		Value   respValue `xml:"params>param>value"`
	}

	var r methodResp
	if err := xml.Unmarshal(data, &r); err != nil {
		return 0, fmt.Errorf("parse auth response: %w", err)
	}

	// Odoo returns boolean 0 when credentials are wrong.
	if r.Value.Boolean == "0" {
		return 0, fmt.Errorf("authentication failed: invalid credentials or API key")
	}

	uidStr := strings.TrimSpace(r.Value.Int)
	if uidStr == "" {
		uidStr = strings.TrimSpace(r.Value.I4)
	}
	if uidStr == "" {
		return 0, fmt.Errorf("unexpected auth response: no UID found in: %s", data)
	}

	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return 0, fmt.Errorf("parse UID %q: %w", uidStr, err)
	}
	if uid <= 0 {
		return 0, fmt.Errorf("authentication failed: UID must be > 0, got %d", uid)
	}
	return uid, nil
}

// extractFaultString pulls the faultString member out of an XML-RPC fault response.
func extractFaultString(data []byte) string {
	type member struct {
		Name  string `xml:"name"`
		Value struct {
			String string `xml:"string"`
		} `xml:"value"`
	}
	type methodResp struct {
		XMLName xml.Name `xml:"methodResponse"`
		Members []member `xml:"fault>value>struct>member"`
	}

	var r methodResp
	if err := xml.Unmarshal(data, &r); err != nil {
		return string(data)
	}
	for _, m := range r.Members {
		if m.Name == "faultString" {
			return m.Value.String
		}
	}
	return "unknown fault"
}

// ─── public API ───────────────────────────────────────────────────────────────

// Version calls /xmlrpc/2/common → version() and returns the server_version
// string (e.g. "17.0").
func (c *Client) Version(ctx context.Context) (string, error) {
	endpoint := fmt.Sprintf("%s/xmlrpc/2/common", c.URL.String())

	data, err := c.call(ctx, endpoint, "version", nil)
	if err != nil {
		return "", fmt.Errorf("version: %w", err)
	}

	// The response is a struct with a "server_version" string member.
	type member struct {
		Name  string `xml:"name"`
		Value struct {
			String string `xml:"string"`
		} `xml:"value"`
	}
	type methodResp struct {
		XMLName xml.Name `xml:"methodResponse"`
		Members []member `xml:"params>param>value>struct>member"`
	}

	var r methodResp
	if err := xml.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("parse version response: %w", err)
	}

	for _, m := range r.Members {
		if m.Name == "server_version" {
			return m.Value.String, nil
		}
	}
	return "", fmt.Errorf("server_version not found in response")
}

// Authenticate calls /xmlrpc/2/common → authenticate and stores the UID on the client.
// Must be called before ExecuteKw.
func (c *Client) Authenticate(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s/xmlrpc/2/common", c.URL.String())

	data, err := c.call(ctx, endpoint, "authenticate", []any{
		c.DB,
		c.Username,
		c.Password,
		map[string]any{}, // context (empty)
	})
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	uid, err := parseUID(data)
	if err != nil {
		return err
	}
	c.UID = uid
	return nil
}

// ExecuteKw calls /xmlrpc/2/object → execute_kw.
//
//   - model:  Odoo model name, e.g. "ir.model"
//   - method: method name, e.g. "search_read"
//   - args:   positional arguments; for search_read this is [domain],
//     e.g. []any{[]any{}} for an empty domain
//   - kwargs: keyword arguments, e.g. map[string]any{"fields": []any{"id","model"}}
//
// Returns the raw XML-RPC response body. Authenticate must be called first.
func (c *Client) ExecuteKw(ctx context.Context, model, method string, args []any, kwargs map[string]any) ([]byte, error) {
	if c.UID == 0 {
		return nil, fmt.Errorf("not authenticated: call Authenticate() first")
	}

	endpoint := fmt.Sprintf("%s/xmlrpc/2/object", c.URL.String())

	kw := map[string]any{}
	if kwargs != nil {
		kw = kwargs
	}

	return c.call(ctx, endpoint, "execute_kw", []any{
		c.DB,
		c.UID,
		c.Password,
		model,
		method,
		args,
		kw,
	})
}
