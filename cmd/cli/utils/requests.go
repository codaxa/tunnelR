package cliutils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MakeAuthenticatedRequest sends requests along with payload, authorization headers
func MakeAuthenticatedRequest(method, path string, payload interface{}) (*http.Response, error) {
	// Load config to get server and token
	config, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}

	if config.Token == "" {
		return nil, fmt.Errorf("no authentication token found")
	}

	server := config.Server
	if server == "" {
		return nil, fmt.Errorf("no server configured. Please connect to server first")
	}

	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "https://" + server
	}

	serverURL, err := url.Parse(server)
	if err != nil {
		return nil, fmt.Errorf("error parsing server URL: %w", err)
	}

	// Ensure the scheme is set
	host := serverURL.Hostname()
	if serverURL.Scheme == "http" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return nil, fmt.Errorf("refusing to send token over plain HTTP to %q; use https:// or connect to localhost", host)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("error encoding request: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	// Construct the full URL
	p, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("invalid request path: %w", err)
	}
	apiURL := serverURL.ResolveReference(p)

	// Create the request
	req, err := http.NewRequestWithContext(ctx, method, apiURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add headers
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+config.Token)

	// Send the request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	return client.Do(req)
}
