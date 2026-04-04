package agent

import (
	"fmt"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/model"
	ollamaprompts "github.com/michelroberge/paulette/backend/internal/prompts/ollama"
)

// MockView holds a single screen's generated HTML fragment for use by the assembler.
type MockView struct {
	ID    string
	Title string
	HTML  string // body fragment only — no <html>/<head>/<body> shell
}

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

OUTPUT FORMAT — follow this exactly:
Wrap your entire response in <!-- RESPONSE:START -->...<!-- RESPONSE:END --> tags. Put the complete HTML file in <htmlcontent>...</htmlcontent> using CDATA to protect HTML characters:

<!-- RESPONSE:START -->
<htmlcontent><![CDATA[<!DOCTYPE html>
...full HTML...
</html>]]></htmlcontent>
<!-- RESPONSE:END -->

- Do NOT write anything outside <!-- RESPONSE:START -->...<!-- RESPONSE:END -->
- Do NOT ask for permission or describe what the mockup contains
- The HTML inside CDATA must be the complete, self-contained file`

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

// frameworkShellHeads contains the <head> elements (CDN links, CSS variables, base styles)
// that the deterministic assembler injects into the shared HTML shell. Each view fragment
// relies on these being present but must not re-include them.
var frameworkShellHeads = map[model.UXFramework]string{
	model.FrameworkTailwind: `<script src="https://cdn.tailwindcss.com"></script>`,

	model.FrameworkBootstrap: `<link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css" rel="stylesheet">
<script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/js/bootstrap.bundle.min.js"></script>
<style>body{background:#0f172a;color:#e2e8f0}</style>`,

	model.FrameworkMUI: `<link href="https://fonts.googleapis.com/css2?family=Roboto:wght@400;500;700&display=swap" rel="stylesheet">
<style>body{font-family:'Roboto',sans-serif;background:#121212;color:#fff}</style>`,

	model.FrameworkShadcn: `<style>
:root{--background:#09090b;--foreground:#fafafa;--card:#18181b;--border:#27272a;--primary:#fafafa;--muted:#71717a}
body{background:var(--background);color:var(--foreground)}
</style>`,

	model.FrameworkVanilla: `<style>
:root{--bg:#0f172a;--panel:#1e293b;--accent:#3b82f6;--text:#e2e8f0;--border:#334155}
body{background:var(--bg);color:var(--text)}
</style>`,
}

// frameworkViewInstructs contains per-view agent instructions (which classes/styles to use)
// without CDN links — those are already in frameworkShellHeads.
var frameworkViewInstructs = map[model.UXFramework]string{
	model.FrameworkTailwind: `Framework: Tailwind CSS (CDN already loaded in shell)
- Use Tailwind utility classes exclusively (bg-slate-900, text-slate-100, rounded-lg, shadow-lg, etc.)
- Dark mode palette: bg-slate-900, bg-slate-800, bg-slate-700, text-slate-100, text-blue-500
- Do NOT include <script> CDN tags — Tailwind is already available
- FORBIDDEN: Do NOT use Bootstrap class names (btn, card, d-flex, container, row, col-*)
- FORBIDDEN: Do NOT write <style> blocks — use utility classes directly on elements`,

	model.FrameworkBootstrap: `Framework: Bootstrap 5 (CDN already loaded in shell)
- Use Bootstrap grid, components (card, navbar, btn, form-control, badge, list-group, etc.)
- Dark theme is already applied globally; use Bootstrap dark variants where available
- Do NOT include <link> or <script> CDN tags — Bootstrap is already available
- FORBIDDEN: Do NOT use Tailwind utility class names (bg-slate-*, text-*, p-4, flex, rounded-lg)
- Always use Bootstrap layout: container, row, col-*`,

	model.FrameworkMUI: `Framework: Material UI (MUI) design language — plain CSS approximation
- Color palette: primary #1976d2, background #121212, surface #1e1e1e, on-surface #fff, secondary #90caf9
- Use box-shadow for elevation (dp2: 0 2px 4px rgba(0,0,0,.4), dp4: 0 4px 8px rgba(0,0,0,.4))
- Roboto font is already loaded; rounded corners: 4px for components, 8px for cards
- Inline <style> blocks for component CSS are fine
- FORBIDDEN: Do NOT add Tailwind or Bootstrap class names — use only inline style="" attributes or <style> blocks`,

	model.FrameworkShadcn: `Framework: Shadcn/UI design language — plain CSS
- Use CSS variables already defined in the shell: var(--background), var(--foreground), var(--card), var(--border), var(--primary), var(--muted)
- Zinc color scale for neutrals, rounded-md (6px) borders, subtle hover states
- Bordered cards with 1px solid var(--border); inline <style> blocks are fine
- FORBIDDEN: Do NOT add Tailwind or Bootstrap class names — use only CSS variables and inline style="" attributes`,

	model.FrameworkVanilla: `Framework: Vanilla CSS
- CSS variables are already defined in the shell: var(--bg), var(--panel), var(--accent), var(--text), var(--border)
- Use CSS Grid and Flexbox; inline <style> blocks are fine
- FORBIDDEN: Do NOT add Tailwind or Bootstrap class names — use only CSS variables and inline style="" attributes`,
}

