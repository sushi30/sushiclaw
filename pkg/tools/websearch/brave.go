package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const braveEndpoint = "https://api.search.brave.com/res/v1/web/search"

type braveProvider struct {
	apiKey string
}

func (b *braveProvider) Name() string { return "brave" }

func (b *braveProvider) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	if maxResults > 20 {
		maxResults = 20
	}
	u, err := url.Parse(braveEndpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("count", fmt.Sprintf("%d", maxResults))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Subscription-Token", b.apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := doRequest(client, req)
	if err != nil {
		return nil, fmt.Errorf("brave search failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brave search returned HTTP %d", resp.StatusCode)
	}

	var body struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("brave search decode error: %w", err)
	}

	var out []Result
	for _, r := range body.Web.Results {
		out = append(out, Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}
	return out, nil
}
