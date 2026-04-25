package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/config"
)

const defaultMaxResults = 10

// Provider is the interface for search backends.
type Provider interface {
	Name() string
	Search(ctx context.Context, query string, maxResults int) ([]Result, error)
}

// Result is a single search result.
type Result struct {
	Title   string
	URL     string
	Snippet string
}

// WebSearchTool satisfies agent-sdk-go's Tool interface.
type WebSearchTool struct {
	provider   Provider
	maxResults int
}

// queryArgs is the expected JSON shape from the agent.
type queryArgs struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// NewTool creates a WebSearchTool from config.
func NewTool(cfg config.WebSearchToolConfig) (interfaces.Tool, error) {
	p, err := buildProvider(cfg)
	if err != nil {
		return nil, err
	}
	max := cfg.MaxResults
	if max <= 0 {
		max = defaultMaxResults
	}
	return &WebSearchTool{provider: p, maxResults: max}, nil
}

func buildProvider(cfg config.WebSearchToolConfig) (Provider, error) {
	switch cfg.Provider {
	case "brave":
		if !cfg.Brave.Enabled {
			return nil, fmt.Errorf("brave search provider is not enabled in config")
		}
		if cfg.Brave.APIKey == nil || cfg.Brave.APIKey.IsZero() || cfg.Brave.APIKey.IsUnresolvedEnv() {
			return nil, fmt.Errorf("brave search requires api_key (set BRAVE_API_KEY env var or config)")
		}
		return &braveProvider{apiKey: cfg.Brave.APIKey.String()}, nil
	case "duckduckgo":
		if !cfg.DuckDuckGo.Enabled {
			return nil, fmt.Errorf("duckduckgo search provider is not enabled in config")
		}
		return &duckDuckGoProvider{}, nil
	case "browserbase":
		if !cfg.Browserbase.Enabled {
			return nil, fmt.Errorf("browserbase search provider is not enabled in config")
		}
		if cfg.Browserbase.APIKey == nil || cfg.Browserbase.APIKey.IsZero() || cfg.Browserbase.APIKey.IsUnresolvedEnv() {
			return nil, fmt.Errorf("browserbase search requires api_key (set BROWSERBASE_API_KEY env var or config)")
		}
		return &browserbaseProvider{apiKey: cfg.Browserbase.APIKey.String()}, nil
	case "tavily":
		return nil, fmt.Errorf("tavily search provider is not yet implemented")
	case "baidu":
		return nil, fmt.Errorf("baidu search provider is not yet implemented")
	case "":
		return nil, fmt.Errorf("web_search provider not configured")
	default:
		return nil, fmt.Errorf("unknown web_search provider: %q", cfg.Provider)
	}
}

func (t *WebSearchTool) Name() string        { return "web_search" }
func (t *WebSearchTool) Description() string { return fmt.Sprintf("Search the web for current information, news, facts, and links using %s. Returns a list of results with titles, URLs, and descriptions.", t.provider.Name()) }

func (t *WebSearchTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: "The search query",
			Required:    true,
		},
		"max_results": {
			Type:        "integer",
			Description: "Maximum number of results to return",
			Required:    false,
			Default:     t.maxResults,
		},
	}
}

// Run executes the tool with the given input string.
func (t *WebSearchTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute executes the tool with the given arguments JSON string.
func (t *WebSearchTool) Execute(ctx context.Context, args string) (string, error) {
	var qa queryArgs
	if err := json.Unmarshal([]byte(args), &qa); err != nil {
		// Fallback: treat the whole input as the query string.
		qa.Query = args
	}
	if qa.Query == "" {
		return "", fmt.Errorf("no query provided")
	}
	max := t.maxResults
	if qa.MaxResults > 0 {
		max = qa.MaxResults
	}

	results, err := t.provider.Search(ctx, qa.Query, max)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "No results found.", nil
	}
	return formatResults(results), nil
}

func formatResults(results []Result) string {
	var out string
	for i, r := range results {
		out += fmt.Sprintf("%d. Title: %s\n   URL: %s\n   Description: %s\n\n", i+1, r.Title, r.URL, r.Snippet)
	}
	return out
}

// retryBackoff is the sleep duration between retries. Overridable for tests.
var retryBackoff = 5 * time.Second

// doRequest performs an HTTP request with naive retry: 3 attempts, fixed backoff.
func doRequest(client *http.Client, req *http.Request) (*http.Response, error) {
	var lastErr error
	for i := range 3 {
		resp, err := client.Do(req)
		if err == nil {
			if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				return resp, nil
			}
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			_ = resp.Body.Close()
		} else {
			lastErr = err
		}
		if i < 2 {
			time.Sleep(retryBackoff)
		}
	}
	return nil, fmt.Errorf("request failed after 3 attempts: %w", lastErr)
}
