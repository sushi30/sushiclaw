package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const browserbaseEndpoint = "https://api.browserbase.com/v1/search"

type browserbaseProvider struct {
	apiKey string
}

func (b *browserbaseProvider) Name() string { return "browserbase" }

func (b *browserbaseProvider) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	if maxResults > 25 {
		maxResults = 25
	}
	body := map[string]any{
		"query":      query,
		"numResults": maxResults,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, browserbaseEndpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-bb-api-key", b.apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := doRequest(client, req)
	if err != nil {
		return nil, fmt.Errorf("browserbase search failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		// continue
	case http.StatusForbidden:
		return nil, fmt.Errorf("browserbase search returned 403: search API not enabled for this project")
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("browserbase search returned 429: rate limit exceeded")
	default:
		return nil, fmt.Errorf("browserbase search returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("browserbase search decode error: %w", err)
	}

	var out []Result
	for _, r := range payload.Results {
		out = append(out, Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: "",
		})
	}
	return out, nil
}
