## Decision: Connection Prompt Storage Location
ID: DEC-005

- Date: 2026-04-10
- Status: Accepted

- Context:
  Need a directory per connection to store prompt templates. Connections are currently in a flat file (~/.paulette/connections.json).

- Options Considered:
  - Option A: ~/.paulette/connection-prompts/{connectionId}/
  - Option B: Embed prompts in connections.json as a map field

- Decision:
  Option A — separate directory per connection

- Rationale:
  Consistent with project prompt storage pattern (.paulette/prompts/). Template files are text, not JSON fields. Easy to browse/edit outside the app.

- Consequences:
  - Positive:
    - Clean separation, reuses PromptStore directly
    - Files are human-readable and editable
  - Negative:
    - Directory lifecycle tied to connection CRUD (create/delete dirs)

- Impact:
  - Affects: PromptStore, ConnectionStore, connection handler
  - Related Plans: 2026-04-10-1-PromptExternalization

---

## Decision: Prompt Resolution Order (3-tier)
ID: DEC-006

- Date: 2026-04-10
- Status: Accepted

- Context:
  With connection-level prompts, need to define the fallback chain.

- Options Considered:
  - Option A: project → connection → hardcoded
  - Option B: project → hardcoded (ignore connection prompts at runtime)

- Decision:
  Option A — project → connection → hardcoded

- Rationale:
  The whole point is that connection prompts are the "default for this model class" and project prompts are "tuned for this specific project". A user sets up Ollama connection with simple prompts, then every project using that connection inherits them automatically, with per-project overrides still possible.

- Consequences:
  - Positive:
    - One-time setup per connection benefits all projects
    - Projects can still override
  - Negative:
    - PromptStore.Load needs a fallback chain (or caller composes)

- Impact:
  - Affects: promptStoreForProject helper, chat handler, bead handler, refinement handler
  - Related Plans: 2026-04-10-1-PromptExternalization
