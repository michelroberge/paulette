# Claudette

**An AI-powered application generator built on Claude.**

## The Name

Claudette was a colleague from my early career who specialized in mainframe development. She was thorough, understood things quickly, and brought light to everyone's days. She had a rare talent for working with you to solve any problem — professional or personal.

I have no idea where Claudette is today, but I'd like to think she's doing well. She's that kind of woman.

Naming this tool after her felt right: put some light on Claude.

## What Is Claudette?

Claudette is an application generator that guides you through a structured development pipeline using conversational AI. Each stage has a dedicated expert you interact with to produce working artifacts.

**The flow:** Vision &rarr; UX Design &rarr; Architecture &rarr; Build &rarr; Review &rarr; Repeat

## How It Works

### Step 1 — Create Your Project

Specify a host directory and get started.

### Step 2 — Clarify Your Vision

Interact with the vision advisor until you have a clear vision document. Approve it when ready, and the UX designer takes over.

### Step 3 — User Journeys

This is where the magic starts. User journeys are generated so you can confirm what will actually be built matches your intent. Pick your favorite UI framework and preview the outcome. Continue tuning with the UX advisor until you're happy, then approve.

### Step 4 — Architecture

The architect receives all prior context and recommends the best architecture for your needs (powered by Opus). Every architectural choice is tied back to user journeys — this matters during the build. Discuss, negotiate, then approve.

### Step 5 — Build

A build agent creates a development plan: the logical sequence for implementing the architecture and user journeys. It then breaks this into **beads** — small, atomic development items with dependencies and prerequisites.

During the build phase, **n parallel agents** (your choice) execute the plan. Every bead is challenged by a devil's advocate for up to 3 iterations before completion. Progress syncs with `bd` so you can track status, dependencies, and blockers in real time.

When all beads are complete, your app is done.

### Autonomous Mode

Toggle autonomous mode and Claudette builds your entire app end-to-end. It can finish a build cycle, review the result, and feed recommendations into an enhancement loop for continuous improvement.

## Tech Stack

- **Backend:** Go (API server, pipeline orchestration, agent management)
- **Frontend:** React + TypeScript (Vite)
- **AI:** Claude API (Anthropic)
- **Issue Tracking:** [beads](https://github.com/beads-project/beads) (`bd`)

## Getting Started

```bash
# Backend
cd backend
go run .

# Frontend
cd frontend
npm install
npm run dev
```

## Project Structure

```
backend/
  main.go                    # Entry point
  internal/
    agent/                   # AI agent orchestration & prompts
    config/                  # Configuration
    handler/                 # HTTP handlers
    model/                   # Data models (project, bead, pipeline)
    pipeline/                # Pipeline state machine
    repository/              # File-system persistence
    server/                  # HTTP server setup
    stream/                  # SSE streaming
    git/                     # Git integration
frontend/
  src/                       # React application
```

## License

MIT
