# Plan: Mock Specs Synthesis Step

## Goal
Add a synthesis step before mock generation that condenses vision + UX design into a single specs document. Use it as the sole context for mock gen if it's smaller than the originals combined.

## Approach

### 1. New prompt: mock-specs-synthesize
- Add to promptfiles/defaults.go
- Instructs the AI to produce a concise specs document from vision + UX
- Output: markdown with screen specs, layout, interactions — optimized for mock HTML gen

### 2. Backend: synthesizeMockSpecs function in handler/mock.go
- Load vision + UX artifacts
- Call LLM with the synthesis prompt
- Compare sizes: if synthesis < vision + UX, save to {dataDir}/ux/mock-specs.md
- Otherwise skip (flag to use originals)
- Cache: if mock-specs.md exists and is newer than UX artifact, reuse it

### 3. Modify mock generation to use specs
- In StartMockRun: check for mock-specs.md, use it as sole context
- In StartMockRunOrchestrated: same
- Fallback: if specs missing or bigger, use original artifacts

### 4. Workflow graph: add node between ux-approved and ux-mock

### 5. Add externalized prompt template

## Related Decisions
- DEC-008: Store at {dataDir}/ux/mock-specs.md
