package analyzer

import "encoding/json"

const (
	ToolScanDeeper            = "scan_deeper"
	ToolCheckDirContent       = "check_dir_content"
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
			Description: "Get file type distribution under a directory",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
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
