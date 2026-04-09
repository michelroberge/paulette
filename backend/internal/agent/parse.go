package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// XML envelope tag names used in all Claude responses.
const (
	TagDiscussion = "discussion"
	TagArtifact   = "artifact"
	TagHTML       = "htmlcontent"
	TagJSON       = "jsonplan"
)

// Envelope markers used by all Claude response prompts.
const (
	markerStart = "<!-- RESPONSE:START -->"
	markerEnd   = "<!-- RESPONSE:END -->"
)

// maxTagLen is the length of the longest structural marker we need to buffer.
// <!-- RESPONSE:START --> = 23 chars
const maxTagLen = 23

// ParsedResponse holds all extracted sections from a Claude XML envelope.
type ParsedResponse struct {
	Discussion string // <discussion> content
	Artifact   string // <artifact> content
	HTML       string // <htmlcontent> CDATA content
	JSON       []byte // <jsonplan> content as bytes
}

// StripThinkBlocks removes all <think>...</think> blocks produced by reasoning
// models (qwen3, deepseek-r1, etc.) before any parsing occurs. The removal is
// case-insensitive on the tag name. Nested blocks are handled by repeated passes.
// An unclosed opening tag causes everything from that tag to the end to be dropped.
func StripThinkBlocks(s string) string {
	const open = "<think>"
	const close = "</think>"
	for {
		lo := strings.ToLower(s)
		si := strings.Index(lo, open)
		if si < 0 {
			return s
		}
		rest := lo[si:]
		ei := strings.Index(rest, close)
		if ei < 0 {
			// Unclosed block — drop everything from the opening tag onward.
			return strings.TrimSpace(s[:si])
		}
		s = s[:si] + s[si+ei+len(close):]
	}
}

// ParseResponse extracts all sections from a model response.
// It tries the XML envelope format first (<discussion>, <artifact>, etc.).
// If no XML tags are found it falls back to the plain "## Discussion / ## Artifact"
// section-header format used by non-XML-friendly Ollama models.
// As a last resort, if the response looks like a standalone document (starts with a
// markdown heading and is substantial), it is returned as the Artifact so that
// models like codellama that ignore section-header instructions still produce a
// saved artifact rather than dumping everything into the chat discussion.
func ParseResponse(raw string) ParsedResponse {
	raw = StripThinkBlocks(raw)
	hasStartMarker := strings.Contains(raw, markerStart)
	hasArtTag      := strings.Contains(raw, "<"+TagArtifact+">")
	hasDiscTag     := strings.Contains(raw, "<"+TagDiscussion+">")
	hasJSONTag     := strings.Contains(raw, "<"+TagJSON+">")

	// Require either (a) properly paired open+close tags, or (b) the envelope
	// start marker combined with any structural tag. Prevents false positives
	// when small models (e.g. llama3.2:1b) use <artifact> as an inline prose
	// marker — those outputs have no closing tag and no envelope marker, so
	// neither condition fires and the response falls through to the
	// looksLikeDocument / extractDocumentAfterPreamble fallbacks.
	properPairedTags := (hasArtTag  && strings.Contains(raw, "</"+TagArtifact+">")) ||
		(hasDiscTag && strings.Contains(raw, "</"+TagDiscussion+">")) ||
		hasJSONTag
	envelopedTags := hasStartMarker && (hasArtTag || hasDiscTag || hasJSONTag)

	if properPairedTags || envelopedTags {
		return parseXMLEnvelope(raw)
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "## discussion") || strings.Contains(lower, "## artifact") {
		return parsePlainSections(raw)
	}
	// No known structure — check if this looks like a document (artifact).
	// Models that ignore section-header formatting instructions often output the
	// document directly, starting with a markdown heading. Treat those as artifacts
	// so the file gets saved. Short/conversational responses remain as discussion.
	trimmed := strings.TrimSpace(raw)
	if looksLikeDocument(trimmed) {
		return ParsedResponse{Artifact: trimmed}
	}
	// Check for a short preamble before the actual document (common with small models
	// that add "Sure! Here's the regenerated artifact:" before the markdown content).
	if artifact, preamble, ok := extractDocumentAfterPreamble(trimmed); ok {
		return ParsedResponse{Discussion: preamble, Artifact: artifact}
	}
	return ParsedResponse{Discussion: trimmed}
}

