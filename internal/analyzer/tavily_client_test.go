package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"disksage/internal/models"
)

type tavilyRequestPayload struct {
	APIKey      string `json:"api_key"`
	Query       string `json:"query"`
	SearchDepth string `json:"search_depth"`
	MaxResults  int    `json:"max_results"`
}

func TestRunTavilySearchNormalizesAndSummarizes(t *testing.T) {
	var got tavilyRequestPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		defer r.Body.Close()

		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}

		results := make([]tavilyResult, 0, 12)
		for i := 1; i <= 12; i++ {
			results = append(results, tavilyResult{
				Title:   fmt.Sprintf("  Title %d  ", i),
				URL:     fmt.Sprintf("  https://example.com/%d  ", i),
				Content: fmt.Sprintf("  Content %d  ", i),
				Score:   float64(i) / 10,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tavilySearchResponse{
			Answer:  "  A short answer  ",
			Results: results,
		})
	}))
	defer server.Close()

	out, err := runTavilySearch(context.Background(), TavilyConfig{
		APIKey:      "test-key",
		BaseURL:     server.URL + "/",
		Query:       "what is clash-verge",
		SearchDepth: "INVALID",
		MaxResults:  99,
	})
	if err != nil {
		t.Fatalf("run tavily search failed: %v", err)
	}

	if got.APIKey != "test-key" {
		t.Fatalf("unexpected api_key: %s", got.APIKey)
	}
	if got.Query != "what is clash-verge" {
		t.Fatalf("unexpected query: %s", got.Query)
	}
	if got.SearchDepth != "basic" {
		t.Fatalf("expected normalized search_depth basic, got %s", got.SearchDepth)
	}
	if got.MaxResults != 10 {
		t.Fatalf("expected clamped max_results 10, got %d", got.MaxResults)
	}

	var summary tavilySearchSummary
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("unmarshal summary failed: %v", err)
	}
	if summary.Query != "what is clash-verge" {
		t.Fatalf("unexpected summary query: %s", summary.Query)
	}
	if summary.Answer != "A short answer" {
		t.Fatalf("unexpected summary answer: %q", summary.Answer)
	}
	if len(summary.TopResults) != 10 {
		t.Fatalf("expected 10 summarized results, got %d", len(summary.TopResults))
	}
	if summary.TopResults[0].Title != "Title 1" {
		t.Fatalf("expected title to be trimmed, got %q", summary.TopResults[0].Title)
	}
	if summary.TopResults[0].URL != "https://example.com/1" {
		t.Fatalf("expected url to be trimmed, got %q", summary.TopResults[0].URL)
	}
}

func TestRunTavilySearchValidatesRequiredFields(t *testing.T) {
	_, err := runTavilySearch(context.Background(), TavilyConfig{Query: "hello"})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "api key is empty") {
		t.Fatalf("expected missing api key error, got: %v", err)
	}

	_, err = runTavilySearch(context.Background(), TavilyConfig{APIKey: "x"})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "query is empty") {
		t.Fatalf("expected missing query error, got: %v", err)
	}
}

func TestExecuteToolCallsTavilyErrorIsRecoverable(t *testing.T) {
	calls := []ToolCall{{
		Name:      ToolTavilySearch,
		Arguments: []byte(`{"query":"unknown app folder"}`),
	}}

	results, recs, err := executeToolCalls(context.Background(), calls, nil, nil, models.LLMConfig{}, nil)
	if err != nil {
		t.Fatalf("expected recoverable tavily error, got err: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected no recommendations, got %d", len(recs))
	}
	if len(results) != 1 {
		t.Fatalf("expected one tool result, got %d", len(results))
	}
	if !strings.Contains(results[0], "tavily_search error") {
		t.Fatalf("expected tavily error result, got: %s", results[0])
	}
}
