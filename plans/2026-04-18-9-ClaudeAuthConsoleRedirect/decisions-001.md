## Decision: Claude OAuth redirect URI — use console callback
ID: DEC-001

- Date: 2026-04-18
- Status: Accepted

- Context:
  Paulette's Claude OAuth flow hardcoded `redirect_uri=http://localhost:8080/callback`, which is one of only two URIs whitelisted for the shared Claude Code OAuth client (`9d1c250a-e61b-44d9-88ed-5944d1962f5e`). Users running paulette on a remote host (e.g. http://10.0.0.215:8080) cannot reach that redirect from their browser, and swapping the redirect to the LAN IP is rejected server-side ("Redirect URI http://10.0.0.215:8080/callback is not supported by client").

- Options Considered:
  - Option A: Keep localhost redirect and document SSH tunneling as the workaround.
  - Option B: Switch to the console redirect URI (`https://console.anthropic.com/oauth/code/callback`) and rely on the existing manual-code-paste flow (`LoginInput`).
  - Option C: Register a new OAuth client with paulette-specific redirect URIs — requires Anthropic coordination.

- Decision:
  Option B.

- Rationale:
  The `LoginInput` handler at backend/internal/handler/auth.go:256 already accepts both bare `CODE#STATE` strings and pasted callback URLs, so no flow changes are needed. This unblocks all deployment topologies (localhost, LAN, reverse proxy) with a one-line swap.

- Consequences:
  - Positive:
    - Works on any host/port combination without OAuth-client changes.
    - No new infrastructure or registration steps.
  - Negative:
    - Users must copy a string from the console success page (one extra click vs. auto-redirect).
    - The in-container `/callback` route becomes dead code for Claude auth (left in place; handles the localhost case if reverted).

- Impact:
  - Affects: `backend/internal/handler/auth.go`
  - Related Plans: 2026-04-18-9-ClaudeAuthConsoleRedirect

- References:
  - None

---

## Decision: Host Claude credentials mount — use `${HOME}/.claude` rw
ID: DEC-002

- Date: 2026-04-18
- Status: Accepted

- Context:
  `docker-compose.yml` mounts `~/.claude/.credentials.json` read-only into the container. This fails silently in several ways: if the host file doesn't exist when compose first runs, Docker auto-creates a *directory* at the mount target; `sudo docker compose up` resolves `~` to `/root` instead of the invoking user; and `claude login` from inside the container can't persist because the mount is read-only.

- Options Considered:
  - Option A: Document the pitfalls and keep the ro file mount.
  - Option B: Ship a `docker-compose.override.yml` that mounts the entire `${HOME}/.claude` directory read-write.
  - Option C: Change the committed `docker-compose.yml` to mount the directory rw for everyone.

- Decision:
  Option B (override file, gitignored).

- Rationale:
  Mounting the whole `.claude` directory rw lets `claude login` run from either host or container, avoids the missing-file-creates-directory trap, and is explicit about `${HOME}` resolution. Keeping it in the gitignored override means the committed compose file stays conservative (ro, single file) while the local deployment gets the ergonomic setup. Assumes host user uid matches the container's `paulette` user (uid 1000).

- Consequences:
  - Positive:
    - `claude login` works from inside the container and persists to the host.
    - No `~` resolution surprises.
  - Negative:
    - Container has rw access to the full host `.claude` directory (session history, plans, caches) — fine for local dev, noteworthy for multi-tenant.
    - Assumes uid 1000 alignment; a uid mismatch would cause permission errors on writes.

- Impact:
  - Affects: `docker-compose.override.yml` (new, gitignored)
  - Related Plans: 2026-04-18-9-ClaudeAuthConsoleRedirect

- References:
  - None
