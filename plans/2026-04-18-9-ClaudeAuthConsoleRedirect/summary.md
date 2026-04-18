# Summary: Claude auth for remote-host paulette deployments

## Problem
Paulette running on `http://10.0.0.215:8080` couldn't complete Claude OAuth — Anthropic rejected the LAN-IP redirect URI, and the host's credentials weren't picked up by the container.

## Changes
1. **`backend/internal/handler/auth.go`** — switched `claudeRedirectURI` from `http://localhost:8080/callback` to `https://console.anthropic.com/oauth/code/callback`. The existing `LoginInput` handler (auth.go:256) already accepts the `CODE#STATE` string pasted from the console success page, so no flow code changed.
2. **`docker-compose.override.yml`** (new, gitignored) — replaces the ro single-file mount of `~/.claude/.credentials.json` with a rw directory mount of `${HOME}/.claude`. Fixes `~`-resolves-to-root-under-sudo, avoids the missing-file-becomes-directory trap, and lets `claude login` run from either host or container.

## User flow post-change
1. Open paulette → Login panel.
2. Click the auth URL → sign in at claude.ai.
3. Copy the `CODE#STATE` string shown on `console.anthropic.com`'s success page.
4. Paste into paulette's login input → credentials written to `~/.claude/.credentials.json` and status flips to authenticated.

## Not done
- Port-from-config for the redirect URI (TODO still present at auth.go:27).
- Documenting the uid-alignment assumption (override assumes host user uid == container paulette uid 1000).

## Related
- Plan: `plans/2026-04-18-9-ClaudeAuthConsoleRedirect/`
- Decisions: DEC-001 (redirect URI swap), DEC-002 (override mount)
