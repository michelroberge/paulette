**CRITICAL** Track plans
- ALWAYS write prompt to `plans/yyyy-MM-dd-<increment>-<planName>/prompt.md` (create dir if not exist)
- ALWAYS write plans to `plans/yyyy-MM-dd-<increment>-<planName>/plan.md` (create dir if not exist)
- This must be your first task once a plan is approved.

**CRITICAL** Track decisions
- ALWAYS summarize decisions to `plans/yyyy-MM-dd-<sequential-increment>-<planName>/decisions-<3-digit-increment>.md` (create dir if not exist)
- This includes:
  - All questions & decisions done in plan
  - Architectural decisions
  - Pattern standardizations

**CRITICAL** Produce summary on plan completion
- ALWAYS generate a summary of the plan in `plans/yyyy-MM-dd-<sequential-increment>-<planName>/summary.md` (create dir if not exist)
- This must be concise enough to have value later and be compact enough to not bust context.

## `decisions.md` format

```md
## Decision: <short title>
ID: DEC-XXX

- Date: YYYY-MM-DD
- Status: Proposed | Accepted | Deprecated | Superseded

- Context:
  <What problem are we solving?>

- Options Considered:
  - Option A: <short description>
  - Option B: <short description>

- Decision:
  <What was chosen?>

- Rationale:
  <Why this option was chosen>

- Consequences:
  - Positive:
    - <benefit>
  - Negative:
    - <tradeoff>

- Impact:
  - Affects: <components / files / systems>
  - Related Plans: <plan names>

- References:
  - <links to other decisions or summaries>
```

