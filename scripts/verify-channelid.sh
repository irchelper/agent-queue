#!/usr/bin/env bash
# verify-channelid.sh — 验证 agent-queue Discord channel/webhook 配置
#
# 用途：
#   1. 列出所有 agent 的 Discord session key（channel ID）
#   2. 测试 AGENT_QUEUE_DISCORD_WEBHOOK_URL（默认 webhook）是否可达
#   3. 如果设置了 AGENT_QUEUE_AGENT_WEBHOOKS，验证各 agent webhook URL
#   4. 检查 launchd plist 当前加载的 env 配置
#   5. 打印 AGENT_QUEUE_AGENT_WEBHOOKS 推荐配置格式（供 devops 填写 webhook URL）
#
# 用法：
#   ./scripts/verify-channelid.sh
#   AGENT_QUEUE_AGENT_WEBHOOKS="coder=https://..." ./scripts/verify-channelid.sh
#
# 依赖：curl, python3（macOS 内置）
# 兼容：bash 3.x (macOS default) / bash 5.x

set -euo pipefail

PASS=0
FAIL=0

ok()   { PASS=$((PASS+1)); echo "  ✅ $1"; }
fail() { FAIL=$((FAIL+1)); echo "  ❌ $1"; }
info() { echo "  ℹ️  $1"; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PLIST="$HOME/Library/LaunchAgents/com.irchelper.agent-queue.plist"
PLIST_SRC="$REPO_ROOT/launchd/com.irchelper.agent-queue.plist"

echo "═══════════════════════════════════════════════════════"
echo "  agent-queue Discord Channel/Webhook 验证"
echo "═══════════════════════════════════════════════════════"
echo ""

# ─────────────────────────────────────────────────────────
# 1. Agent session key 列表（channel ID）
# ─────────────────────────────────────────────────────────
echo "── 1. Agent Session Keys（Discord channel IDs）──"

# Format: "agent:session_key"
AGENT_ENTRIES=(
  "ceo:agent:ceo:discord:channel:1475338424293789877"
  "coder:agent:coder:discord:channel:1475338640593916045"
  "thinker:agent:thinker:discord:channel:1475338689646297305"
  "writer:agent:writer:discord:channel:1475339585075548200"
  "devops:agent:devops:discord:channel:1475339626872049736"
  "qa:agent:qa:discord:channel:1475679634442944532"
  "security:agent:security:discord:channel:1475339809206697984"
  "ops:agent:ops:discord:channel:1475339864361664684"
  "pm:agent:pm:discord:channel:1476150796071600242"
  "uiux:agent:uiux:discord:channel:1476150914216886395"
  "vision:agent:vision:discord:channel:1475680076380110969"
)

for entry in "${AGENT_ENTRIES[@]}"; do
  agent="${entry%%:*}"
  session_key="${entry#*:}"
  channel_id="${session_key##*:}"
  printf "  %-10s  channel_id=%-22s  session_key=%s\n" "$agent" "$channel_id" "$session_key"
done
echo ""

# ─────────────────────────────────────────────────────────
# 2. launchd plist 配置检查
# ─────────────────────────────────────────────────────────
echo "── 2. launchd plist 配置 ──"

PLIST_FILE=""
if [[ -f "$PLIST" ]]; then
  PLIST_FILE="$PLIST"
  info "使用已安装 plist: $PLIST"
elif [[ -f "$PLIST_SRC" ]]; then
  PLIST_FILE="$PLIST_SRC"
  info "使用源 plist: $PLIST_SRC"
else
  fail "未找到 plist 文件"
fi

PLIST_WEBHOOK=""
PLIST_USER_ID=""
PLIST_PORT="19827"
PLIST_AGENT_WEBHOOKS=""

if [[ -n "$PLIST_FILE" ]]; then
  get_plist_env() {
    python3 - "$1" <<'PYEOF'
import sys, plistlib
key = sys.argv[1]
plist_file = sys.argv[2] if len(sys.argv) > 2 else ""
PYEOF
    python3 -c "
import sys, plistlib
with open('$PLIST_FILE', 'rb') as f:
    pl = plistlib.load(f)
env = pl.get('EnvironmentVariables', {})
print(env.get(sys.argv[1], ''))
" "$1" 2>/dev/null || echo ""
  }

  PLIST_WEBHOOK=$(get_plist_env "AGENT_QUEUE_DISCORD_WEBHOOK_URL")
  PLIST_USER_ID=$(get_plist_env "AGENT_QUEUE_DISCORD_USER_ID")
  PLIST_PORT=$(get_plist_env "AGENT_QUEUE_PORT")
  PLIST_AGENT_WEBHOOKS=$(get_plist_env "AGENT_QUEUE_AGENT_WEBHOOKS")

  if [[ -n "$PLIST_WEBHOOK" ]]; then
    ok "AGENT_QUEUE_DISCORD_WEBHOOK_URL 已配置"
    info "  URL: ${PLIST_WEBHOOK:0:70}..."
  else
    fail "AGENT_QUEUE_DISCORD_WEBHOOK_URL 未在 plist 中配置"
  fi

  if [[ -n "$PLIST_USER_ID" ]]; then
    ok "AGENT_QUEUE_DISCORD_USER_ID=${PLIST_USER_ID}"
  else
    fail "AGENT_QUEUE_DISCORD_USER_ID 未配置"
  fi

  info "AGENT_QUEUE_PORT=${PLIST_PORT:-未设置（默认19827）}"

  if [[ -n "$PLIST_AGENT_WEBHOOKS" ]]; then
    ok "AGENT_QUEUE_AGENT_WEBHOOKS 已配置（per-agent webhook 路由已启用）"
    info "  值: ${PLIST_AGENT_WEBHOOKS:0:80}..."
  else
    info "AGENT_QUEUE_AGENT_WEBHOOKS 未配置（所有通知走默认 #日报 webhook）"
  fi
fi
echo ""

# ─────────────────────────────────────────────────────────
# 3. 测试 webhook URL 可达性
# ─────────────────────────────────────────────────────────
echo "── 3. Webhook 可达性测试 ──"

test_webhook() {
  local label="$1"
  local url="$2"

  if [[ -z "$url" ]]; then
    fail "$label: URL 为空，跳过"
    return
  fi

  local http_code
  http_code=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$url" \
    -H "Content-Type: application/json" \
    -d "{\"content\":\"[verify-channelid] $label 连通性测试 $(date '+%H:%M:%S')\"}" \
    --max-time 10 2>/dev/null || echo "000")

  if [[ "$http_code" == "204" || "$http_code" == "200" ]]; then
    ok "$label: HTTP $http_code ✓"
  elif [[ "$http_code" == "000" ]]; then
    fail "$label: 连接失败（curl error / timeout）"
  elif [[ "$http_code" == "429" ]]; then
    info "$label: HTTP 429 限速（URL 有效但触发 rate limit）"
    PASS=$((PASS+1))
  else
    fail "$label: HTTP $http_code"
  fi
}

# 使用运行时 env 优先，fallback 到 plist
EFFECTIVE_WEBHOOK="${AGENT_QUEUE_DISCORD_WEBHOOK_URL:-${PLIST_WEBHOOK:-}}"
EFFECTIVE_AGENT_WEBHOOKS="${AGENT_QUEUE_AGENT_WEBHOOKS:-${PLIST_AGENT_WEBHOOKS:-}}"

# 默认 webhook 测试
if [[ -n "$EFFECTIVE_WEBHOOK" ]]; then
  test_webhook "默认 webhook (#日报)" "$EFFECTIVE_WEBHOOK"
else
  fail "默认 webhook URL 未找到（plist 与运行时 env 均未设置）"
fi

# per-agent webhook 测试
if [[ -n "$EFFECTIVE_AGENT_WEBHOOKS" ]]; then
  echo ""
  echo "  per-agent webhook 测试："
  IFS=',' read -ra entries <<< "$EFFECTIVE_AGENT_WEBHOOKS"
  for entry in "${entries[@]}"; do
    entry="${entry// /}"  # trim spaces
    agent="${entry%%=*}"
    url="${entry#*=}"
    if [[ -n "$agent" && -n "$url" && "$agent" != "$url" ]]; then
      test_webhook "  $agent" "$url"
    fi
  done
else
  info "AGENT_QUEUE_AGENT_WEBHOOKS 未设置，跳过 per-agent 测试"
fi
echo ""

# ─────────────────────────────────────────────────────────
# 4. AGENT_QUEUE_AGENT_WEBHOOKS 配置模板
# ─────────────────────────────────────────────────────────
echo "── 4. AGENT_QUEUE_AGENT_WEBHOOKS 配置模板（供 devops 填写）──"
echo ""
echo "  在 launchd plist EnvironmentVariables 中添加："
echo ""
echo "  <key>AGENT_QUEUE_AGENT_WEBHOOKS</key>"
printf "  <string>"
AGENTS=(coder thinker writer devops qa security ops pm uiux vision)
for i in "${!AGENTS[@]}"; do
  agent="${AGENTS[$i]}"
  if [[ $i -gt 0 ]]; then printf ","; fi
  printf "%s=WEBHOOK_URL_%s" "$agent" "$(echo "$agent" | tr '[:lower:]' '[:upper:]')"
done
printf "</string>\n"
echo ""
echo "  说明："
for agent in "${AGENTS[@]}"; do
  channel_id=""
  for entry in "${AGENT_ENTRIES[@]}"; do
    a="${entry%%:*}"
    if [[ "$a" == "$agent" ]]; then
      sk="${entry#*:}"
      channel_id="${sk##*:}"
      break
    fi
  done
  printf "    %-10s  channel_id=%-22s  → 该频道的 Incoming Webhook URL\n" "$agent" "$channel_id"
done
echo ""

# ─────────────────────────────────────────────────────────
# 5. 健康检查：server 是否运行
# ─────────────────────────────────────────────────────────
echo "── 5. Agent-queue server 健康检查 ──"
PORT="${AGENT_QUEUE_PORT:-${PLIST_PORT:-19827}}"
HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:${PORT}/health" --max-time 3 2>/dev/null || echo "000")
if [[ "$HEALTH" == "200" ]]; then
  ok "server 运行中（http://localhost:${PORT}/health → HTTP 200）"
else
  fail "server 未响应（http://localhost:${PORT}/health → HTTP $HEALTH）"
fi
echo ""

# ─────────────────────────────────────────────────────────
# Summary
# ─────────────────────────────────────────────────────────
echo "═══════════════════════════════════════════════════════"
echo "  验证结果: ✅ $PASS 通过 / ❌ $FAIL 失败"
echo "═══════════════════════════════════════════════════════"

exit $FAIL
