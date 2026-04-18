## Decision: Graph Layout Direction
ID: DEC-007

- Date: 2026-04-10
- Status: Accepted

- Context:
  Current graph is left-to-right (LR). User wants a cascade (top-to-bottom).

- Decision:
  Change dagre rankdir from 'LR' to 'TB'. Adjust node handles from Left/Right to Top/Bottom.

- Impact:
  - Affects: WorkflowGraph.tsx, WorkflowNodeTypes.tsx
