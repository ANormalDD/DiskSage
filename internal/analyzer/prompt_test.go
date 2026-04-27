package analyzer

import (
	"strings"
	"testing"

	"disksage/internal/models"
)

func TestBuildPromptRequiresIntermediateStateOutsideReasoning(t *testing.T) {
	system, _ := BuildPrompt("tree", models.LLMConfig{})
	if !strings.Contains(system, "不要把分类、候选列表或阶段性结论只放在 reasoning 中") {
		t.Fatalf("expected system prompt to require intermediate state outside reasoning, got: %s", system)
	}
}
