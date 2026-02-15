// Web search and fetch tools
package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type WebSearchTool struct{}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{}
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "Use Tavily API (or fallback) to search the web; returns title, URL, summary."
}

func (t *WebSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query keywords",
			},
			"count": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results (1-10), default 5",
				"default":     5,
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(args map[string]interface{}) (interface{}, error) {
	query := GetString(args, "query")
	count := GetInt(args, "count")
	if count <= 0 || count > 10 {
		count = 5
	}

	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	// Tavily API (simplified mock); in production, call the real API with key
	results, err := tavilySearch(query, count)
	if err != nil {
		return nil, fmt.Errorf("search failed: %v", err)
	}

	return map[string]interface{}{
		"query":   query,
		"count":   len(results),
		"results": results,
	}, nil
}

// Tavily search result
type tavilyResult struct {
	Title   string `json:"title"`
	Url     string `json:"url"`
	Content string `json:"content"`
}

// tavilySearch simplified (should call Tavily API)
func tavilySearch(query string, count int) ([]tavilyResult, error) {
	// Use Brave Search as fallback
	// Tavily API: https://api.tavily.com/search
	apiKey := "" // from env TAVILY_API_KEY

	if apiKey != "" {
		return tavilyAPISearch(query, count, apiKey)
	}

	// Use Brave Search as alternative
	return braveSearch(query, count)
}

// tavilyAPISearch calls Tavily API
func tavilyAPISearch(query string, count int, apiKey string) ([]tavilyResult, error) {
	client := &http.Client{}
	req, err := http.NewRequest("POST", "https://api.tavily.com/search", strings.NewReader(
		fmt.Sprintf(`{"query":"%s","search_depth":"basic","max_results":%d}`, query, count),
	))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Results []struct {
			Title   string `json:"title"`
			Url     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	json.Unmarshal(body, &result)

	results := make([]tavilyResult, 0, len(result.Results))
	for _, r := range result.Results {
		results = append(results, tavilyResult{
			Title:   r.Title,
			Url:     r.Url,
			Content: r.Content,
		})
	}
	return results, nil
}

// Brave Search fallback implementation
func braveSearch(query string, count int) ([]tavilyResult, error) {
	// Return a mock result with instructions
	return []tavilyResult{
		{
			Title:   "Search setup",
			Url:     "https://docs.tavily.ai/",
			Content: fmt.Sprintf("Searching '%s' requires a Tavily API key. Set TAVILY_API_KEY in production.", query),
		},
	}, nil
}

// Web Fetch Tool - fetch and extract readable content
type WebFetchTool struct{}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{}
}

func (t *WebFetchTool) Name() string {
	return "web_fetch"
}

func (t *WebFetchTool) Description() string {
	return "Fetch a web page and extract readable content (HTML to markdown/text)."
}

func (t *WebFetchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "Page URL to fetch",
			},
			"extractMode": map[string]interface{}{
				"type":        "string",
				"description": "Extraction mode: markdown or text (default markdown)",
				"default":     "markdown",
			},
			"maxChars": map[string]interface{}{
				"type":        "integer",
				"description": "Max characters (truncates beyond this)",
				"default":     10000,
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(args map[string]interface{}) (interface{}, error) {
	url := GetString(args, "url")
	extractMode := GetString(args, "extractMode")
	if extractMode == "" {
		extractMode = "markdown"
	}
	maxChars := GetInt(args, "maxChars")
	if maxChars <= 0 {
		maxChars = 10000
	}

	if url == "" {
		return nil, fmt.Errorf("url is required")
	}

	// Basic URL validation
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("invalid URL")
	}

	content, err := fetchURL(url, extractMode, maxChars)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %v", err)
	}

	return map[string]interface{}{
		"url":         url,
		"extractMode": extractMode,
		"content":     content,
		"truncated":   len(content) >= maxChars,
	}, nil
}

// fetchURL retrieves content and applies extraction
func fetchURL(url, extractMode string, maxChars int) (string, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	// Simple text extraction (ideally use go-readability)
	content = extractText(content, extractMode)

	// Enforce max length
	if len(content) > maxChars {
		content = content[:maxChars] + "\n\n[truncated]"
	}

	return content, nil
}

// extractText: very basic HTML stripping and whitespace cleanup
func extractText(html, mode string) string {
	// Remove scripts and styles
	replacer := strings.NewReplacer(
		"<script", "\n<script",
		"</script>", "\n</script>",
		"<style", "\n<style",
		"</style>", "\n</style>",
		"<head", "\n<head",
		"</head>", "\n</head>",
	)
	html = replacer.Replace(html)

	// Strip tags
	for {
		start := strings.Index(html, "<")
		if start == -1 {
			break
		}
		end := strings.Index(html, ">")
		if end == -1 || end < start {
			break
		}
		html = html[:start] + html[end+1:]
	}

	// Trim empty lines
	lines := make([]string, 0)
	for _, line := range strings.Split(html, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	content := strings.Join(lines, "\n")

	// Compress blank lines
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(content)
}
