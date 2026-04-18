## User prompt

> when I try to authenticate with claude, I get Redirect URI http://10.0.0.215:8080/callback is not supported by client. I setup claude on this host, in the container, but paulette won't pick it up. Perhaps we're not looking in the right place?

Follow-up:

> let's go with — Uncomment the console URI (backend/internal/handler/auth.go:26) and use the manual code paste flow (LoginInput at line 256) — the callback page on console.anthropic.com shows a CODE#STATE string you paste into paulette's login box. But I don't understand, if I login on host, why is 1 failing then?

## Context

User is running paulette in Docker on a remote host (accessed via http://10.0.0.215:8080). Anthropic's OAuth client for Claude Code only whitelists `http://localhost:8080/callback` and `https://console.anthropic.com/oauth/code/callback` as redirect URIs, so a host-based deployment cannot use the localhost callback. The manual code-paste flow already exists in `LoginInput` (backend/internal/handler/auth.go:256), so the only change needed is to flip `claudeRedirectURI` to the console URI.