// extractDocumentAfterPreamble detects a short conversational preamble (< 300 chars)
// followed by a substantial markdown document starting with a heading.
// Returns (artifact, preamble, true) when found.
func extractDocumentAfterPreamble(s string) (string, string, bool) {
	for _, prefix := range []string{"\n# ", "\n## "} {
		idx := strings.Index(s, prefix)
		if idx > 0 && idx < 300 {
			docPart := strings.TrimSpace(s[idx:])
			if len(docPart) >= 200 {
				preamble := strings.TrimSpace(s[:idx])
				return docPart, preamble, true
			}
		}
	}
	return "", "", false
}

// looksLikeDocument reports whether s appears to be a standalone document
// rather than a conversational reply. The heuristic: the text must be
// substantial (≥200 bytes) AND must begin with a markdown heading ("# " or "## ").
// Conversational replies almost never start with a heading; generated documents
// (vision, UX, architecture, build plans) always do.
func looksLikeDocument(s string) bool {
	if len(s) < 200 {
		return false
	}
	return strings.HasPrefix(s, "# ") || strings.HasPrefix(s, "## ")
}

// parseXMLEnvelope extracts sections from the XML envelope format.
func parseXMLEnvelope(raw string) ParsedResponse {
	return ParsedResponse{
		Discussion: extractBetween(raw, "<"+TagDiscussion+">", "</"+TagDiscussion+">"),
		Artifact:   extractBetween(raw, "<"+TagArtifact+">", "</"+TagArtifact+">"),
		HTML:       extractCDATA(extractBetween(raw, "<"+TagHTML+">", "</"+TagHTML+">")),
		JSON:       extractJSONPlan(raw),
	}
}

// parsePlainSections extracts Discussion and Artifact from the plain section-header
// format ("## Discussion\n...\n## Artifact\n...") used by non-XML-friendly models.
func parsePlainSections(raw string) ParsedResponse {
	return ParsedResponse{
		Discussion: extractPlainSection(raw, "## Discussion"),
		Artifact:   extractPlainSection(raw, "## Artifact"),
	}
}

// rePlainHeading matches markdown headings with 1-3 # characters, case-insensitive.
// Captures: group 1 = heading text (lowercased comparison done by caller).
var rePlainHeading = regexp.MustCompile(`(?im)^#{1,3}\s+(.+)$`)

// extractPlainSection returns the content after the given heading keyword up to
// the next markdown heading or end of string. Matching is case-insensitive and
// supports 1-3 # characters (e.g. "## Discussion", "### discussion", "# ARTIFACT").
// Uses the last occurrence so a preamble before the real content is skipped.
func extractPlainSection(raw, heading string) string {
	// Extract just the keyword from the heading pattern (e.g. "## Discussion" → "discussion").
	keyword := strings.ToLower(strings.TrimLeft(heading, "# "))

	// Find all heading positions and pick the last one matching our keyword.
	matches := rePlainHeading.FindAllStringIndex(raw, -1)
	matchIdx := -1
	matchEnd := 0
	for _, m := range matches {
		line := raw[m[0]:m[1]]
		// Extract heading text after the # characters.
		text := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if strings.EqualFold(text, keyword) {
			matchIdx = m[0]
			matchEnd = m[1]
		}
	}
	if matchIdx < 0 {
		return ""
	}

	content := raw[matchEnd:]
	// Strip the newline immediately after the heading.
	content = strings.TrimLeft(content, "\r\n")
	// Stop at the next markdown heading if present.
	if next := rePlainHeading.FindStringIndex(content); next != nil {
		content = content[:next[0]]
	}
	return strings.TrimSpace(content)
}

