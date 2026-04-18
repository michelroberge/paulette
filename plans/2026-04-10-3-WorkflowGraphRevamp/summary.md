- Purpose:
  Revamp the workflow graph to accurately represent the actual pipeline flow including loops, branches, approval gates, and the guided vs free vision mode.

- Outcome:
  - **TB cascade layout** — dagre rankdir changed to 'TB', all handles Top/Bottom
  - **Accurate flow graph** representing:
    - Begin → Decision (Guided vs Free) → two paths → merge after approval
    - Vision Chat loop with approval gate (refine or approve)
    - Guided path: Summarize → Generate Questions → Extract Facts → Score → Coherence → Critique → Converged? → loop or Synthesize
    - UX: chat loop → approval → Generate Mock → Architecture
    - Architecture: chat loop → approval → Build
    - Build: chat loop → approval → Parse Plan → Code Writer → Devil's Advocate → LGTM? → branch → All Done? → Validation Gate
    - Complete: Generate Summary → Approved? → Done
  - **DecisionNode** — diamond-shaped node for branch points (Guided?, Approved?, LGTM?, Converged?, All Done?)
  - **MergeNode** — small circle where paths rejoin after vision mode branch
  - **Selected node persistent highlight** — `.workflow-node-selected` class with blue glow, toggled via data.selected flag
  - **Context artifacts display** — "Context Artifacts: vision.md, ux-design.md" bar in prompt editor, mapped per node via NODE_CONTEXT_MAP
  - **Dashed animated edges** for loop-back paths with labels (refine, issues, next bead, no)

- Key Decisions:
  - DEC-007: TB layout direction

- Impact:
  - Files modified: WorkflowGraph.tsx (complete rewrite of graph definition), WorkflowNodeTypes.tsx (new DecisionNode, MergeNode, TB handles, selected prop), WorkflowPromptEditor.tsx (contextArtifacts prop), App.css (decision/merge/selected/context styles)
