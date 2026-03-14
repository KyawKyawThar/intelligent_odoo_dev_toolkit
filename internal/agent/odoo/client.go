package odoo

import (
	"fmt"
	"net/http"
	"net/url"
)

// Client represents a client for the Odoo XML-RPC API.
type Client struct {
	URL      *url.URL
	DB       string
	Username string
	// Password holds the Odoo user password or API key — both are passed in
	// the same position in XML-RPC authenticate / execute_kw calls.
	Password string
	UID      int
	client   *http.Client
}

// NewClient creates a new Odoo XML-RPC client.
func NewClient(rawURL, db, username, password string) (*Client, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Odoo URL: %w", err)
	}

	return &Client{
		URL:      parsedURL,
		DB:       db,
		Username: username,
		Password: password,
		client:   &http.Client{},
	}, nil
}
