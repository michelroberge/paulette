package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/michelroberge/claudette/backend/internal/model"
)

const mockSystemPromptBase = `You are a UI mockup generator for an AI Product Factory. Based on the provided UX design document, generate a complete standalone HTML page that visually represents the proposed UI as high-fidelity wireframe mockups.

Requirements:
- Generate a SINGLE self-contained HTML file with all styling inline or in a <style> block
- Navigation bars, sidebars, buttons, cards, inputs, and lists should look like realistic UI components
- Use Unicode symbols for icons (e.g. ☰ ✓ ← → ✕ ⚙ 🔍 + ●)
- Include realistic placeholder text — product names, usernames, dates, descriptions
- Screens should have a realistic fixed width (e.g. 1024px centered) with a subtle drop shadow

%s

Screen navigation:
- If the document describes multiple screens or pages, implement a tab bar at the very top of the page with one tab per screen
- Clicking a tab shows only that screen and hides all others (use inline JavaScript to toggle visibility)
- The active tab should be visually highlighted
- If there is only one screen, no tab bar is needed — just render it directly

CRITICAL OUTPUT RULES:
- Your ENTIRE response must be ONLY the HTML file — nothing else
- Start your response with <!DOCTYPE html> as the very first characters
- End your response with </html> as the very last characters
- Do NOT include any explanation, summary, preamble, markdown, code fences, or commentary before or after the HTML
- Do NOT ask for permission, confirmation, or offer to save the file
- Do NOT describe what the mockup contains
- Just output the raw HTML. Nothing else.`

var frameworkInstructions = map[model.UXFramework]string{
	model.FrameworkTailwind: `Framework: Tailwind CSS
- Include the Tailwind CSS Play CDN: <script src="https://cdn.tailwindcss.com"></script>
- Use Tailwind utility classes exclusively for all styling (bg-slate-900, text-slate-100, rounded-lg, shadow-lg, etc.)
- Use dark mode palette: bg-slate-900, bg-slate-800, bg-slate-700, text-slate-100, text-blue-500
- Do NOT use a <style> block for layout — use utility classes directly on elements`,

	model.FrameworkBootstrap: `Framework: Bootstrap 5
- Include Bootstrap 5 CSS CDN: <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css" rel="stylesheet">
- Include Bootstrap 5 JS CDN: <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/js/bootstrap.bundle.min.js"></script>
- Use Bootstrap grid, components (card, navbar, btn, form-control, badge, list-group, etc.)
- Add a custom <style> block for dark theme overrides: body {background:#0f172a; color:#e2e8f0}`,

	model.FrameworkMUI: `Framework: Material UI (MUI) design language
- MUI requires React so use plain CSS that approximates the Material Design aesthetic
- Color palette: primary #1976d2, background #121212, surface #1e1e1e, on-surface #ffffff, secondary #90caf9
- Use box-shadow for Material elevation levels (dp2: 0 2px 4px rgba(0,0,0,.4), dp4: 0 4px 8px rgba(0,0,0,.4))
- Typography: use Roboto font via <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@400;500;700&display=swap">
- Rounded corners: 4px for components, 8px for cards`,

	model.FrameworkShadcn: `Framework: Shadcn/UI design language
- Use Shadcn/UI visual conventions with plain CSS (no React required)
- Define CSS variables in :root: --background: #09090b; --foreground: #fafafa; --card: #18181b; --border: #27272a; --primary: #fafafa; --muted: #71717a
- Use zinc color scale for neutrals, rounded-md (6px) borders, subtle ring borders
- Component style: bordered cards with 1px solid var(--border), subtle hover states`,

	model.FrameworkVanilla: `Framework: Vanilla CSS
- Use only plain CSS in a <style> block — no external dependencies
- Modern dark aesthetic: dark background (#0f172a), slate panels (#1e293b), blue accents (#3b82f6), light text (#e2e8f0)
- Use CSS Grid and Flexbox for layout`,
}

// buildMockSystemPrompt constructs the mock generation prompt for the given framework.
func buildMockSystemPrompt(cfg *model.FrameworkConfig) string {
	if cfg == nil {
		cfg = &model.FrameworkConfig{Framework: model.FrameworkVanilla}
	}

	instructions, ok := frameworkInstructions[cfg.Framework]
	if !ok || cfg.Framework == model.FrameworkOther {
		name := cfg.CustomName
		if name == "" {
			name = "custom framework"
		}
		instructions = fmt.Sprintf(`Framework: %s
- Use plain CSS in a <style> block as the base styling approach
- Apply %s design conventions as closely as possible in a standalone HTML file
- Modern dark aesthetic as a baseline: background #0f172a, panels #1e293b, accents #3b82f6`, name, name)
	}

	return fmt.Sprintf(mockSystemPromptBase, instructions)
}

