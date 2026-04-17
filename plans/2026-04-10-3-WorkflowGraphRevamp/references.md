- Relevant Decisions:
  - [PromptExternalization] DEC-003: Workflow as standalone route
    - Impact: Already in place, just updating the graph content

- Reusable Patterns:
  - WorkflowGraph.tsx uses @xyflow/react + dagre — same stack, new layout
  - WorkflowNodeTypes.tsx has custom node components — add DecisionNode
  - WorkflowPromptEditor.tsx — add context artifacts display

- Actual Pipeline Flow (from codebase analysis):
  - Vision: Begin → Decision(guided/free) → Free: chat loop → Guided: refinement phases → Approved? → UX
  - UX: chat loop → Approved? → Mock generation → Architecture
  - Architecture: chat loop → Approved? → Build (extracts validation commands)
  - Build: chat → Parse plan → Bead loop (execute → review → branch) → Validation gate → Complete
  - Complete: Generate summary → Approve summary

- Risks / Conflicts:
  - Vertical cascade layout needs more height — may need scroll or zoom
  - Decision nodes (diamonds) need a new node type
  - The refinement loop has many sub-phases — need to balance detail vs readability
