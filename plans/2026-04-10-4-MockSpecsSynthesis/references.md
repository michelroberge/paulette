- Relevant Decisions:
  - [PromptExternalization] DEC-001: Go text/template for externalized prompts
    - Impact: The new synthesis prompt should also be externalized

- Reusable Patterns:
  - BuildMockContext (agent/mock.go:360) already truncates vision to 2000 chars
  - Mock handler (handler/mock.go) loads artifacts at lines 219, 248-249, 994, 1003-1004
  - PromptStore / promptfiles package for externalized prompts
  - artifactRepo.ReadWithFallback for loading stage artifacts

- Key Injection Points:
  - Simple path: mock.go:248-253 — loads vision + build, calls BuildMockContext, appends UX
  - Orchestrated path: mock.go:1003-1005 — same pattern
  - Both paths need the synthesized specs instead of raw vision + UX

- Risks / Conflicts:
  - Synthesis is an extra LLM call adding latency — should be cached
  - Need to store synthesized specs on disk so it's not regenerated each mock attempt
