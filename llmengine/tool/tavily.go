package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultTavilyEndpoint = "https://api.tavily.com/search"
	defaultSearchTimeout  = 15 * time.Second
)

type TavilySearchTool struct {
	apiKey   string
	client   *http.Client
	endpoint string
}

func NewTavilySearchTool(apiKey string) (*TavilySearchTool, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("tavily API key is required")
	}
	return &TavilySearchTool{
		apiKey:   apiKey,
		client:   &http.Client{Timeout: defaultSearchTimeout},
		endpoint: defaultTavilyEndpoint,
	}, nil
}

func (t *TavilySearchTool) Name() string {
	return "web_search"
}

func (t *TavilySearchTool) Description() string {
	return "Searches the live web and returns concise results with titles, URLs, and content snippets."
}

func (t *TavilySearchTool) Call(ctx context.Context, input string) (string, error) {
	query := strings.TrimSpace(input)
	if query == "" {
		return "", errors.New("search query is required")
	}

	body, err := json.Marshal(tavilySearchRequest{
		Query:         query,
		SearchDepth:   "basic",
		MaxResults:    5,
		IncludeAnswer: true,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("tavily search returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed tavilySearchResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	return formatTavilySearchResponse(parsed), nil
}

type tavilySearchRequest struct {
	Query         string `json:"query"`
	SearchDepth   string `json:"search_depth"`
	MaxResults    int    `json:"max_results"`
	IncludeAnswer bool   `json:"include_answer"`
}

type tavilySearchResponse struct {
	Answer  string               `json:"answer"`
	Results []tavilySearchResult `json:"results"`
}

type tavilySearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

func formatTavilySearchResponse(resp tavilySearchResponse) string {
	var b strings.Builder
	if strings.TrimSpace(resp.Answer) != "" {
		b.WriteString("Answer: ")
		b.WriteString(strings.TrimSpace(resp.Answer))
		b.WriteString("\n\n")
	}
	if len(resp.Results) == 0 {
		b.WriteString("No web results returned.")
		return b.String()
	}
	b.WriteString("Results:\n")
	for i, result := range resp.Results {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, strings.TrimSpace(result.Title)))
		b.WriteString("URL: ")
		b.WriteString(strings.TrimSpace(result.URL))
		b.WriteByte('\n')
		if strings.TrimSpace(result.Content) != "" {
			b.WriteString("Content: ")
			b.WriteString(strings.TrimSpace(result.Content))
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("Score: %.4f\n\n", result.Score))
	}
	return strings.TrimSpace(b.String())
}
