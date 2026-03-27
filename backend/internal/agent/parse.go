package agent

import (
	"regexp"
	"strings"
)

// XML envelope tag names used in all Claude responses.
const (
	TagResponse   = "response"
	TagDiscussion = "discussion"
	TagArtifact   = "artifact"
	TagHTML       = "htmlcontent"
	TagJSON       = "jsonplan"
)

// maxTagLen is the length of the longest closing tag we need to buffer.
// </discussion> = 13 chars
const maxTagLen = 13

// ParsedResponse holds all extracted sections from a Claude XML envelope.
type ParsedResponse struct {
	Discussion string // <discussion> content
	Artifact   string // <artifact> content
	HTML       string // <htmlcontent> CDATA content
	JSON       []byte // <jsonplan> content as bytes
}

// ParseResponse extracts all sections from a model response.
// It tries the XML envelope format first (<discussion>, <artifact>, etc.).
// If no XML tags are found it falls back to the plain "## Discussion / ## Artifact"
// section-header format used by non-XML-friendly Ollama models.
func ParseResponse(raw string) ParsedResponse {
	if strings.Contains(raw, "<"+TagDiscussion+">") || strings.Contains(raw, "<"+TagArtifact+">") {
		return parseXMLEnvelope(raw)
	}
	if strings.Contains(raw, "## Discussion") || strings.Contains(raw, "## Artifact") {
		return parsePlainSections(raw)
	}
	// No known structure — return everything as discussion.
	return ParsedResponse{Discussion: strings.TrimSpace(raw)}
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

// extractPlainSection returns the content after the given "## Heading" marker up to
// the next "## " heading or end of string. Uses the last occurrence of the heading
// so a preamble before the real content is skipped.
func extractPlainSection(raw, heading string) string {
	idx := strings.LastIndex(raw, heading)
	if idx < 0 {
		return ""
	}
	content := raw[idx+len(heading):]
	// Strip the newline immediately after the heading.
	content = strings.TrimLeft(content, "\r\n")
	// Stop at the next "## " heading if present.
	if next := strings.Index(content, "\n## "); next >= 0 {
		content = content[:next]
	}
	return strings.TrimSpace(content)
}

func extractBetweenMarkers(s, start, end string) string {
	si := strings.Index(s, start)
	if si < 0 {
		return ""
	}
	si += len(start)

	ei := strings.LastIndex(s, end)
	if ei <= si {
		return ""
	}

	return strings.TrimSpace(s[si:ei])
}

// extractBetween returns trimmed content between open and close tags.
// Returns "" if either tag is absent or close precedes open.
func extractBetween(s, open, close string) string {
	si := strings.Index(s, open)
	if si < 0 {
		return ""
	}
	si += len(open)
	ei := strings.LastIndex(s, close)
	if ei <= si {
		return ""
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

// extractJSONPlan pulls <jsonplan> content as a byte slice for json.Unmarshal.
func extractJSONPlan(s string) []byte {
	content := extractBetween(s, "<"+TagJSON+">", "</"+TagJSON+">")
	if content == "" {
		return nil
	}
	return repairJSON([]byte(content))
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
	case "<" + TagResponse + ">":
		return sfInRoot, true
	case "</" + TagResponse + ">":
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

	// 3. Attempt to close braces/brackets
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
