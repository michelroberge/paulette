- Purpose:
  Add a synthesis step before mock generation that condenses vision + UX design into a smaller specs document, reducing context window usage for mock HTML generation.

- Outcome:
  - **New prompt**: `mock-specs-synthesize.md.tmpl` — instructs AI to produce a concise Mock Specs document focusing only on visual layout, screens, navigation, interactions. Aims for 40-60% compression.
  - **Backend synthesis**: `synthesizeMockSpecs()` in handler/mock.go calls the LLM to synthesize, compares sizes, caches at `{dataDir}/ux/mock-specs.md`. Returns empty string if synthesis is larger (caller falls back to originals).
  - **Wired into both paths**: Simple path (StartMockRun) and orchestrated path (StartMockRunOrchestrated) both try synthesis first, fall back to original vision + UX context.
  - **Workflow graph**: New "Synthesize Specs" node between UX Approved and Generate Mock, with description and prompt mapping.

- Key Decisions:
  - DEC-008: Cache at {dataDir}/ux/mock-specs.md
  - Size gate: if synthesis >= originals combined, use originals instead

- Impact:
  - Files modified: promptfiles/defaults.go, handler/mock.go, WorkflowGraph.tsx
  - New artifact: {dataDir}/ux/mock-specs.md (cached synthesis output)

- Follow-ups:
  - Invalidate mock-specs.md cache when UX artifact is re-approved
  - Add a "Regenerate Specs" button in the mock UI
