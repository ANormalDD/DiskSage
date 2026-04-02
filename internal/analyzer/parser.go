package analyzer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"disksage/internal/models"
)

func ParseRecommendations(raw string) ([]models.Recommendation, error) {
	payload := preprocessModelText(raw)

	if payload == "" {
		return nil, fmt.Errorf("empty recommendation payload")
	}

	candidate := payload
	if extracted, ok := extractJSONCandidate(payload); ok {
		candidate = extracted
	}

	var list []models.Recommendation
	if err := json.Unmarshal([]byte(candidate), &list); err != nil {
		var wrapped struct {
			Recommendations []models.Recommendation `json:"recommendations"`
		}
		if err2 := json.Unmarshal([]byte(candidate), &wrapped); err2 != nil {
			return nil, fmt.Errorf("parse recommendations failed: %w", err)
		}
		list = wrapped.Recommendations
	}
	return sanitizeRecommendations(list), nil
}

func preprocessModelText(raw string) string {
	payload := strings.TrimSpace(raw)
	payload = strings.TrimPrefix(payload, "```json")
	payload = strings.TrimPrefix(payload, "```")
	payload = strings.TrimSuffix(payload, "```")
	payload = strings.TrimSpace(payload)

	thinkTag := regexp.MustCompile(`(?s)<think>.*?</think>`)
	payload = thinkTag.ReplaceAllString(payload, "")

	return strings.TrimSpace(payload)
}

func extractJSONCandidate(text string) (string, bool) {
	runes := []rune(text)
	start := -1
	for i, r := range runes {
		if r == '[' || r == '{' {
			start = i
			break
		}
	}
	if start == -1 {
		return "", false
	}

	for end := len(runes); end > start; end-- {
		candidate := strings.TrimSpace(string(runes[start:end]))
		if !strings.HasSuffix(candidate, "]") && !strings.HasSuffix(candidate, "}") {
			continue
		}
		var anyJSON any
		if json.Unmarshal([]byte(candidate), &anyJSON) == nil {
			return candidate, true
		}
	}

	return "", false
}

func sanitizeRecommendations(in []models.Recommendation) []models.Recommendation {
	seen := make(map[string]struct{}, len(in))
	out := make([]models.Recommendation, 0, len(in))
	for _, item := range in {
		if item.Path == "" {
			continue
		}
		if _, ok := seen[item.Path]; ok {
			continue
		}
		if item.Size < 0 {
			item.Size = 0
		}
		if item.CleanMethod == "" {
			item.CleanMethod = models.MethodRecycle
		}
		if item.Category == "" {
			item.Category = models.CategoryReview
		}
		seen[item.Path] = struct{}{}
		out = append(out, item)
	}
	return out
}
