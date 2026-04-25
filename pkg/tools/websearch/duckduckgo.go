package websearch

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const ddgEndpoint = "https://html.duckduckgo.com/html/"

type duckDuckGoProvider struct{}

func (d *duckDuckGoProvider) Name() string { return "duckduckgo" }

func (d *duckDuckGoProvider) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	u, err := url.Parse(ddgEndpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := doRequest(client, req)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo search failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("duckduckgo search returned HTTP %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo parse error: %w", err)
	}

	var results []Result
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			if hasClass(n, "result") || hasClass(n, "web-result") {
				res := extractResult(n)
				if res.URL != "" {
					results = append(results, res)
				}
				if len(results) >= maxResults {
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
			if len(results) >= maxResults {
				return
			}
		}
	}
	traverse(doc)

	if len(results) == 0 {
		return nil, fmt.Errorf("duckduckgo returned no parseable results (layout may have changed)")
	}
	return results, nil
}

func hasClass(n *html.Node, class string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" && strings.Contains(a.Val, class) {
			return true
		}
	}
	return false
}

func extractResult(n *html.Node) Result {
	var r Result
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "a":
				if hasClass(node, "result__a") {
					for _, a := range node.Attr {
						if a.Key == "href" {
							r.URL = cleanDDGURL(a.Val)
						}
					}
					if r.Title == "" {
						r.Title = textContent(node)
					}
				}
			case "div":
				if hasClass(node, "result__snippet") && r.Snippet == "" {
					r.Snippet = textContent(node)
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return r
}

func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return strings.TrimSpace(n.Data)
	}
	var parts []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if p := textContent(c); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

func cleanDDGURL(raw string) string {
	const prefix = "/l/?kh=-1&uddg="
	if strings.HasPrefix(raw, prefix) {
		if u, err := url.QueryUnescape(raw[len(prefix):]); err == nil {
			return u
		}
	}
	if !strings.HasPrefix(raw, "http") {
		return "https://duckduckgo.com" + raw
	}
	return raw
}
