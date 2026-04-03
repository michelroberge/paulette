package llmparse

import (
	"encoding/json"
	"strings"
)

// -----------------------------
// JSON Strategy
// -----------------------------
type JSONStrategy struct{}

func (j JSONStrategy) Name() string { return "json" }

func (j JSONStrategy) Detect(input string) float64 {
	s := strings.TrimSpace(input)
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return 0.9
	}
	if strings.Contains(s, "{") && strings.Contains(s, "}") {
		return 0.6
	}
	return 0
}

func (j JSONStrategy) Parse(input string, target any) error {
	if err := json.Unmarshal([]byte(input), target); err == nil {
		return nil
	}
	if js := extractJSON(input); js != "" {
		return json.Unmarshal([]byte(js), target)
	}
	return json.Unmarshal([]byte(input), target)
}

// -----------------------------
// List Strategy
// -----------------------------
type ListStrategy struct{}

func (l ListStrategy) Name() string { return "list" }

func (l ListStrategy) Detect(input string) float64 {
	lines := strings.Split(input, "\n")
	score := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			score++
		}
		if len(line) > 2 && line[0] >= '0' && line[0] <= '9' && strings.Contains(line, ".") {
			score++
		}
	}
	if score >= 3 {
		return 0.8
	}
	if score > 0 {
		return 0.4
	}
	return 0
}

func (l ListStrategy) Parse(input string, target any) error {
	var result []string
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			result = append(result, strings.TrimSpace(line[2:]))
			continue
		}
		if idx := strings.Index(line, "."); idx != -1 {
			result = append(result, strings.TrimSpace(line[idx+1:]))
		}
	}
	switch ptr := target.(type) {
	case *[]string:
		*ptr = result
		return nil
	case *any:
		*ptr = result
		return nil
	}
	return ErrTypeMismatch
}

// -----------------------------
// HTML Strategy
// -----------------------------
type HTMLStrategy struct{}

func (h HTMLStrategy) Name() string { return "html" }

func (h HTMLStrategy) Detect(input string) float64 {
	if strings.Contains(input, "<htmlcontent>") {
		return 0.95
	}
	if strings.Contains(input, "```") {
		return 0.85
	}
	s := strings.TrimSpace(input)
	if strings.HasPrefix(s, "<") {
		return 0.6
	}
	return 0
}

func (h HTMLStrategy) Parse(input string, target any) error {
	var result string
	// 1. Try <htmlcontent><![CDATA[...]]></htmlcontent> envelope
	if s := extractHTMLEnvelope(input); s != "" {
		result = s
	} else if blocks := extractCodeBlocks(input); len(blocks) > 0 {
		// 2. Try ``` code fence
		result = blocks[0]
	} else {
		// 3. Raw HTML fallback
		s := strings.TrimSpace(input)
		if strings.HasPrefix(s, "<") {
			result = s
		}
	}
	if result == "" {
		return ErrTypeMismatch
	}
	switch ptr := target.(type) {
	case *string:
		*ptr = result
		return nil
	case *any:
		*ptr = result
		return nil
	}
	return ErrTypeMismatch
}

// -----------------------------
// Code Strategy
// -----------------------------
type CodeStrategy struct{}

func (c CodeStrategy) Name() string { return "code" }

func (c CodeStrategy) Detect(input string) float64 {
	score := 0.0
	if strings.Contains(input, "```") {
		return 0.95
	}
	keywords := []string{"func ", "return", "{", "}", "import ", "package "}
	for _, k := range keywords {
		if strings.Contains(input, k) {
			score += 0.15
		}
	}
	if score > 0.5 {
		return score
	}
	return 0
}

func (c CodeStrategy) Parse(input string, target any) error {
	blocks := extractCodeBlocks(input)
	if len(blocks) == 0 {
		blocks = []string{strings.TrimSpace(input)}
	}
	switch ptr := target.(type) {
	case *[]string:
		*ptr = blocks
		return nil
	case *any:
		*ptr = blocks
		return nil
	}
	return ErrTypeMismatch
}
