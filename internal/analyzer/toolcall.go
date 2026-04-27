package analyzer

import (
	"encoding/json"
	"strings"

	"disksage/internal/models"
)

const (
	ToolScanDeeper            = "scan_deeper"
	ToolCheckDirContent       = "check_dir_content"
	ToolTavilySearch          = "tavily_search"
	ToolSubmitRecommendations = "submit_recommendations"
	ToolFinishAnalysis        = "finish_analysis"
)

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func DefaultToolDefinitions() []ToolDefinition {
	return BuildToolDefinitions(models.DefaultAppConfig().LLM)
}

func IsTavilySearchEnabled(cfg models.LLMConfig) bool {
	if !cfg.EnableWebSearch {
		return false
	}
	return strings.TrimSpace(cfg.TavilyAPIKey) != ""
}

func BuildToolDefinitions(cfg models.LLMConfig) []ToolDefinition {
	tools := []ToolDefinition{
		{
			Name:        ToolScanDeeper,
			Description: "Scan specific directory with deeper depth",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string"},
					"depth": map[string]any{"type": "integer"},
				},
				"required": []string{"path", "depth"},
			},
		},
		{
			Name:        ToolCheckDirContent,
			Description: "Get directory evidence including file type distribution and folder timestamps (CreatedAt, ModifiedAt)",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
			},
		},
	}

	if IsTavilySearchEnabled(cfg) {
		tools = append(tools, ToolDefinition{
			Name:        ToolTavilySearch,
			Description: "Search web knowledge via Tavily for unknown folders/apps to determine whether they are safe to clean",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":        map[string]any{"type": "string"},
					"search_depth": map[string]any{"type": "string", "enum": []string{"basic", "advanced"}},
					"max_results":  map[string]any{"type": "integer"},
				},
				"required": []string{"query"},
			},
		})
	}

	tools = append(tools,
		ToolDefinition{
			Name:        ToolSubmitRecommendations,
			Description: "Incrementally submit a batch of cleanup recommendations. Can be called multiple times during analysis to accumulate results. Does NOT end the analysis.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"recommendations": map[string]any{"type": "array"},
				},
				"required": []string{"recommendations"},
			},
		},
		ToolDefinition{
			Name:        ToolFinishAnalysis,
			Description: "Signal that analysis is complete. Call this after all submit_recommendations calls are done. No parameters required.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	)

	return tools
}
