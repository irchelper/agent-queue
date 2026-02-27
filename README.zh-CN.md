[English](./README.md)

# ainative

**让 AI agent 自己跑，不需要人盯着。** ainative 是一个轻量级任务队列，内置 AI-native 工作台 UI，让多个 AI agent 自主协调——不依赖任何中心调度者在线。

Agent 主动 poll 任务、原子认领、通过 HTTP 汇报完成。串行链全程自动推进：任务 A 完成后，任务 B 自动解锁，下一个 agent 在下次 poll 时认领执行，无需人工传递接力棒。

单二进制。零外部依赖。本机直接运行。

---

## Quick Start

```bash
# 1. 克隆并构建
git clone https://github.com/irchelper/ainative.git
cd ainative
make build          # 编译 Go 二进制 + 嵌入前端

# 2. 启动服务
./agent-queue       # 监听 :19827

# 3. 打开工作台
open http://localhost:19827/

# 4. 创建第一个任务
curl -s -X POST localhost:19827/tasks \
  -H 'Content-Type: application/json' \
  -d '{"title":"我的第一个任务","assigned_to":"coder"}'
```

需要 Go 1.22+。无需配置数据库——SQLite 已内嵌。

---

## 为什么需要 ainative？

没有持久化任务队列的 multi-agent 系统会以可预期的方式崩溃：

- **调度者单点故障**："CEO" agent 必须一直在线才能推进下一步。它一睡着，整条链就卡住。
- **状态不持久**：任务状态存在 LLM 上下文里。上下文压缩或开新 session = 进度丢失。
- **沉默失败**：Agent 完成了工作，但没人收到通知。用户只能来问"做完了吗？"

ainative 把任务状态从 agent 记忆搬到 SQLite。任何 agent 崩溃后都能恢复。串行链自动推进。任务完成直接通知你。

---

## 功能特性

- **任务队列** — 完整 CRUD + 乐观锁（`version` 字段）；并发认领 → 409 Conflict
- **依赖关系图** — `depends_on` 数组；前置完成 → 后续自动解锁
- **8 态状态机** — `pending → claimed → in_progress → review → done / blocked / failed / cancelled`
- **原子派发** — `POST /dispatch` 一步建任务 + 唤醒 agent session
- **串行链派发** — `POST /dispatch/chain` 一次性创建完整串行链，自动设置 `depends_on`
- **CEO 通知** — 任务/链路完成后通知 CEO session（RetryQueue：30s/60s/120s 退避）
- **自动重试路由** — 任务失败后按 `retry_routing` 表自动改派，无需人工介入
- **Stale 任务恢复** — 超时未认领的任务自动重派
- **Web UI** — 内置 SPA 工作台（Vue 3 + TypeScript + Tailwind）；embed.FS，无需独立服务
- **Discord webhook** — 支持全局或按 agent 分发 `done`/`failed` 通知
- **健康检查** — `GET /health`，供 launchd/systemd 监控

---

## Web UI

访问 `http://localhost:19827/` 打开工作台：

- **Dashboard** — 待办任务 + 异常汇总（blocked/failed/超时）
- **目标追踪** — 串行链进度（链路视图 + 每步 result）
- **看板** — 全局任务状态（7列，审计视图）
- **任务详情** — 时间线（状态变更历史 + 👤/🤖 标识）
- **目标输入** — 自然语言输入 + 模板拆解 + 一键创建链路

---

## API 参考

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/health` | 服务与数据库健康检查 |
| `POST` | `/tasks` | 创建任务（支持 `depends_on`、`notify_ceo_on_complete` 等） |
| `GET` | `/tasks` | 列出任务（按 `status`、`assigned_to`、`deps_met` 过滤）|
| `GET` | `/tasks/:id` | 任务详情（含依赖链 + 状态历史）|
| `PATCH` | `/tasks/:id` | 更新状态/结果（需携带 `version`）|
| `POST` | `/tasks/:id/claim` | 原子认领：`{"version": N, "agent": "name"}` |
| `GET` | `/tasks/poll` | 获取最优任务（`?assigned_to=X`）；无任务返回 `null` |
| `GET` | `/tasks/summary` | 全局计数 + 活跃任务列表 |
| `POST` | `/dispatch` | 建任务 + 触发 agent session；支持 `notify_ceo_on_complete` |
| `POST` | `/dispatch/chain` | 创建完整串行链，自动设置 `depends_on` |

完整 API spec：[`docs/api/openapi.yaml`](./docs/api/openapi.yaml)

### Agent poll 示例（shell）

```bash
RESP=$(curl -s "localhost:19827/tasks/poll?assigned_to=myagent")
TASK_ID=$(echo $RESP | jq -r '.task.id // empty')

