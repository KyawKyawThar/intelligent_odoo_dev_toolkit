// Package creds defines the Provider interface used by agent components
// to obtain the current API key and trigger automatic re-registration
// when the server returns 401 Unauthorized.
package creds

// Provider gives agent components access to the current API key and
// lets them trigger credential refresh on authentication failure.
type Provider interface {
	// APIKey returns the current API key.
	APIKey() string

	// RefreshOnUnauthorized attempts to re-register with the cloud server
	// and returns the new API key. Components should call this when they
	// receive an HTTP 401 and then retry the request with the new key.
	// It is safe to call from multiple goroutines — only one refresh runs
	// at a time.
	RefreshOnUnauthorized() (string, error)
}