// extractBetween returns trimmed content between open and close tags.
// Returns "" if the open tag is absent. If the close tag is absent (truncated
// response from token-limited models), returns content from the open tag to end
// of string so that partially generated artifacts are still captured.
func extractBetween(s, open, close string) string {
	si := strings.Index(s, open)
	if si < 0 {
		return ""
	}
	si += len(open)
	ei := strings.LastIndex(s, close)
	if ei <= si {
		// Close tag missing — treat content from open tag to end as the body.
		// This handles token-limited models (e.g. Groq) that truncate mid-artifact.
		trimmed := strings.TrimSpace(s[si:])
		if trimmed == "" {
			return ""
		}
		return trimmed
	}
	return strings.TrimSpace(s[si:ei])
}

// extractCDATA strips <![CDATA[ ... ]]> wrapper if present.
func extractCDATA(s string) string {
	const cdataStart = "<![CDATA["
	const cdataEnd = "]]>"
	if strings.HasPrefix(s, cdataStart) && strings.HasSuffix(s, cdataEnd) {
		return s[len(cdataStart) : len(s)-len(cdataEnd)]
	}
	return s
}

// reCodeFenceJSON matches a ```json ... ``` code fence.
var reCodeFenceJSON = regexp.MustCompile("(?s)```(?:json)?\\s*\\n(.*?)\\n\\s*```")

// extractJSONPlan pulls <jsonplan> content as a byte slice for json.Unmarshal.
// Falls back through multiple strategies for models that ignore envelope instructions:
// 1. <jsonplan> tags (preferred)
// 2. ```json code fence
// 3. First line starting with { or [ through last line ending with } or ]
// 4. Outermost { ... } or [ ... ] in the raw string (most aggressive)
func extractJSONPlan(s string) []byte {
	s = StripThinkBlocks(s)

	// Strategy 1: <jsonplan> tags
	content := extractBetween(s, "<"+TagJSON+">", "</"+TagJSON+">")
	if content != "" {
		return repairJSON([]byte(content))
	}

	// Strategy 2: ```json code fence
	if m := reCodeFenceJSON.FindStringSubmatch(s); len(m) > 1 {
		return repairJSON([]byte(m[1]))
	}

	// Strategy 3: line-based — find first line starting with {/[ and last ending with }/]
	lines := strings.Split(s, "\n")
	startLine, endLine := -1, -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if startLine < 0 && len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			startLine = i
		}
		if len(trimmed) > 0 && (trimmed[len(trimmed)-1] == '}' || trimmed[len(trimmed)-1] == ']') {
			endLine = i
		}
	}
	if startLine >= 0 && endLine >= startLine {
		block := strings.Join(lines[startLine:endLine+1], "\n")
		return repairJSON([]byte(block))
	}

	// Strategy 4: outermost braces (most aggressive fallback)
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return nil
	}
	end := strings.LastIndexAny(s, "}]")
	if end <= start {
		return nil
	}
	return repairJSON([]byte(s[start : end+1]))
}

// streamFilterState tracks which section of the XML envelope is being streamed.
type streamFilterState int

const (
	sfBeforeRoot   streamFilterState = iota
	sfInRoot                         // inside <!-- RESPONSE:START -->, between tags
	sfInDiscussion                   // inside <discussion> — streamable
	sfInArtifact                     // inside <artifact> — streamable
	sfInHTML                         // inside <htmlcontent> — not streamed to UI
	sfInJSON                         // inside <jsonplan> — not streamed to UI
)

// StreamFilter is a stateful filter that strips XML envelope tags from a streaming
// Claude response, emitting only the content sections that should be visible to the user
// (discussion and artifact). Safe for artifact content that contains HTML or XML tags,
// because it only matches the specific structural tag names defined above.
type StreamFilter struct {
	state streamFilterState
	hold  string // buffered tail that may be an incomplete structural tag
}

