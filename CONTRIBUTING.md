# Contributing to ainative

Thank you for your interest in contributing! This document covers the development setup, coding conventions, and PR process.

---

## Getting Started

### Prerequisites

- Go 1.22+
- Node.js 20+ (for frontend development)
- `jq` (for running shell-based tests)

### Setup

```bash
git clone https://github.com/irchelper/ainative.git
cd ainative

# Backend only
go build -o agent-queue .
go test ./...

# Full stack (frontend + backend)
make build-web   # build frontend → dist/
make build       # compile binary with embedded frontend
```

---

## Project Structure

```
ainative/
├── cmd/server/         # Entry point (main.go)
├── internal/
│   ├── handler/        # HTTP handlers (~1000 lines)
│   ├── store/          # SQLite queries
│   ├── model/          # Data types
│   ├── notify/         # Notifier interface + SessionNotifier + RetryQueue
│   ├── db/             # Schema init + migrations
│   ├── fsm/            # State machine validation
│   ├── failparser/     # Result parsing for retry routing
│   ├── openclaw/       # OpenClaw client (optional dependency)
│   └── config/         # YAML + env config loading
├── web/                # Frontend SPA (Vue 3 + TS + Vite + Tailwind)
├── docs/
│   ├── ARCH.md         # Architecture decisions
│   ├── PRD.md          # Product requirements
│   └── guides/         # User guides
├── scripts/            # launchd install scripts
└── Makefile
```

---

## Development Workflow

### Backend

```bash
# Run tests (with race detector)
make test

# Run a single package
go test ./internal/handler/... -v

# Vet
make vet

# Run locally (no frontend)
go run ./cmd/server --port 19827 --db data/queue.db
```

### Frontend

```bash
cd web
npm install
npm run dev       # Vite dev server at :5173, proxies API to :19827
npm test          # Vitest unit tests
npm run build     # Build to ../dist/
npm run lint      # ESLint + Prettier check
```

### Full stack dev

```bash
# Terminal 1 — Go API server
make dev-api

# Terminal 2 — Vite hot reload
cd web && npm run dev
# Open http://localhost:5173
```

---

## Coding Guidelines

### Go

- **No new external dependencies** without discussion — the goal is a lean, auditable binary
- Keep `internal/handler/handler.go` under 1500 lines; split when approaching the limit
- All new DB columns via `ALTER TABLE ... ADD COLUMN` (idempotent, same pattern as existing migrations)
- New API endpoints under `/api/` prefix; existing paths (`/tasks`, `/dispatch`, etc.) are frozen
- Tests live next to the code (`handler_test.go`, `store_test.go`, etc.)
- Use table-driven tests for state machine / routing logic

### Frontend (Vue 3 + TypeScript)

- Composition API with `<script setup>` — no Options API
- TypeScript strict mode; no `any` without a comment explaining why
- Components in `web/src/components/`; pages in `web/src/pages/`
- State via Pinia stores — no prop-drilling beyond 2 levels
- Polling via `usePolling` composable — pause on tab hidden, exponential backoff on error

### Commits

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add /api/timeline/:id endpoint
fix: retry_routing UNIQUE index prevents duplicate seeds
docs: update ARCH.md with V12 workbench architecture
chore: add dist/ and web/node_modules/ to .gitignore
test: add TestOnTaskComplete_SingleTask cases
```

Types: `feat`, `fix`, `docs`, `test`, `chore`, `refactor`, `perf`

---

## Pull Request Process

1. **Fork** the repo and create a branch: `git checkout -b feat/your-feature`
2. **Write tests** for new behavior (Go: `go test ./...`; frontend: `npm test`)
3. **Run all checks** before opening a PR:
   ```bash
   make test    # Go tests (race detector on)
   make vet     # go vet
   cd web && npm run lint && npm test
   ```
4. **Open a PR** with:
   - What the change does (one paragraph)
   - Why it's needed
   - How to test it manually
5. **CI must pass** — test.yml runs Go tests on ubuntu + macos, frontend tests on ubuntu
6. **One approving review** required before merge

---

## Reporting Issues

- **Bug**: Include Go version, OS, steps to reproduce, actual vs expected behavior
- **Feature request**: Describe the use case first, not just the solution
- **Security**: Email directly (see repo contact) — do not open a public issue

---

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](./LICENSE).
