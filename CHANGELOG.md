# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-02-27

### Added

#### Core Task Engine
- **V1–V6 MVP**: Task CRUD (`POST /tasks`, `GET /tasks`, `PATCH /tasks/:id`, `POST /tasks/:id/claim`, `GET /tasks/poll`), SQLite persistence, FSM state machine (pending → claimed → in_progress → done/failed), Discord webhook notifications
- **V7**: `superseded_by` field for task supersession; `depends_on` dependency graph; `blocked_downstream` status propagation
- **V8**: Chain notifications (`notify_ceo_on_complete`); `retry_routing` table for auto-reassignment; triggered dispatch bug fix
- **V9**: `RetryQueue` with exponential backoff for failed tasks; stale task ticker (auto-fail tasks exceeding max dispatch time)
- **V10**: Two-stage review-reject chain in `autoRetry`; `isReviewReject` detection; 4 seed routing rules (vision/pm/ops)
- **V11**: Per-agent webhook channel routing (`agent_channel_map`); stale task `max_dispatches` limit; `cancelled` terminal state (no retry, no downstream unlock)
- **V13**: `autoAdvance` — success-path dispatch symmetric to `autoRetry` (next agent auto-dispatched on task completion)
- **V14**: Result routing — JSON `next_agent` field in task result triggers follow-up task dispatch
- **V16**: Agent task timeout auto-fail (configurable per-agent deadline; ticker marks timed-out tasks as failed)
- **V19**: `store.SummaryFiltered(assignedTo)` — `GET /tasks/summary?assigned_to=` filtering; `ORDER BY priority DESC, created_at ASC` for task lists
- **V22**: `POST /api/tasks/bulk` — batch cancel (FSM bypass, writes history) and reassign operations
- **V25-A**: FSM allows `claimed → cancelled` and `in_progress → cancelled` transitions (align PATCH path with bulk cancel)

#### Dispatch & Orchestration
- **V8**: `POST /dispatch` with `notify_ceo_on_complete` support for single-task CEO notification
- **V8**: `POST /dispatch/chain` — linear serial chain (A→B→C) with shared `chain_id`
- **V15**: `POST /dispatch/from-template/:name` — task templates CRUD (`GET/POST /templates`, `DELETE /templates/:id`)
- **V17**: Human approval node in dispatch chains; frontend `GoalInputPage` for natural-language task creation
- **V18**: `POST /dispatch/graph` — arbitrary DAG dispatch (Kahn BFS dependency resolution, shared `CreateGraph` transaction)
- **V19**: `PATCH /tasks/:id` dynamic priority (`priority *int`: 0=normal/1=high/2=urgent; bypasses FSM)

#### API & Documentation
- **V12**: AI Workbench skeleton — Vue 3 + TypeScript + Vite + Tailwind frontend; `embed.FS` static serving; OpenAPI-first design; `/api/` prefix for all new endpoints
- **V17**: SSE real-time updates (`GET /events`; `SSEHub.Broadcast` on task state changes)
- **V18**: `GET /tasks?search=` full-text search (SQLite LIKE on title + description)
- **V20**: `GET /docs` Scalar API documentation (CDN integration, inline OpenAPI 3.1 spec, 18 endpoints, 6 schemas); `GET /openapi.json`
- **V20**: `GET /api/graph/:chain_id` — DAG graph endpoint (tasks + depends_on relationships)
- **V21**: `GET /api/agents/stats` — per-agent aggregated statistics (total/done/failed/avg_duration_minutes/success_rate)
- **V23-A**: `GET /api/config` returns `version` and masked `outbound_webhook_url`
- **V24-B**: `GET /api/tasks/:id/comments`, `POST /api/tasks/:id/comments` — task comment threads with SSE broadcast on new comment

#### Notifications
- **V23-A**: `OutboundWebhookNotifier` — HMAC-SHA256 signed HTTP POST on done/failed/cancelled; goroutine best-effort, 5s timeout; `MultiNotifier` fan-out; `AGENT_QUEUE_WEBHOOK_URL` + `AGENT_QUEUE_WEBHOOK_SECRET` env vars

#### Frontend (AI Workbench Web UI)
- **V12**: Core pages — `DashboardPage`, `KanbanPage`, `TaskDetailPage`, `GoalInputPage`
- **V17**: Human approval UI; real-time task updates via SSE
- **V20**: `GraphVisualizationPage` — DAG topology visualization; Kahn BFS level layout; status-color nodes (gray/yellow/blue/green/red); click-to-navigate
- **V21**: `AgentStatsPage` — responsive card grid, success rate progress bars (green ≥80% / yellow ≥50% / red <50%)
- **V21**: `DashboardPage` search bar — debounce 300ms, client-side filter, absolute-position overlay results
- **V22**: `DashboardPage` multi-select toolbar — checkbox selection, bulk cancel/reassign, blue highlight
- **V22**: Mobile responsive layout — hamburger menu, Sidebar hidden on `< lg`, `grid-cols-1 md:grid-cols-2` breakpoints
- **V23-A**: `SettingsPage` — system info, webhook status (masked URL + trigger events), agents list
- **V23-B**: `TaskDetailPage` timeline duration badges (`Xm Ys` format, monospace); chain inline view (current task highlighted blue, click-to-navigate siblings)
- **V24-A**: i18n dual-language (Chinese/English) via `vue-i18n@9`; locale toggle in navbar and sidebar; 6 namespaces, ~40 keys; `localStorage` persistence
- **V24-B**: `TaskDetailPage` comment section — avatar initials, author/timestamp, `Ctrl+Enter` submit, SSE-driven refresh

#### Developer Experience
- **P3**: `CONTRIBUTING.md`, `docs/guides/agent-integration.md`, `docs/guides/configuration.md`
- **P3**: GitHub Actions CI/CD workflows (test / build / release)
- **P3**: OpenAPI spec + generated TypeScript types

### Changed

- Project renamed from `agent-queue` to **ainative** (README, docs); binary name remains `agent-queue`
- `store.Summary()` refactored to call `store.SummaryFiltered("")` (backward compatible)
- `GET /tasks` default sort changed to `ORDER BY priority DESC, created_at ASC` (V19)
- `AppLayout.vue` navigation items converted to computed (i18n-driven, V24-A)

### Fixed

- **V10.1**: `isReviewReject` logic fix; vision/pm/ops seed routing rules corrected
- **V11**: `cancelled` state correctly excluded from retry and downstream unlock
- **V19**: `GET /tasks/summary?assigned_to=` now filters correctly (was returning global stats regardless of param)
- **V23-A**: `tasksSummary` handler no longer ignores `*http.Request` parameter
- **V25-A**: `PATCH /tasks/:id {status: "cancelled"}` now works for `claimed` and `in_progress` tasks (previously FSM rejected these transitions)
- CEO notification deduplication (two root causes fixed in `0ba05a4`)
- Retry routing deduplication + `UNIQUE` index for idempotent seed (`34b43a0`)

[1.0.0]: https://github.com/irchelper/ainative/releases/tag/v1.0.0