// Feed processes a new chunk from the Claude stream.
// Returns the portion that should be forwarded to the user as a "chunk" SSE event.
func (f *StreamFilter) Feed(chunk string) string {
	input := f.hold + chunk
	f.hold = ""

	var out strings.Builder
	i := 0

	for i < len(input) {
		if input[i] != '<' {
			if f.streamable() {
				out.WriteByte(input[i])
			}
			i++
			continue
		}

		// We're at a '<'. Try to match a complete structural tag.
		// Find the next '>'.
		rest := input[i:]
		end := strings.IndexByte(rest, '>')
		if end < 0 {
			// No '>' yet. If the tail is short enough it might be an incomplete tag.
			if len(rest) <= maxTagLen {
				f.hold = rest
				break
			}
			// Tail is longer than the longest structural tag — can't be one.
			if f.streamable() {
				out.WriteByte('<')
			}
			i++
			continue
		}

		tag := rest[:end+1] // includes '<' and '>'
		if newState, ok := f.transition(tag); ok {
			f.state = newState
			i += end + 1
		} else {
			// Not a structural tag — emit '<' and step past it.
			if f.streamable() {
				out.WriteByte('<')
			}
			i++
		}
	}

	return out.String()
}

// streamable reports whether the current state should be forwarded to the UI.
func (f *StreamFilter) streamable() bool {
	return f.state == sfInDiscussion || f.state == sfInArtifact
}

// transition checks whether tag is a known structural tag and returns the new state.
func (f *StreamFilter) transition(tag string) (streamFilterState, bool) {
	switch tag {
	case markerStart:
		return sfInRoot, true
	case markerEnd:
		return sfBeforeRoot, true
	case "<" + TagDiscussion + ">":
		return sfInDiscussion, true
	case "</" + TagDiscussion + ">":
		return sfInRoot, true
	case "<" + TagArtifact + ">":
		return sfInArtifact, true
	case "</" + TagArtifact + ">":
		return sfInRoot, true
	case "<" + TagHTML + ">":
		return sfInHTML, true
	case "</" + TagHTML + ">":
		return sfInRoot, true
	case "<" + TagJSON + ">":
		return sfInJSON, true
	case "</" + TagJSON + ">":
		return sfInRoot, true
	}
	return 0, false
}

// reInsertCommaAfterValue matches a value (string, number, bool, null, }, ])
// followed by whitespace/newline then a quote — missing comma before next key.
var reInsertCommaAfterValue = regexp.MustCompile(`("|true|false|null|\d|[}\]])\s*\n\s*"`)

// reInsertCommaObjects matches adjacent objects in an array: } { → }, {
var reInsertCommaObjects = regexp.MustCompile(`\}\s*\{`)

func repairJSON(input []byte) []byte {
	s := string(input)

	// 1. Trim obvious junk before/after JSON
	start := strings.IndexAny(s, "{[")
	end := strings.LastIndexAny(s, "}]")
	if start >= 0 && end > start {
		s = s[start : end+1]
	}

	// 2. Remove trailing commas
	s = regexp.MustCompile(`,(\s*[}\]])`).ReplaceAllString(s, "$1")

	// 3. Insert missing commas between fields.
	// Handles: "value"\n"key" → "value",\n"key"
	s = reInsertCommaAfterValue.ReplaceAllStringFunc(s, func(m string) string {
		// Find the split point: insert comma after the value token.
		for i := len(m) - 1; i >= 0; i-- {
			if m[i] == '"' && (i == 0 || m[i-1] != '\\') {
				return m[:i] + "," + m[i:]
			}
		}
		return m
	})

	// 4. Insert missing commas between adjacent objects: }{ → },{
	s = reInsertCommaObjects.ReplaceAllString(s, "},{")

	// 5. Attempt to close braces/brackets
	openBraces := strings.Count(s, "{")
	closeBraces := strings.Count(s, "}")
	for closeBraces < openBraces {
		s += "}"
		closeBraces++
	}

	openBrackets := strings.Count(s, "[")
	closeBrackets := strings.Count(s, "]")
	for closeBrackets < openBrackets {
		s += "]"
		closeBrackets++
	}

	return []byte(s)
}

