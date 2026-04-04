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

// extractHTMLEnvelope extracts HTML from a <htmlcontent><![CDATA[...]]></htmlcontent> envelope.
// Returns the trimmed inner content, or "" if the envelope is not present.
func extractHTMLEnvelope(s string) string {
	const open = "<htmlcontent>"
	const close = "</htmlcontent>"
	si := strings.Index(s, open)
	if si < 0 {
		return ""
	}
	si += len(open)
	ei := strings.LastIndex(s, close)
	if ei <= si {
		return ""
	}
	content := strings.TrimSpace(s[si:ei])
	// Strip CDATA wrapper if present
	const cdataStart = "<![CDATA["
	const cdataEnd = "]]>"
	if strings.HasPrefix(content, cdataStart) && strings.HasSuffix(content, cdataEnd) {
		content = content[len(cdataStart) : len(content)-len(cdataEnd)]
	}
	return strings.TrimSpace(content)
}

// extractEmbeddedHTML finds the first block-level HTML opening tag in s and
// returns s from that position onward (trimmed). This handles Ollama model
// responses that prefix HTML with explanatory text such as
// "Here is the component:\n\n<div class=\"card\">...".
// Returns "" if no qualifying tag is found.
func extractEmbeddedHTML(s string) string {
	blockTags := []string{
		"<div", "<section", "<nav", "<main", "<article",
		"<header", "<footer", "<ul", "<ol", "<table", "<form", "<aside",
	}
	earliest := -1
	for _, tag := range blockTags {
		if idx := strings.Index(s, tag); idx >= 0 && (earliest < 0 || idx < earliest) {
			earliest = idx
		}
	}
	if earliest < 0 {
		return ""
	}
	return strings.TrimSpace(s[earliest:])
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
