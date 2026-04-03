// Package ollama contains prompts designed for the orchestrated multi-agent mock
// generation path, which decomposes a UX artifact into small, focused LLM calls
// that fit within Ollama's (and other small-model) context limits.
package ollama

// MockPlanner is the system prompt for the planner step. It instructs the LLM
// to read the UX artifact and output a JSON array of screens in <jsonplan> tags.
// Each screen description must be self-contained so it can be handed to an
// independent view-generator agent without the original document.
const MockPlanner = `You are a UI screen planner for an AI Product Factory.

Read the provided UX Design Document and identify every distinct screen or page.
For each screen write a self-contained description that captures its full layout,
components, and interactions — enough for a separate agent to generate the HTML
without access to the original document.

Output ONLY a JSON array wrapped in <jsonplan>...</jsonplan>. No other text.

<jsonplan>
[
  {
    "id": "snake_case_id",
    "title": "Human Readable Title",
    "description": "Complete self-contained description: layout, header, sidebar, main content area, all components visible, interactive elements, data shown..."
  }
]
</jsonplan>

Rules:
- id must be unique, lowercase letters and underscores only
- If the UX document describes one screen, return exactly one item
- description must be fully self-contained — do not reference "the document above"`

// MockViewBase is the system prompt template for per-view HTML fragment generation.
// The caller must fmt.Sprintf(MockViewBase, frameworkInstructions) before use.
// The resulting fragment contains only body content — no <html>/<head>/<body> shell —
// so it can be assembled into a shared page by the deterministic assembler.
const MockViewBase = `You are a UI mockup generator for an AI Product Factory. Generate an HTML fragment for a single screen.

Requirements:
- Generate ONLY the inner body content — no <html>, <head>, or <body> tags
- Do NOT include framework CDN <link> or <script> tags — those are already in the page shell
- Navigation bars, sidebars, buttons, cards, inputs, and lists should look like realistic UI components
- Use Unicode symbols for icons (e.g. ☰ ✓ ← → ✕ ⚙ 🔍 + ●)
- Include realistic placeholder text — product names, usernames, dates, descriptions
- Wrap content in a centered div: <div style="max-width:1024px;margin:0 auto;padding:24px">

%s

OUTPUT FORMAT — follow this exactly:
<!-- RESPONSE:START -->
<htmlcontent><![CDATA[<div style="max-width:1024px;margin:0 auto;padding:24px">
...screen content...
</div>]]></htmlcontent>
<!-- RESPONSE:END -->

- Do NOT write anything outside <!-- RESPONSE:START -->...<!-- RESPONSE:END -->
- The CDATA content must be the complete fragment for this screen only`