// validateJSON checks data against a minimal JSON Schema subset.
// Supported keywords: type, required, properties, items.
func validateJSON(data []byte, schema []byte) error {
	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var sch map[string]interface{}
	if err := json.Unmarshal(schema, &sch); err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}
	return validateValue(value, sch, "$")
}

func validateValue(value interface{}, schema map[string]interface{}, path string) error {
	if typ, ok := schema["type"].(string); ok {
		if err := checkJSONType(value, typ, path); err != nil {
			return err
		}
	}
	if obj, ok := value.(map[string]interface{}); ok {
		if err := validateRequired(obj, schema, path); err != nil {
			return err
		}
		if err := validateProperties(obj, schema, path); err != nil {
			return err
		}
	}
	if arr, ok := value.([]interface{}); ok {
		return validateItems(arr, schema, path)
	}
	return nil
}

func validateRequired(obj map[string]interface{}, schema map[string]interface{}, path string) error {
	required, ok := schema["required"].([]interface{})
	if !ok {
		return nil
	}
	for _, r := range required {
		key, _ := r.(string)
		if _, exists := obj[key]; !exists {
			return fmt.Errorf("%s: missing required field %q", path, key)
		}
	}
	return nil
}

func validateProperties(obj map[string]interface{}, schema map[string]interface{}, path string) error {
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return nil
	}
	for key, propSchema := range props {
		val, exists := obj[key]
		if !exists {
			continue
		}
		ps, ok := propSchema.(map[string]interface{})
		if !ok {
			continue
		}
		if err := validateValue(val, ps, path+"."+key); err != nil {
			return err
		}
	}
	return nil
}

func validateItems(arr []interface{}, schema map[string]interface{}, path string) error {
	items, ok := schema["items"].(map[string]interface{})
	if !ok {
		return nil
	}
	for i, item := range arr {
		if err := validateValue(item, items, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func checkJSONType(value interface{}, typ string, path string) error {
	switch typ {
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("%s: expected object, got %T", path, value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("%s: expected array, got %T", path, value)
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s: expected string, got %T", path, value)
		}
	case "number", "integer":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("%s: expected number, got %T", path, value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: expected boolean, got %T", path, value)
		}
	}
	return nil
}

// JSONLLMFixer is called when validateJSON fails after repairJSON.
// It should invoke an LLM with the invalid JSON and schema, and return the fixed JSON.
type JSONLLMFixer func(ctx context.Context, invalidJSON []byte, schema []byte) ([]byte, error)

// maxFixerAttempts is the number of times the LLM fixer is called before giving up.
const maxFixerAttempts = 2

// ParseAndValidateJSON extracts <jsonplan> from raw then runs the full pipeline:
//  1. repairJSON (already done inside extractJSONPlan)
//  2. validateJSON against schema
//  3. IF invalid → fixer(ctx, data, schema) up to maxFixerAttempts times
//  4. repairJSON after each fix attempt
//  5. validateJSON after each fix attempt
//  6. IF still invalid after all attempts → return error
//
// If fixer is nil, step 3 is skipped and the first validation error is returned.
func ParseAndValidateJSON(ctx context.Context, raw string, schema []byte, fixer JSONLLMFixer) ([]byte, error) {
	data := extractJSONPlan(raw) // includes repairJSON
	if data == nil {
		return nil, fmt.Errorf("no <jsonplan> found in response")
	}

	if err := validateJSON(data, schema); err == nil {
		return data, nil
	} else if fixer == nil {
		return nil, err
	}

	current := data
	var lastErr error
	for attempt := 0; attempt < maxFixerAttempts; attempt++ {
		fixed, fixErr := fixer(ctx, current, schema)
		if fixErr != nil {
			return nil, fmt.Errorf("LLM fix attempt %d failed: %w", attempt+1, fixErr)
		}

		fixed = repairJSON(fixed)

		if err := validateJSON(fixed, schema); err == nil {
			return fixed, nil
		} else {
			lastErr = err
			current = fixed // feed the partially-fixed result back
		}
	}

	return nil, fmt.Errorf("JSON invalid after %d LLM fix attempts: %w", maxFixerAttempts, lastErr)
}