const maxMockRetries = 2

const mockRetryPrompt = `Your previous response was NOT valid HTML. You returned conversational text instead of raw HTML.

I need you to output ONLY the HTML mockup. Your response must:
- Start with <!DOCTYPE html> as the very first characters
- End with </html> as the very last characters
- Contain NO explanation, summary, or commentary

Here is your previous (wrong) response for reference — do NOT repeat this mistake:
---
%s
---

Now output the complete HTML wireframe mockup. RAW HTML ONLY.`

// looksLikeHTML checks whether the response starts with a valid HTML doctype or tag.
func looksLikeHTML(s string) bool {
	trimmed := strings.TrimSpace(s)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html")
}

// invokeMockClaude runs a single Claude invocation and collects streamed events.
// It sends chunks to the provided channel and returns the full response text.
func invokeMockClaude(ctx context.Context, systemPrompt, userPrompt string, ch chan<- StreamEvent) (string, error) {
	cmd := exec.CommandContext(ctx, "claude",
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--system-prompt", systemPrompt,
	)
	cmd.Stdin = strings.NewReader(userPrompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start claude: %w", err)
	}
	defer cmd.Wait()

	var fullResponse strings.Builder
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event claudeEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		switch event.Type {
		case "assistant":
			if event.Message != nil {
				for _, c := range event.Message.Content {
					if c.Type == "text" && c.Text != "" {
						fullResponse.WriteString(c.Text)
						ch <- StreamEvent{Type: "chunk", Content: c.Text}
					}
				}
			}
		case "result":
			if event.Usage != nil {
				ch <- StreamEvent{Type: "tokens", Content: strconv.Itoa(event.Usage.OutputTokens)}
			}
			if fullResponse.Len() == 0 && event.Result != "" {
				fullResponse.WriteString(event.Result)
				ch <- StreamEvent{Type: "chunk", Content: event.Result}
			}
		}
	}

	return fullResponse.String(), nil
}

// GenerateMock streams an HTML wireframe mockup from Claude based on UX artifact content.
// If Claude returns conversational text instead of raw HTML, it retries up to maxMockRetries
// times with a correction prompt.
func GenerateMock(ctx context.Context, uxArtifact string, refinement string, frameworkCfg *model.FrameworkConfig) (<-chan StreamEvent, error) {
	var prompt strings.Builder
	prompt.WriteString("UX Design Document:\n---\n")
	prompt.WriteString(uxArtifact)
	prompt.WriteString("\n---\n\nGenerate the HTML wireframe mockup for all screens described above.")
	if refinement != "" {
		prompt.WriteString("\n\nRefinement instruction: ")
		prompt.WriteString(refinement)
	}

	systemPrompt := buildMockSystemPrompt(frameworkCfg)

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)

		userPrompt := prompt.String()

		for attempt := 0; attempt <= maxMockRetries; attempt++ {
			if ctx.Err() != nil {
				return
			}

			response, err := invokeMockClaude(ctx, systemPrompt, userPrompt, ch)
			if err != nil {
				ch <- StreamEvent{Type: "done", Content: ""}
				return
			}

			if looksLikeHTML(response) {
				ch <- StreamEvent{Type: "done", Content: response}
				return
			}

			// Response was not HTML — retry with correction prompt
			if attempt < maxMockRetries {
				// Truncate the bad response for the retry prompt (keep first 500 chars)
				snippet := response
				if len(snippet) > 500 {
					snippet = snippet[:500] + "..."
				}
				userPrompt = fmt.Sprintf(mockRetryPrompt, snippet)
				ch <- StreamEvent{Type: "chunk", Content: "\n\n[Response was not valid HTML — retrying...]\n\n"}
			} else {
				// Exhausted retries — return whatever we got
				ch <- StreamEvent{Type: "done", Content: response}
			}
		}
	}()

	return ch, nil
}

// ExtractHTML strips markdown code fences if Claude accidentally wraps the HTML.
func ExtractHTML(raw string) string {
	s := strings.TrimSpace(raw)
	// Remove ```html ... ``` or ``` ... ```
	if strings.HasPrefix(s, "```") {
		first := strings.Index(s, "\n")
		if first >= 0 {
			s = s[first+1:]
		}
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
		s = strings.TrimSpace(s)
	}
	return s
}
