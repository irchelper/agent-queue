# Configuration Reference

ainative works with zero configuration out of the box. All settings are optional and can be provided via environment variables or a `config.yaml` file.

---

## Configuration Priority

Settings are resolved in this order (highest priority first):

1. **Command-line flags** (`--port`, `--db`, `--config`, `--static-dir`)
2. **Environment variables** (`AGENT_QUEUE_*`)
3. **Config file** (`config.yaml` in working directory, or `--config=path`)
4. **Defaults**

---

## Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `19827` | HTTP listen port |
| `--db` | `data/queue.db` | SQLite database path |
| `--config` | `config.yaml` | Config file path |
| `--static-dir` | _(empty)_ | Local static files dir (dev mode); empty = use embed.FS |

```bash
./agent-queue --port 8080 --db /var/lib/ainative/queue.db
```

---

## Environment Variables

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_QUEUE_PORT` | `19827` | HTTP listen port |
| `AGENT_QUEUE_DB_PATH` | `data/queue.db` | SQLite database path. Use an absolute path when running via launchd/systemd to avoid working-directory issues. |

### Notifications

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_QUEUE_DISCORD_WEBHOOK_URL` | — | Discord Incoming Webhook URL. Receives `done` and `failed` events for all tasks. |
| `AGENT_QUEUE_AGENT_WEBHOOKS` | — | Per-agent webhook overrides. Format: `agent1=url1,agent2=url2`. Routes `done`/`failed` by `assigned_to`; falls back to the global webhook on miss. |

### OpenClaw Integration

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_QUEUE_OPENCLAW_API_URL` | `http://localhost:18789` | OpenClaw gateway URL for `sessions_send` (agent dispatch and CEO notifications). |
| `AGENT_QUEUE_OPENCLAW_API_KEY` | — | OpenClaw gateway token. |

### Stale Task Recovery

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_QUEUE_STALE_CHECK_INTERVAL` | `10m` | How often to scan for stale (unclaimed/stuck) tasks. |
| `AGENT_QUEUE_STALE_THRESHOLD` | `30m` | A task is considered stale if `updated_at` has not changed within this duration while in `pending` or `in_progress`. |
| `AGENT_QUEUE_MAX_STALE_DISPATCHES` | `3` | Maximum number of stale re-dispatch attempts before alerting the CEO session. |

---

## Config File (config.yaml)

Place `config.yaml` in the working directory, or specify with `--config=path`.

```yaml
server:
  port: 19827
  db: data/queue.db

agents:
  known:
    - name: coder
      label: Engineer
    - name: thinker
      label: Architect
    - name: writer
      label: Technical Writer
    - name: qa
      label: QA Engineer
    - name: devops
      label: DevOps
    # Add custom agents here

timeouts:
  agent_minutes: 30             # Global agent task timeout
  stale_check_interval: 10m     # Stale scan frequency
  stale_threshold: 30m          # Stale detection threshold
  max_stale_dispatches: 3       # Max re-dispatch before CEO alert

notifications:
  webhook_url: ""               # Global Discord webhook
  agent_webhooks: {}            # Per-agent: {coder: "url", qa: "url"}
  openclaw_url: http://localhost:18789
  openclaw_key: ""

web:
  static_dir: ""                # Empty = embed.FS (production)
                                # Set to "dist" for dev mode with local files
```

All fields are optional. Unset fields fall back to environment variables, then defaults.

---

## macOS launchd Setup

Edit `launchd/com.irchelper.agent-queue.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.irchelper.agent-queue</string>

  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/agent-queue</string>
    <string>--db</string>
    <string>/Users/YOU/.ainative/queue.db</string>
  </array>

  <key>EnvironmentVariables</key>
  <dict>
    <key>AGENT_QUEUE_DISCORD_WEBHOOK_URL</key>
    <string>https://discord.com/api/webhooks/...</string>
    <key>AGENT_QUEUE_OPENCLAW_API_URL</key>
    <string>http://localhost:18789</string>
    <key>AGENT_QUEUE_OPENCLAW_API_KEY</key>
    <string>your-gateway-token</string>
  </dict>

  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/tmp/ainative.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/ainative-error.log</string>
</dict>
</plist>
```

Install and start:

```bash
make build
cp agent-queue /usr/local/bin/agent-queue
bash scripts/launchd-install.sh
curl http://localhost:19827/health
```

---

## Linux systemd Setup

Create `/etc/systemd/system/ainative.service`:

```ini
[Unit]
Description=ainative task queue
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/agent-queue --db /var/lib/ainative/queue.db
Restart=always
RestartSec=5
Environment="AGENT_QUEUE_DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/..."
Environment="AGENT_QUEUE_OPENCLAW_API_URL=http://localhost:18789"
Environment="AGENT_QUEUE_OPENCLAW_API_KEY=your-token"

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ainative
sudo systemctl status ainative
```

---

## Database

ainative uses SQLite in WAL mode. The database is created automatically on first run.

**Path recommendation:** Use an absolute path in production deployments to avoid working-directory confusion with launchd/systemd:

```bash
./agent-queue --db /var/lib/ainative/queue.db
# or
AGENT_QUEUE_DB_PATH=/var/lib/ainative/queue.db ./agent-queue
```

**Backup:** The database is a single file. Back it up with:
```bash
cp /var/lib/ainative/queue.db /backup/queue-$(date +%Y%m%d).db
# or use SQLite online backup:
sqlite3 /var/lib/ainative/queue.db ".backup /backup/queue.db"
```

**Schema migrations** run automatically on startup — no manual migration steps needed.

---

## Defaults Summary

| Setting | Default |
|---------|---------|
| Port | `19827` |
| Database | `data/queue.db` (relative to working dir) |
| Stale check interval | `10m` |
| Stale threshold | `30m` |
| Max stale dispatches | `3` |
| OpenClaw URL | `http://localhost:18789` |
| Static files | embed.FS (frontend built into binary) |
