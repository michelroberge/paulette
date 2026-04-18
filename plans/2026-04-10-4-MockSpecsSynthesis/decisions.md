## Decision: Mock Specs Synthesis Storage
ID: DEC-008

- Date: 2026-04-10
- Status: Accepted

- Context:
  The synthesis step produces a specs document from vision + UX. It should be cached so repeated mock generations don't re-synthesize.

- Decision:
  Store at {dataDir}/ux/mock-specs.md. Regenerate when UX artifact changes (or on explicit request).

- Rationale:
  Same directory as mock.html. Simple file check. The mock handler already writes to {dataDir}/ux/.

- Impact:
  - Affects: handler/mock.go, agent/mock.go
