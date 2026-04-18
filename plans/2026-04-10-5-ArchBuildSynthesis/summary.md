- Purpose:
  Add synthesis steps before Architecture and Build Plan stages to condense upstream artifacts, reducing context window usage — same pattern as the mock specs synthesis.

- Outcome:
  - **2 new prompts** in defaults.go:
    - `arch-specs-synthesize.md.tmpl` — condenses vision + UX into architecture-focused specs (functional reqs, data entities, API surface, integration points)
    - `build-specs-synthesize.md.tmpl` — condenses arch-specs + architecture into build-focused specs (components, dependencies, tech stack, validation commands)
  - **Reusable `synthesizeSpecs()` helper** in handler/synthesis.go — takes inputs, prompt name, cache path, runs LLM, applies size gate, caches result. Used by both architecture and build synthesis.
  - **`applySynthesis()` function** — called from chat handler before prompt assembly. For architecture stage: synthesizes vision+UX, replaces both in previousArtifacts. For build stage: uses arch-specs + architecture artifact as inputs, replaces all upstream artifacts.
  - **Build synthesis chains from arch synthesis** — if arch-specs.md exists (from architecture synthesis), it's used as input instead of raw vision+UX.
  - **Size gate** on both: if synthesis >= originals combined, originals are used instead.
  - **Cached** at `{dataDir}/architecture/arch-specs.md` and `{dataDir}/build/build-specs.md`.
  - **Workflow graph updated** with 2 new nodes: "Synthesize Arch Specs" before Architecture, "Synthesize Build Specs" before Build Plan. Both have descriptions, prompt mappings, and context artifact listings.

- Key Decisions:
  - Same pattern as DEC-008 (mock specs synthesis)
  - Build synthesis chains from arch synthesis output when available

- Impact:
  - New files: handler/synthesis.go
  - Modified: promptfiles/defaults.go, handler/chat.go, WorkflowGraph.tsx
