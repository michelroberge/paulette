# Paulette

A structured AI-assisted product development pipeline that guides projects through five traceable stages: **Vision**, **UX**, **Architecture**, **Build**, and **Complete**.

Paulette is designed for solo founders, product-minded developers, and AI-first small teams who want to maintain context and traceability across the full product lifecycle -- from idea to working code.

## Key Features

- **Five-stage pipeline** -- each stage's output feeds directly into the next, preventing context loss between phases
- **Pluggable LLM providers** -- route each stage to a different model backend:
  - Ollama, LM Studio (local/free)
  - OpenAI, Anthropic, Google Gemini (cloud API)
  - Claude CLI (subprocess)
- **Per-project configuration** -- global defaults with project-level overrides for stage-to-provider assignments
- **Build task DAG** -- visual dependency graph for code generation tasks
- **Integrated code editor** -- Monaco-based editor with markdown preview
- **Docker-ready** -- single-container deployment with embedded frontend

## Tech Stack

| Layer | Technology |
|-------|------------|
| Backend | Go (chi router) |
| Frontend | React 19, TypeScript, Vite |
| Styling | Tailwind CSS |
| Auth | OIDC / OAuth2 (optional) |
| Container | Docker (multi-stage build) |

## Getting Started

### Prerequisites

- Go 1.26+
- Node.js 22+
- (Optional) Docker & Docker Compose

### Development

```sh
# Frontend
cd frontend && npm run dev

# Backend
cd backend && go run main.go
```

### Docker

```sh
# Production
docker-compose up

# Development (with hot reload)
docker-compose -f docker-compose.dev.yml up
```

### Configuration

Copy `.env.example` to `.env` and set the required variables. At minimum, configure a `PORT` and `REPOS_PATH`. Enable OIDC variables if you need authentication.

Provider connections are managed through the UI at the **Configure** page.

## Documentation

- [Vision](docs/0.2.0/vision/vision.md)
- [UX Design](docs/0.2.0/ux/ux.md)
- [UX Mock](docs/0.2.0/ux/mock.html)
- [Architecture](docs/0.2.0/architecture/architecture.md)
- [Iteration Summary](docs/0.2.0/summary.md)

## License

All rights reserved.
