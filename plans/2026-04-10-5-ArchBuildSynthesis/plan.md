# Plan: Architecture & Build Plan Synthesis Steps

## Goal
Add synthesis steps before Architecture and Build Plan stages to condense upstream artifacts, reducing context window usage.

## Architecture Synthesis
- Input: vision + UX design
- Output: concise arch-specs focused on technical requirements, data models, API surface, constraints
- Cache: {dataDir}/architecture/arch-specs.md
- Size gate: skip if synthesis >= vision + UX combined
- Injected into architecture prompt as VisionArtifact + UXArtifact replacements

## Build Synthesis  
- Input: arch-specs (or vision+UX if arch-specs was skipped) + architecture artifact
- Output: concise build-specs focused on what needs to be built, components, dependencies
- Cache: {dataDir}/build/build-specs.md
- Size gate: skip if synthesis >= inputs combined
- Injected into build prompt as all 3 artifact replacements

## Implementation
1. Two new prompts in defaults.go: arch-specs-synthesize.md.tmpl, build-specs-synthesize.md.tmpl
2. Reusable synthesizeSpecs() helper in handler — takes inputs, prompt name, cache path, provider
3. Wire into chat handler: before prompt assembly for architecture/build stages, try synthesis
4. Pass synthesized content into previousArtifacts map (overwriting originals)
5. Update workflow graph with 2 new nodes + descriptions + prompt mappings

## Related Decisions
- Same pattern as DEC-008 (mock specs synthesis)