// frameworkStylerContracts contains the allowed class/style contracts for each framework,
// injected into the MockStyler system prompt to constrain the styler's rewriting pass.
var frameworkStylerContracts = map[model.UXFramework]string{
	model.FrameworkTailwind: `Framework: Tailwind CSS
ALLOWED: Only Tailwind utility classes — bg-*, text-*, p-*, m-*, flex, grid, rounded-*, shadow-*, border-*, hover:*, w-*, h-*, gap-*, items-*, justify-*, overflow-*, opacity-*, font-*, leading-*, tracking-*
FORBIDDEN: Any Bootstrap class names (btn, card, d-flex, container, row, col-*) or MUI/Shadcn names
FORBIDDEN: Any <style> blocks — move all styling to utility classes`,

	model.FrameworkBootstrap: `Framework: Bootstrap 5
ALLOWED: Only Bootstrap component classes — btn, btn-*, card, card-body, card-header, navbar, nav, nav-link, container, row, col-*, form-control, badge, list-group, list-group-item, table, alert, modal, d-flex, d-grid, gap-*, justify-content-*, align-items-*
FORBIDDEN: Any Tailwind utility classes (bg-slate-*, text-*, p-4, rounded-lg, shadow-lg)`,

	model.FrameworkMUI: `Framework: MUI plain CSS approximation
ALLOWED: Only inline style="" attributes and <style> blocks. No class="" values with framework names.
Remove any Tailwind or Bootstrap class names from class="" attributes — replace with inline style="" using MUI palette: primary #1976d2, background #121212, surface #1e1e1e`,

	model.FrameworkShadcn: `Framework: Shadcn/UI plain CSS
ALLOWED: Only CSS variable references in style="" attributes: var(--background), var(--foreground), var(--card), var(--border), var(--primary), var(--muted)
Remove any Tailwind or Bootstrap class names from class="" attributes — replace with inline style="" using the CSS variables`,

	model.FrameworkVanilla: `Framework: Vanilla CSS
ALLOWED: Only CSS variable references in style="" attributes: var(--bg), var(--panel), var(--accent), var(--text), var(--border)
Remove any Tailwind or Bootstrap class names from class="" attributes — replace with inline style="" using the CSS variables`,
}

// frameworkShellHead returns the <head> snippet for the given framework config.
func frameworkShellHead(cfg *model.FrameworkConfig) string {
	if cfg == nil {
		return frameworkShellHeads[model.FrameworkVanilla]
	}
	h, ok := frameworkShellHeads[cfg.Framework]
	if !ok {
		return frameworkShellHeads[model.FrameworkVanilla]
	}
	return h
}

// frameworkViewInstruct returns the per-view agent framework instruction for the given config.
func frameworkViewInstruct(cfg *model.FrameworkConfig) string {
	if cfg == nil {
		return frameworkViewInstructs[model.FrameworkVanilla]
	}
	instruct, ok := frameworkViewInstructs[cfg.Framework]
	if !ok || cfg.Framework == model.FrameworkOther {
		name := cfg.CustomName
		if name == "" {
			name = "custom framework"
		}
		return fmt.Sprintf(`Framework: %s — plain CSS approximation
- Use plain CSS inline <style> blocks as the base styling
- Apply %s design conventions as closely as possible
- Modern dark aesthetic: background #0f172a, panels #1e293b, accents #3b82f6`, name, name)
	}
	return instruct
}

// buildMockViewSystemPrompt constructs the per-view system prompt for the given framework.
func buildMockViewSystemPrompt(cfg *model.FrameworkConfig) string {
	return fmt.Sprintf(ollamaprompts.MockViewBase, frameworkViewInstruct(cfg))
}

