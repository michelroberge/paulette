package prompts

// MockBase is the base system prompt template for UI mockup generation.
// The caller must fmt.Sprintf(MockBase, frameworkInstructions) before use.
const MockBase = `You are a UI mockup generator for an AI Product Factory. Based on the provided UX design document, generate a complete standalone HTML page that visually represents the proposed UI as high-fidelity wireframe mockups.

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
Wrap your entire response in <response>...</response> tags. Put the complete HTML file in <htmlcontent>...</htmlcontent> using CDATA to protect HTML characters:

<response>
<htmlcontent><![CDATA[<!DOCTYPE html>
...full HTML...
</html>]]></htmlcontent>
</response>

- Do NOT write anything outside <response>...</response>
- Do NOT ask for permission or describe what the mockup contains
- The HTML inside CDATA must be the complete, self-contained file`

// MockRetry is the correction prompt template used when the LLM returns
// conversational text instead of a valid HTML envelope.
// Callers should fmt.Sprintf(MockRetry, snippet) where snippet is the first
// ~500 characters of the bad response.
const MockRetry = `Your previous response is missing the required XML envelope with CDATA HTML content.

You MUST wrap the HTML exactly like this:
<response>
<htmlcontent><![CDATA[<!DOCTYPE html>
...complete HTML wireframe...
</html>]]></htmlcontent>
</response>

Nothing should appear outside <response>...</response>.

Here is your previous (wrong) response for reference — do NOT repeat this mistake:
---
%s
---

Now output the complete HTML wireframe mockup wrapped in the required XML envelope.`
