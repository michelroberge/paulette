# ── Stage 1: Build React frontend ──────────────────────────────────────────
FROM node:22-bookworm-slim AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json  ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# ── Stage 2: Build Go binary + bd (beads) ─────────────────────────────────
FROM golang:1.26-bookworm AS go-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# embed.go expects static/ inside the backend directory
COPY --from=frontend-builder /app/frontend/dist/ ./static/
RUN CGO_ENABLED=0 GOOS=linux go build -o /paulette .

# Build bd from source against bookworm's libicu72 so the runtime binary
# links libicui18n.so.72 (available in bookworm) instead of .so.74 (trixie-only).
RUN apt-get update \
    && apt-get install -y --no-install-recommends libicu-dev \
    && rm -rf /var/lib/apt/lists/*
RUN CGO_ENABLED=1 go install github.com/steveyegge/beads/cmd/bd@latest

# ── Stage 3: Lightweight runtime ───────────────────────────────────────────
FROM node:22-bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    ca-certificates \
    openssh-client \
    gosu \
    libicu72 \
    && rm -rf /var/lib/apt/lists/* \
    # Dolt — standalone binary, used by beads for version-controlled issue tracking
    && curl -L https://github.com/dolthub/dolt/releases/latest/download/install.sh | bash \
    # Claude Code CLI — required for all AI agent calls
    && npm install -g @anthropic-ai/claude-code \
    # rtk — token optimizer (optional, non-fatal if install fails)
    && (curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh || echo "rtk install skipped (optional)")

# Copy paulette binary, bd binary, and entrypoint
COPY --from=go-builder /paulette /usr/local/bin/paulette
COPY --from=go-builder /go/bin/bd /usr/local/bin/bd
RUN ln -s /usr/local/bin/bd /usr/local/bin/beads
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh && \
    usermod -u 999 node && groupmod -g 999 node && \
    groupadd -g 1000 paulette && \
    useradd -u 1000 -g 1000 -m -s /bin/bash paulette && \
    mkdir -p /home/paulette/.paulette /home/paulette/repos \
             /home/paulette/.claude \
             /home/paulette/.config/bd && \
    printf '%s' \
        '{"permissions":{"allow":[' \
        '"Write(/home/paulette/repos/**)",' \
        '"Edit(/home/paulette/repos/**)",' \
        '"Bash(*)",' \
        '"Read(/home/paulette/repos/**)"' \
        ']}}' \
        > /home/paulette/.claude/settings.json && \
    chown -R paulette:paulette /home/paulette/.paulette \
                               /home/paulette/repos \
                               /home/paulette/.claude \
                               /home/paulette/.config && \
    # Trust all directories (needed for volume-mounted repos from the host OS)
    su paulette -c "git config --global --add safe.directory '*'"

WORKDIR /home/paulette

ENV HOME=/home/paulette \
    REGISTRY_PATH=/home/paulette/.paulette \
    REPOS_PATH=/home/paulette/repos \
    CLAUDE_PATH=claude \
    PORT=8080

EXPOSE 8080
# Entrypoint runs as root, fixes permissions on bind-mounted repos, then execs paulette user
ENTRYPOINT ["/entrypoint.sh"]
