# Agent Integration Guide

This guide explains how to integrate an AI agent (LLM-based or script-based) with ainative.

---

## Overview

An agent interacts with ainative through a simple HTTP lifecycle:

```
startup → poll → claim → in_progress → done/failed
```

All state lives in ainative's SQLite database. Agents are stateless — they can crash and restart without losing progress.

---

## The Agent Loop

### 1. Poll for work

On session startup, call:

```bash
curl -s "http://localhost:19827/tasks/poll?assigned_to=YOUR_AGENT_NAME"
```

Response when a task is available:
```json
{
  "task": {
    "id": "1897f8b09888cbf0",
    "title": "Implement login feature",
    "description": "Full task spec here...",
    "status": "pending",
    "version": 1,
    "priority": 0
  }
}
```

Response when no tasks are available:
```json
{"task": null}
```

> **Convention**: poll returns `null` → exit silently. Do not loop indefinitely.

### 2. Claim the task (atomic)

```bash
curl -s -X POST "http://localhost:19827/tasks/$TASK_ID/claim" \
  -H 'Content-Type: application/json' \
  -d "{\"version\": $VERSION, \"agent\": \"YOUR_AGENT_NAME\"}"
```

- Returns `409 Conflict` if another agent claimed it first — this is normal. Poll again.
- On success, `status` becomes `claimed` and `version` increments.

### 3. Mark in progress

```bash
curl -s -X PATCH "http://localhost:19827/tasks/$TASK_ID" \
  -H 'Content-Type: application/json' \
  -d "{\"status\": \"in_progress\", \"version\": $((VERSION+1))}"
```

### 4. Report completion

**Success:**
```bash
curl -s -X PATCH "http://localhost:19827/tasks/$TASK_ID" \
  -H 'Content-Type: application/json' \
  -d "{\"status\": \"done\", \"result\": \"One-line summary of what was done\", \"version\": $((VERSION+2))}"
```

**Failure / blocked:**
```bash
curl -s -X PATCH "http://localhost:19827/tasks/$TASK_ID" \
  -H 'Content-Type: application/json' \
  -d "{\"status\": \"failed\", \"result\": \"Reason for failure\", \"version\": $((VERSION+2))}"
```

> When a task fails, ainative's `retry_routing` table may automatically re-assign it to another agent. No manual intervention needed in most cases.

---

## Complete Shell Example

```bash
#!/bin/bash
AGENT="myagent"
BASE="http://localhost:19827"

# Poll
RESP=$(curl -s "$BASE/tasks/poll?assigned_to=$AGENT")
TASK_ID=$(echo "$RESP" | jq -r '.task.id // empty')

if [ -z "$TASK_ID" ]; then
  exit 0  # No work available, exit silently
fi

VER=$(echo "$RESP" | jq -r '.task.version')
TITLE=$(echo "$RESP" | jq -r '.task.title')
DESC=$(echo "$RESP" | jq -r '.task.description // ""')

# Claim
CLAIM=$(curl -s -X POST "$BASE/tasks/$TASK_ID/claim" \
  -H 'Content-Type: application/json' \
  -d "{\"version\":$VER,\"agent\":\"$AGENT\"}")

if echo "$CLAIM" | jq -e '.error' > /dev/null 2>&1; then
  exit 0  # Claimed by another agent, exit silently
fi

VER=$((VER+1))

# In progress
curl -s -X PATCH "$BASE/tasks/$TASK_ID" \
  -H 'Content-Type: application/json' \
  -d "{\"status\":\"in_progress\",\"version\":$VER}" > /dev/null

VER=$((VER+1))

# --- Do the actual work here ---
RESULT="Task completed successfully"

# Done
curl -s -X PATCH "$BASE/tasks/$TASK_ID" \
  -H 'Content-Type: application/json' \
  -d "{\"status\":\"done\",\"result\":\"$RESULT\",\"version\":$VER}"
```

---

## LLM Agent Patterns

### Reading task description

The `description` field contains the full task spec. Agents should read it at claim time:

```bash
curl -s "http://localhost:19827/tasks/$TASK_ID" | jq -r '.description'
```

### Including task_id in notifications

When a task spec includes `【ainative】task_id: <id>, version: <v>`, the agent should use that ID for all PATCH calls. This is the standard dispatch format from the CEO agent.

### Heartbeat / keepalive

For long-running tasks, periodically PATCH `updated_at` to signal the task is alive and prevent stale re-dispatch:

```bash
# Keep the task from being considered stale
curl -s -X PATCH "$BASE/tasks/$TASK_ID" \
  -H 'Content-Type: application/json' \
  -d "{\"status\":\"in_progress\",\"version\":$CURRENT_VER}"
```

> The default stale threshold is 30 minutes. Tasks idle beyond this are re-dispatched.

---

## OpenClaw Integration

If you're using [OpenClaw](https://openclaw.ai) as your agent runtime, ainative integrates via `SessionNotifier`:

### 1. Configure OpenClaw gateway

Add to `openclaw.json`:
```json
{
  "gateway": {
    "tools": {
      "allow": ["sessions_send"]
    }
  }
}
```

### 2. Set environment variables

```bash
AGENT_QUEUE_OPENCLAW_API_URL=http://localhost:18789
AGENT_QUEUE_OPENCLAW_API_KEY=your-gateway-token
```

### 3. How dispatch works

When you call `POST /dispatch`, ainative:
1. Creates the task in SQLite
2. Calls OpenClaw `sessions_send` to wake the target agent's session
3. The agent receives a notification and runs its poll loop

### 4. Agent session naming

ainative maps agent names to OpenClaw session keys. The default mapping covers common agent names (`coder`, `thinker`, `writer`, `qa`, `devops`, `security`, `ops`, `vision`, `pm`, `uiux`).

For custom agents, extend `internal/openclaw/client.go` or configure via `config.yaml`:
```yaml
agents:
  known:
    - name: myagent
      session_key: "agent:myagent:discord:channel:1234567890"
```

---

## Agent Naming Conventions

| Convention | Example | Description |
|------------|---------|-------------|
| `assigned_to` | `"coder"` | Lowercase, no spaces |
| `result` | `"Implemented login; tests pass"` | One line, factual |
| `failure_reason` | `"Missing API key for service X"` | Why it failed, not what to do |

---

## Error Reference

| HTTP Status | Meaning | Action |
|-------------|---------|--------|
| `409 Conflict` | Claim race — another agent got it | Poll again |
| `412 Precondition Failed` | Wrong `version` in PATCH | Re-fetch task, use latest version |
| `404 Not Found` | Task doesn't exist | Log and skip |
| `400 Bad Request` | Invalid payload | Check JSON structure |
| `503 Service Unavailable` | Server starting up | Retry after 5s |
