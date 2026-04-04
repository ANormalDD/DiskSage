package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTavilyTimeout = 25 * time.Second

type tavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type tavilySearchResponse struct {
	Answer  string         `json:"answer"`
	Results []tavilyResult `json:"results"`
}

type tavilySearchSummary struct {
	Query      string         `json:"query"`
	Answer     string         `json:"answer,omitempty"`
	TopResults []tavilyResult `json:"top_results"`
}

func runTavilySearch(ctx context.Context, cfg TavilyConfig) (string, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = "https://api.tavily.com"
	}
	base = strings.TrimRight(base, "/")

	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", fmt.Errorf("tavily api key is empty")
	}
	if strings.TrimSpace(cfg.Query) == "" {
		return "", fmt.Errorf("tavily query is empty")
	}

	depth := strings.ToLower(strings.TrimSpace(cfg.SearchDepth))
	if depth == "" {
		depth = "basic"
	}
	if depth != "basic" && depth != "advanced" {
		depth = "basic"
	}

	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 10 {
		maxResults = 10
	}

	payload := map[string]any{
		"api_key":      cfg.APIKey,
		"query":        cfg.Query,
		"search_depth": depth,
		"max_results":  maxResults,
	}
	body, _ := json.Marshal(payload)

	requestCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, defaultTavilyTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, base+"/search", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("tavily search failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed tavilySearchResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("tavily response parse failed: %w", err)
	}

	summary := tavilySearchSummary{
		Query:      cfg.Query,
		Answer:     strings.TrimSpace(parsed.Answer),
		TopResults: make([]tavilyResult, 0, len(parsed.Results)),
	}
	for i, item := range parsed.Results {
		if i >= maxResults {
			break
		}
		summary.TopResults = append(summary.TopResults, tavilyResult{
			Title:   strings.TrimSpace(item.Title),
			URL:     strings.TrimSpace(item.URL),
			Content: strings.TrimSpace(item.Content),
			Score:   item.Score,
		})
	}

	resultBlob, _ := json.MarshalIndent(summary, "", "  ")
	return string(resultBlob), nil
}
