# ── Stage 1: Build React frontend ──────────────────────────────────────────
FROM node:22-bookworm-slim AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json  ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# ── Stage 2: Build Go binary ───────────────────────────────────────────────
FROM golang:1.26-bookworm AS go-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# embed.go expects static/ inside the backend directory
COPY --from=frontend-builder /app/frontend/dist/ ./static/
RUN CGO_ENABLED=0 GOOS=linux go build -o /paulette .

# ── Stage 3: Lightweight runtime ───────────────────────────────────────────
FROM node:22-bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    ca-certificates \
    openssh-client \
    gosu \
    && rm -rf /var/lib/apt/lists/* \
    # Dolt — standalone binary, used by beads for version-controlled issue tracking
    && curl -L https://github.com/dolthub/dolt/releases/latest/download/install.sh | bash \
    # Claude Code CLI — required for all AI agent calls
    && npm install -g @anthropic-ai/claude-code \
    # beads (bd) — issue tracker CLI; npm primary, curl script fallback
    && (npm install -g @beads/bd 2>/dev/null || curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash) \
    && bd --version \
    # rtk — token optimizer (optional, non-fatal if install fails)
    && (curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh || echo "rtk install skipped (optional)")

# Copy paulette binary and entrypoint from the Go build stage
COPY --from=go-builder /paulette /usr/local/bin/paulette
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh && \
    useradd -m -s /bin/bash paulette && \
    mkdir -p /home/paulette/.paulette /home/paulette/repos \
             /home/paulette/.claude \
             /home/paulette/.config/bd && \
    printf 'no-db: true\n' > /home/paulette/.config/bd/config.yaml && \
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