// BuildMockViewSystemPrompt is the exported version of buildMockViewSystemPrompt.
func BuildMockViewSystemPrompt(cfg *model.FrameworkConfig) string {
	return buildMockViewSystemPrompt(cfg)
}

// BuildMockStylerSystemPrompt returns the system prompt for the Styler post-processing
// step, which rewrites class="" attributes in assembled HTML to use only the correct
// framework classes. Returns "" for FrameworkOther or nil config (caller should skip).
func BuildMockStylerSystemPrompt(cfg *model.FrameworkConfig) string {
	if cfg == nil || cfg.Framework == model.FrameworkOther {
		return ""
	}
	contract, ok := frameworkStylerContracts[cfg.Framework]
	if !ok {
		return ""
	}
	return fmt.Sprintf(ollamaprompts.MockStyler, contract)
}

// BuildMockPlannerSystemPrompt returns the system prompt used by the orchestrated
// planner step to decompose a UX artifact into a screen list.
func BuildMockPlannerSystemPrompt() string {
	return fmt.Sprintf(ollamaprompts.MockPlanner)
}

// AssembleMockHTML combines generated view fragments into a single self-contained
// HTML file with tab navigation. This is purely deterministic — no LLM involved.
func AssembleMockHTML(cfg *model.FrameworkConfig, views []MockView) string {
	var b strings.Builder

	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"UTF-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	b.WriteString("<title>UI Mockup</title>\n")
	b.WriteString(frameworkShellHead(cfg))
	b.WriteString("\n<style>\n")
	b.WriteString("*{box-sizing:border-box}\n")
	b.WriteString(".mock-tab-bar{display:flex;gap:4px;padding:8px 16px;background:#1e293b;border-bottom:1px solid #334155;flex-wrap:wrap}\n")
	b.WriteString(".mock-tab{padding:6px 16px;border:none;border-radius:6px;cursor:pointer;background:transparent;color:#94a3b8;font-size:14px;font-family:inherit}\n")
	b.WriteString(".mock-tab.active{background:#3b82f6;color:#fff}\n")
	b.WriteString(".mock-screen{display:none}\n")
	b.WriteString(".mock-screen.active{display:block}\n")
	b.WriteString("</style>\n</head>\n<body>\n")

	// Tab bar — only when there are multiple screens
	if len(views) > 1 {
		b.WriteString("<nav class=\"mock-tab-bar\">\n")
		for i, v := range views {
			active := ""
			if i == 0 {
				active = " active"
			}
			b.WriteString(fmt.Sprintf(
				"  <button class=\"mock-tab%s\" onclick=\"mockShow('%s',this)\">%s</button>\n",
				active, v.ID, v.Title,
			))
		}
		b.WriteString("</nav>\n")
	}

	// Screen containers
	for i, v := range views {
		active := ""
		if i == 0 {
			active = " active"
		}
		b.WriteString(fmt.Sprintf("<div id=\"mock-screen-%s\" class=\"mock-screen%s\">\n", v.ID, active))
		b.WriteString(v.HTML)
		b.WriteString("\n</div>\n")
	}

	// Tab-switching script
	b.WriteString("<script>\n")
	b.WriteString("function mockShow(id,btn){\n")
	b.WriteString("  document.querySelectorAll('.mock-screen').forEach(function(s){s.classList.remove('active')});\n")
	b.WriteString("  document.querySelectorAll('.mock-tab').forEach(function(t){t.classList.remove('active')});\n")
	b.WriteString("  var s=document.getElementById('mock-screen-'+id);\n")
	b.WriteString("  if(s)s.classList.add('active');\n")
	b.WriteString("  if(btn)btn.classList.add('active');\n")
	b.WriteString("}\n")
	b.WriteString("</script>\n")

	b.WriteString("</body>\n</html>")
	return b.String()
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

// MaxMockRetries is the exported maximum number of retry attempts when the LLM
// does not include a valid HTML envelope in its response. Exported so the mock
// handler can implement the same retry loop when using the provider layer.
const MaxMockRetries = maxMockRetries

const mockRetryPrompt = `Your previous response is missing the required XML envelope with CDATA HTML content.

You MUST wrap the HTML exactly like this:
<!-- RESPONSE:START -->
<htmlcontent><![CDATA[<!DOCTYPE html>
...complete HTML wireframe...
</html>]]></htmlcontent>
<!-- RESPONSE:END -->

Nothing should appear outside <!-- RESPONSE:START -->...<!-- RESPONSE:END -->.

Here is your previous (wrong) response for reference — do NOT repeat this mistake:
---
%s
---

Now output the complete HTML wireframe mockup wrapped in the required XML envelope.`

// hasHTMLBlock checks whether the response contains the XML envelope with htmlcontent,
// or raw HTML content (for models that don't follow the envelope format).
func hasHTMLBlock(s string) bool {
	if strings.Contains(s, "<htmlcontent>") && strings.Contains(s, "</htmlcontent>") {
		return true
	}
	// Fallback: check for raw HTML in the response
	lower := strings.ToLower(s)
	return (strings.Contains(lower, "<!doctype html>") || strings.Contains(lower, "<html")) &&
		strings.Contains(lower, "</html>")
}

// HasHTMLBlock is the exported version of hasHTMLBlock. It checks whether the
// response string contains the expected XML envelope with CDATA HTML content.
// Exported so the mock handler can use it in the provider-layer retry loop.
func HasHTMLBlock(s string) bool { return hasHTMLBlock(s) }

// BuildMockSystemPrompt is the exported version of buildMockSystemPrompt.
// It constructs the mock generation system prompt for the given framework
// configuration. Exported so the mock handler can build the prompt when
// calling the provider layer directly (bypassing agent.GenerateMock).
func BuildMockSystemPrompt(cfg *model.FrameworkConfig) string { return buildMockSystemPrompt(cfg) }

// BuildMockContext builds a compressed project context block that precedes the
// UX design document in mock generation prompts. This ensures the LLM knows
// what the app is about (its name, vision, and high-level plan) rather than
// generating generic wireframes that don't match the project.
func BuildMockContext(projectName, visionContent, buildContent string) string {
	if visionContent == "" && buildContent == "" && projectName == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Project Context\n")
	if projectName != "" {
		b.WriteString("Project: ")
		b.WriteString(projectName)
		b.WriteString("\n")
	}
	if visionContent != "" {
		b.WriteString("\n### Vision\n")
		// Include up to ~2000 chars of vision to keep context manageable
		if len(visionContent) > 2000 {
			b.WriteString(visionContent[:2000])
			b.WriteString("\n... (truncated)\n")
		} else {
			b.WriteString(visionContent)
			b.WriteString("\n")
		}
	}
	if buildContent != "" {
		b.WriteString("\n### Build Plan Summary\n")
		// Include up to ~1500 chars of build plan for high-level context
		if len(buildContent) > 1500 {
			b.WriteString(buildContent[:1500])
			b.WriteString("\n... (truncated)\n")
		} else {
			b.WriteString(buildContent)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n---\n\n")
	return b.String()
}

// MockRetryPrompt is the exported correction prompt template used when the LLM
// returns conversational text instead of a valid HTML envelope. Callers should
// fmt.Sprintf(agent.MockRetryPrompt, snippet) where snippet is the first 500
// characters of the bad response.
const MockRetryPrompt = mockRetryPrompt

// ExtractHTML extracts HTML from a Claude XML envelope response.
// Falls back to extracting raw HTML if no envelope is found.
func ExtractHTML(raw string) string {
	html := ParseResponse(raw).HTML
	if html != "" {
		return html
	}
	// Fallback: extract raw HTML from the response
	return extractRawHTML(raw)
}

// extractRawHTML finds HTML content in a response that lacks the XML envelope.
// It looks for <!DOCTYPE html>...</html> or <html>...</html> blocks,
// including inside code blocks (```html ... ```).
func extractRawHTML(s string) string {
	// Try code block first
	codeStart := strings.Index(s, "```html")
	if codeStart >= 0 {
		after := s[codeStart+7:]
		codeEnd := strings.Index(after, "```")
		if codeEnd > 0 {
			candidate := strings.TrimSpace(after[:codeEnd])
			if len(candidate) > 50 {
				return candidate
			}
		}
	}

	// Try raw HTML
	lower := strings.ToLower(s)
	dtIdx := strings.Index(lower, "<!doctype html>")
	if dtIdx < 0 {
		dtIdx = strings.Index(lower, "<html")
	}
	if dtIdx < 0 {
		return ""
	}

	endIdx := strings.LastIndex(lower, "</html>")
	if endIdx < dtIdx {
		return ""
	}

	return strings.TrimSpace(s[dtIdx : endIdx+7])
}
