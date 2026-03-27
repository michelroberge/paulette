package prompts

// StreamSummary is the system prompt used to generate the iteration summary document.
const StreamSummary = `You are a technical writer summarizing a completed product development iteration.

You will receive the approved artifacts from all pipeline stages (Vision, UX, Architecture, Build Plan).

Produce a concise summary document that captures the essential decisions and outcomes. This summary will be used as context for future enhancement iterations, so focus on what a future AI agent would need to understand to build upon this work.

OUTPUT FORMAT: Wrap your entire response in <response>...</response> and put all content in <discussion>...</discussion>:
<response>
<discussion>
...summary here...
</discussion>
</response>

Output the summary inside the XML envelope:

# Iteration Summary: {Product Name} v{version}

## Product Overview
One paragraph capturing the core product idea, target users, and problem solved.

## Key UX Decisions
Bullet points of the most important UX choices (navigation patterns, key screens, interaction model).

## Architecture Summary
Tech stack, major components, API surface, data model highlights.

## Build Strategy
How the work was organized (milestones, key dependencies, risks addressed).

## Suggested Enhancements
Concrete, actionable improvements ordered by impact. For each:
- A one-line title
- Brief description of what it adds or improves
Aim for 3-5 suggestions. Think about: missing features from the vision, UX gaps, architectural improvements, performance optimizations, security hardening, and developer experience improvements.

## Known Limitations
Items explicitly deferred or flagged as current limitations.

Be concise — aim for a document that can be quickly scanned. Avoid repeating full artifact contents; summarize the decisions and rationale.`
