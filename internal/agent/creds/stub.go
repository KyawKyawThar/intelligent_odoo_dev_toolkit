package creds

import "fmt"

// Stub is a simple Provider for use in tests.
// It returns a fixed API key and its RefreshOnUnauthorized always fails.
type Stub struct {
	Key string
}

func (s *Stub) APIKey() string { return s.Key }

func (s *Stub) RefreshOnUnauthorized() (string, error) {
	return "", fmt.Errorf("stub: refresh not supported")
}
