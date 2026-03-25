package agent

import "strings"

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

// ParseResponse extracts all sections from a <response>...</response> XML envelope.
// Tolerant of missing sections and partial documents.
func ParseResponse(raw string) ParsedResponse {
	return ParsedResponse{
		Discussion: extractBetween(raw, "<"+TagDiscussion+">", "</"+TagDiscussion+">"),
		Artifact:   extractBetween(raw, "<"+TagArtifact+">", "</"+TagArtifact+">"),
		HTML:       extractCDATA(extractBetween(raw, "<"+TagHTML+">", "</"+TagHTML+">")),
		JSON:       extractJSONPlan(raw),
	}
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
	return []byte(content)
}

// streamFilterState tracks which section of the XML envelope is being streamed.
type streamFilterState int

const (
	sfBeforeRoot   streamFilterState = iota
	sfInRoot                         // inside <response>, between tags
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
