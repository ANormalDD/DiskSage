package analyzer

import (
	"testing"

	"disksage/internal/models"
)

func TestBuildToolDefinitionsWithoutSearchTool(t *testing.T) {
	cfg := models.LLMConfig{
		EnableWebSearch: false,
		TavilyAPIKey:    "test-key",
	}

	tools := BuildToolDefinitions(cfg)
	for _, tool := range tools {
		if tool.Name == ToolTavilySearch {
			t.Fatalf("did not expect %s when search is disabled", ToolTavilySearch)
		}
	}
}

func TestBuildToolDefinitionsWithSearchTool(t *testing.T) {
	cfg := models.LLMConfig{
		EnableWebSearch: true,
		TavilyAPIKey:    "test-key",
	}

	tools := BuildToolDefinitions(cfg)
	found := false
	for _, tool := range tools {
		if tool.Name == ToolTavilySearch {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s when search is enabled", ToolTavilySearch)
	}
}
