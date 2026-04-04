package analyzer

import "encoding/json"

const (
	ToolScanDeeper            = "scan_deeper"
	ToolCheckDirContent       = "check_dir_content"
	ToolTavilySearch          = "tavily_search"
	ToolSubmitRecommendations = "submit_recommendations"
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
	return []ToolDefinition{
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
		{
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
		},
		{
			Name:        ToolSubmitRecommendations,
			Description: "Submit final cleanup recommendations",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"recommendations": map[string]any{"type": "array"},
				},
				"required": []string{"recommendations"},
			},
		},
	}
}