if [ -n "$TASK_ID" ]; then
  VER=$(echo $RESP | jq -r '.task.version')

  # 认领
  curl -s -X POST "localhost:19827/tasks/$TASK_ID/claim" \
    -H 'Content-Type: application/json' \
    -d "{\"version\":$VER,\"agent\":\"myagent\"}"

  # 执行中
  curl -s -X PATCH "localhost:19827/tasks/$TASK_ID" \
    -H 'Content-Type: application/json' \
    -d "{\"status\":\"in_progress\",\"version\":$((VER+1))}"

  # 完成
  curl -s -X PATCH "localhost:19827/tasks/$TASK_ID" \
    -H 'Content-Type: application/json' \
    -d "{\"status\":\"done\",\"result\":\"任务完成摘要\",\"version\":$((VER+2))}"
fi
```

详见 [Agent 接入指南](./docs/guides/agent-integration.md)。

---

## 配置

零配置开箱即用。所有配置项均为可选：

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `AGENT_QUEUE_DISCORD_WEBHOOK_URL` | — | Discord Incoming Webhook，接收任务通知 |
| `AGENT_QUEUE_AGENT_WEBHOOKS` | — | 按 agent 分发：`agent1=url1,agent2=url2` |
| `AGENT_QUEUE_OPENCLAW_API_URL` | `http://localhost:18789` | OpenClaw gateway URL |
| `AGENT_QUEUE_OPENCLAW_API_KEY` | — | OpenClaw gateway token |
| `AGENT_QUEUE_DB_PATH` | `data/queue.db` | SQLite 数据库路径 |
| `AGENT_QUEUE_STALE_CHECK_INTERVAL` | `10m` | Stale 任务扫描间隔 |
| `AGENT_QUEUE_STALE_THRESHOLD` | `30m` | 超时判定阈值 |
| `AGENT_QUEUE_MAX_STALE_DISPATCHES` | `3` | 最大重派次数 |

### 配置文件（可选）

在工作目录创建 `config.yaml`：

```yaml
server:
  port: 19827
  db: data/queue.db
notifications:
  webhook_url: "https://discord.com/api/webhooks/..."
  openclaw_url: "http://localhost:18789"
timeouts:
  stale_check_interval: 10m
  stale_threshold: 30m
```

完整字段说明：[配置参考](./docs/guides/configuration.md)

---

## 部署

### macOS（launchd）

```bash
make build
# 编辑 launchd/com.irchelper.agent-queue.plist，填入环境变量
bash scripts/launchd-install.sh
curl http://localhost:19827/health   # 验证
```

### Linux（systemd）

```bash
make build
# 复制 agent-queue 到 /usr/local/bin/
# 创建 /etc/systemd/system/ainative.service（参考配置参考文档）
systemctl enable --now ainative
```

---

## 开发

```bash
make test      # Go 测试（含 race detector）
make vet       # go vet
make build     # 编译（含前端 embed）
make build-web # 仅构建前端
make clean     # 删除 binary（不删数据库）
make clean-all # 删除 binary + 数据库（危险操作）
```

前端热更新开发：
```bash
make dev-api          # 启动 Go API server
cd web && npm run dev # 启动 Vite dev server（代理 API 到 :19827）
```

---

## 架构说明

- **存储**：SQLite WAL 模式，单文件，ACID 事务；路径通过 `AGENT_QUEUE_DB_PATH` 配置
- **后端**：Go `net/http`，无框架；`version` 字段乐观锁
- **前端**：Vue 3 + TypeScript + Vite + Tailwind；`embed.FS` 嵌入（单二进制）
- **通知**：Discord Incoming Webhook（用户审计）+ SessionNotifier（agent 唤醒 / CEO 告警）
- **部署**：单二进制；launchd（macOS）/ systemd（Linux）；KeepAlive 自动重启

完整架构：[`docs/ARCH.md`](./docs/ARCH.md) | 产品规格：[`docs/PRD.md`](./docs/PRD.md)

---

## 参与贡献

参见 [CONTRIBUTING.md](./CONTRIBUTING.md)（开发环境配置、编码规范、PR 流程）。

## License

MIT
