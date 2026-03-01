# ainative

**ainative** is a lightweight task queue for coordinating multiple AI agents via HTTP.

- **SQLite-backed**: task state is durable (no "lost context" when an agent/session crashes).
- **Autonomous chaining**: dependencies unlock downstream tasks automatically.
- **Local-first**: single binary, zero external dependencies.

For the full architecture and API details, see:
- `docs/ARCH.md`
- `CHANGELOG.md`
