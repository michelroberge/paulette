# Plan: Switch Claude OAuth redirect URI to console callback

## Goal

Make Claude Code OAuth work for paulette instances that are not reachable at `http://localhost:8080` (e.g. remote host deployments accessed via LAN IP).

## Change

- In `backend/internal/handler/auth.go`:
  - Set `claudeRedirectURI = "https://console.anthropic.com/oauth/code/callback"`.
  - Keep the `LoginInput` handler (line 256) as the completion path — user pastes the `CODE#STATE` string shown on console.anthropic.com's success page.
  - The in-container `/callback` route (`Callback` handler) becomes dead code for the Claude OAuth flow but is left in place in case someone reverts to the localhost redirect.

## Why the LAN-access path is blocked

Anthropic's public OAuth client id `9d1c250a-e61b-44d9-88ed-5944d1962f5e` whitelists only two redirect URIs: `http://localhost:8080/callback` and `https://console.anthropic.com/oauth/code/callback`. Rewriting the redirect to `http://10.0.0.215:8080/callback` is rejected server-side with "Redirect URI … is not supported by client".

## Out of scope

- Port-from-config rewrite (the TODO at auth.go:27) — separate task.
- Fixing the silent-failure mode where `~/.claude/.credentials.json` doesn't exist on the host and Docker auto-creates a directory at the mount target. That's a deployment-docs change, not a code change.

## Verification

- `go build ./backend/...` succeeds.
- Manual: click Login in paulette UI → follow the URL → sign in at claude.ai → copy the `CODE#STATE` string from the console success page → paste into paulette's input → verify `~/.claude/.credentials.json` is written inside the container (or on the host if the mount is writable) and the status badge flips to authenticated.
