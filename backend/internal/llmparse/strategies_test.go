package llmparse

import (
	"testing"
)

// samplePlannerOutput is the exact text Ollama returned instead of JSON.
const samplePlannerOutput = `Here are the screens described in the document, listed for clarity:

1. End User Dashboard (/): This is the initial landing page where users can see a list of playable games. The interface includes a game cover and title, and a "Play" button that opens the selected game in a new tab or shows an error message if the proxy is unreachable.
2. Add New Game Drawer (JRN-v0.1.0-001): This modal slides in from the right when the user clicks "+ Add Game" on the Admin Dashboard. It contains fields for game name, description, cover image, internal port, proxy slug, AI Enhanced toggle, and status. The form has inline validation, optimistic updates, and toast notifications upon submission.
3. Edit Game Drawer (JRN-v0.1.0-003): This is similar to the Add New Game drawer but pre-filled with the game's details when a user clicks "Edit" on the Admin Dashboard. The difference is that it allows the admin to edit existing games, while the Add New Game drawer creates new ones.
4. Confirmation Modal (JRN-v0.1.0-002): This modal appears when a user clicks "Delete" on an admin game card. It displays a message asking for confirmation to delete the game and includes a Cancel button to discard the action.
5. Game Proxy Status Indicator (on Admin cards): Each admin game card shows a live proxy health dot, indicating the status of the associated game's proxy (proxying, stopped, or unreachable). Hovering the dot displays a tooltip with details like "Proxy active - localhost:3001".
6. Admin Dashboard (/admin): This is the admin interface where admins can manage games. It includes a sidebar for navigation and a game list that shows the game title, cover image, and status. The page allows adding new games, editing existing ones, deleting games, toggling their active/inactive status, and viewing the dashboard as a user would see it.
7. View as User (not explicitly mentioned in the document but implied): This is the behavior when clicking "View as User" on the Admin Dashboard, which opens the End User Dashboard (/ ) in a new tab while preserving the admin session context.`

// TestListStrategyDetect verifies the sample gets a score high enough to be selected.
func TestListStrategyDetect(t *testing.T) {
	score := (ListStrategy{}).Detect(samplePlannerOutput)
	if score < 0.5 {
		t.Errorf("expected Detect score >= 0.5, got %f", score)
	}
}

// TestListStrategyParseDirectly verifies Parse works when called with *[]string.
func TestListStrategyParseDirectly(t *testing.T) {
	ls := ListStrategy{}
	var titles []string
	if err := ls.Parse(samplePlannerOutput, &titles); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(titles) != 7 {
		t.Errorf("expected 7 titles, got %d: %v", len(titles), titles)
	}
}

// TestListStrategyParseViaAny verifies Parse works when called with *any (engine intermediate path).
func TestListStrategyParseViaAny(t *testing.T) {
	ls := ListStrategy{}
	var intermediate any
	if err := ls.Parse(samplePlannerOutput, &intermediate); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	result, ok := intermediate.([]string)
	if !ok {
		t.Fatalf("expected intermediate to be []string, got %T", intermediate)
	}
	if len(result) != 7 {
		t.Errorf("expected 7 items, got %d", len(result))
	}
}

// TestEngineParseListIntoStringSlice is the exact code path used in runPlannerCall.
// This is the regression test for the bug where the engine's *any intermediate
// caused ListStrategy to return ErrTypeMismatch.
func TestEngineParseListIntoStringSlice(t *testing.T) {
	engine := NewEngine([]Strategy{ListStrategy{}}, nil)
	var titles []string
	if err := engine.Parse(samplePlannerOutput, &titles); err != nil {
		t.Fatalf("engine.Parse returned error: %v", err)
	}
	if len(titles) != 7 {
		t.Errorf("expected 7 titles, got %d: %v", len(titles), titles)
	}
	if titles[0] == "" {
		t.Error("first title is empty")
	}
	if titles[6] == "" {
		t.Error("last title is empty")
	}
}

// TestHTMLStrategyParseViaAny verifies HTMLStrategy works through the engine's intermediate path.
func TestHTMLStrategyParseViaAny(t *testing.T) {
	hs := HTMLStrategy{}
	input := "<!-- RESPONSE:START -->\n<htmlcontent><![CDATA[<div>hello</div>]]></htmlcontent>\n<!-- RESPONSE:END -->"
	var intermediate any
	if err := hs.Parse(input, &intermediate); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	s, ok := intermediate.(string)
	if !ok {
		t.Fatalf("expected string, got %T", intermediate)
	}
	if s != "<div>hello</div>" {
		t.Errorf("unexpected html: %q", s)
	}
}

// TestHTMLStrategyCodeFence verifies HTMLStrategy falls back to code fence extraction.
func TestHTMLStrategyCodeFence(t *testing.T) {
	hs := HTMLStrategy{}
	input := "Here is the component:\n```html\n<div class=\"card\">content</div>\n```"
	var result string
	if err := hs.Parse(input, &result); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty html from code fence")
	}
}

// TestHTMLStrategyDetectPreamble verifies that Detect scores > 0 when the model
// prefixes HTML with explanatory text (common Ollama behavior).
func TestHTMLStrategyDetectPreamble(t *testing.T) {
	input := "Here is the HTML component:\n\n<div class=\"card\">content</div>"
	score := (HTMLStrategy{}).Detect(input)
	if score <= 0 {
		t.Errorf("expected Detect score > 0 for preamble-prefixed HTML, got %f", score)
	}
}

// TestHTMLStrategyParsePreamble verifies that Parse extracts HTML from a response
// that has preamble text before the actual HTML tags.
func TestHTMLStrategyParsePreamble(t *testing.T) {
	input := "Here is the HTML component:\n\n<div class=\"card\">content</div>"
	var result string
	if err := (HTMLStrategy{}).Parse(input, &result); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	expected := "<div class=\"card\">content</div>"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestHTMLStrategyDetectPlainText verifies that Detect returns 0 for plain text
// with no HTML tags.
func TestHTMLStrategyDetectPlainText(t *testing.T) {
	input := "This is just a regular sentence with no HTML at all."
	score := (HTMLStrategy{}).Detect(input)
	if score != 0 {
		t.Errorf("expected Detect score 0 for plain text, got %f", score)
	}
}

// TestHTMLStrategyParsePreambleMultipleTags verifies extraction when there are
// multiple block-level tags — should extract from the first one.
func TestHTMLStrategyParsePreambleMultipleTags(t *testing.T) {
	input := "I'll create a navigation bar and content section:\n\n<nav class=\"main-nav\">Nav</nav>\n<div class=\"content\">Body</div>"
	var result string
	if err := (HTMLStrategy{}).Parse(input, &result); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
	if result[0:4] != "<nav" {
		t.Errorf("expected result to start with <nav, got %q", result[:20])
	}
}
