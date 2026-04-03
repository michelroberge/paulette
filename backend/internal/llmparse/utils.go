package llmparse

import (
	"encoding/json"
	"strings"
)

// mapToTarget marshals input → JSON → target struct
func mapToTarget(input any, target any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// extractJSON attempts to find JSON between first {/[ and last }/]
func extractJSON(s string) string {
	start := strings.IndexAny(s, "{[")
	end := strings.LastIndexAny(s, "}]")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}

// extractCodeBlocks extracts text inside ``` code fences
func extractCodeBlocks(s string) []string {
	var results []string
	parts := strings.Split(s, "```")
	for i := 1; i < len(parts); i += 2 {
		block := strings.TrimSpace(parts[i])
		if idx := strings.Index(block, "\n"); idx != -1 {
			block = block[idx+1:]
		}
		results = append(results, block)
	}
	return results
}
