#!/usr/bin/env bash
# homer hub 容器化 E2E 冒烟。
# 用法: bash e2e/hub-smoke.sh   （需要 docker compose 与 jq）
#
# 步骤 1–7 是 plan §5 的快乐路径（健康检查、双 agent、资源快照、远程
# status、漂移 push/pull）。其后追加三条仍走同一套 compose 拓扑的断言：
#   8. 鉴权负例：无 token / 错 token → 401，拓扑保持运行
#   9. 密钥环：agent-a 加密，hub 收取后下发，agent-b 用口令解开明文
#  10. 按机器停用：agent-b 停用 pi 后，显式 pull 返回 400，扇出里它被跳过
#
# 断言失败时 EXIT trap 先打印 compose ps 与近期日志，再 down -v。
# 负例（401 / 400）是断言本身，不会拆拓扑。
#
# 未放进本脚本（issue #35，另案或成本不适合这条冒烟）：
# - 应用升级：镜像只有 homer，没有 pi / herdr / opencode 本体
# - 离线冲突：要中途停容器并制造三方冲突，不稳定且更慢
# - 控制台 chromedp、真实公网 DERP、双机 homer pair、CI 接线（#34）
set -euo pipefail

cd "$(dirname "$0")"
COMPOSE="docker compose -f docker-compose.yml"

HUB_URL=http://localhost:17760
AUTH="Authorization: Bearer test-token"
# 口令至少 8 位。明文标记故意不像密钥模式，避免被 secret scanner 拦住。
KEY_PASSWORD="hub-smoke-password"
KEY_MARKER="KEYRING-HUB-SMOKE-PLAINTEXT-91c7"
# 两个 agent 的进程 HOME 都是 /root/toolhome（见 docker-compose.yml），
# HOMER_HOME 则是 /root/.homer。密钥环根 ~/.homer/keyring 按进程 HOME 展开，
# 所以加密必须在 HOME=/root/toolhome 下执行；明文用绝对路径，解开时不依赖 ~。
KEY_PLAINTEXT="/root/toolhome/hub-smoke-secret.txt"
KEYRING_TREE="/root/toolhome/.homer/keyring"
LAST_BODY=""

need() { command -v "$1" >/dev/null || { echo "缺少依赖: $1"; exit 2; }; }
need curl
need jq

fail() {
  echo "    FAIL: $*" >&2
  exit 1
}

# expect_status 把响应体留在 LAST_BODY。不用 curl -f，这样 4xx 仍能断言。
expect_status() {
  local desc="$1" want="$2"
  shift 2
  local body code
  body=$(mktemp)
  code=$(curl -sS -o "$body" -w '%{http_code}' "$@" || true)
  LAST_BODY=$(cat "$body" 2>/dev/null || true)
  rm -f "$body"
  if [ "$code" != "$want" ]; then
    echo "    FAIL: $desc（HTTP $code，期望 $want）" >&2
    printf '%s\n' "$LAST_BODY" >&2
    exit 1
  fi
  echo "    OK: $desc（HTTP $code）"
}

assert_jq() {
  local desc="$1" filter="$2"
  if ! printf '%s' "$LAST_BODY" | jq -e "$filter" >/dev/null; then
    echo "    FAIL: $desc" >&2
    printf '%s\n' "$LAST_BODY" >&2
    exit 1
  fi
  echo "    OK: $desc"
}

# assert_absent 确认同步树里没有明文、口令或 age 数据密钥。
# 路径不存在也算失败：否则「没扫到」会把漏同步伪装成通过。
assert_absent() {
  local svc="$1" path="$2" result
  result=$($COMPOSE exec -T "$svc" sh -c '
    path="$1"; marker="$2"; password="$3"
    if ! test -e "$path"; then echo MISSING; exit 0; fi
    hits=$(grep -r -n -F -e "$marker" -e "$password" -e "AGE-SECRET-KEY-" "$path" || true)
    if [ -n "$hits" ]; then
      printf "%s\n" "$hits"
      echo LEAK
      exit 0
    fi
    echo CLEAN
  ' sh "$path" "$KEY_MARKER" "$KEY_PASSWORD")
  case "$result" in
    CLEAN) echo "    OK: $svc:$path 无明文、口令或数据密钥" ;;
    MISSING) fail "$svc 上没有 $path，无法确认密钥材料没有进同步树" ;;
    *) printf '%s\n' "$result" >&2; fail "$svc:$path 出现了明文、口令或 AGE-SECRET-KEY" ;;
  esac
}

teardown() {
  local code=$?
  if [ "$code" -ne 0 ]; then
    echo "==> 失败 (exit $code)，compose 状态与日志：" >&2
    $COMPOSE ps >&2 || true
    $COMPOSE logs --tail=200 >&2 || true
  fi
  $COMPOSE down -v >/dev/null 2>&1 || true
}
trap teardown EXIT

echo "==> 构建并启动 compose 拓扑（origin + hub + agent-a + agent-b）"
$COMPOSE up -d --build >/dev/null

wait_for() {
  local desc="$1" url="$2" extra="${3:-}"
  for _ in $(seq 1 30); do
    if curl -fsS $extra "$url" >/dev/null 2>&1; then
      echo "    OK: $desc"
      return 0
    fi
    sleep 2
  done
  echo "    FAIL: $desc ($url)"
  $COMPOSE ps
  return 1
}

echo "==> 1. hub 健康检查"
wait_for "GET /api/health" "$HUB_URL/api/health"

echo "==> 2. 双 agent 在线"
for _ in $(seq 1 30); do
  if curl -fsS -H "$AUTH" "$HUB_URL/api/agents" 2>/dev/null \
      | jq -e '[.agents[] | select(.stale == false) | .agentId]
          | contains(["agent-a","agent-b"])' >/dev/null; then
    echo "    OK: agent-a 与 agent-b 均在线"
    break
  fi
  sleep 2
  if [ "$_" = "30" ]; then
    echo "    FAIL: agents 未注册"
    curl -fsS -H "$AUTH" "$HUB_URL/api/agents" 2>/dev/null | jq . || true
    exit 1
  fi
done

echo "==> 2b. 机器资源快照（agent 自己采集的 CPU / 内存 / 网卡）"
for id in agent-a agent-b; do
  ok=0
  for _ in $(seq 1 15); do
    if curl -fsS -H "$AUTH" "$HUB_URL/api/agents" 2>/dev/null \
        | jq -e --arg id "$id" '
            .agents[] | select(.agentId == $id) | .host
            | (.memory.total > 0)
              and (.cpu.cores > 0)
              and (.cpu.usage != null)
              and ([.nets[]? | select((.addrs // []) | length > 0)] | length >= 1)
          ' >/dev/null; then
      echo "    OK: $id"
      curl -fsS -H "$AUTH" "$HUB_URL/api/agents" \
        | jq -c --arg id "$id" '.agents[] | select(.agentId == $id) | {agentId, host: {os: .host.os, distro: .host.distro, cpu: .host.cpu, memory: .host.memory, nets: .host.nets}}'
      ok=1
      break
    fi
    sleep 2
  done
  if [ "$ok" != 1 ]; then
    echo "    FAIL: $id 没有上报可用的资源快照"
    curl -fsS -H "$AUTH" "$HUB_URL/api/agents" | jq . || true
    exit 1
  fi
done

echo "==> 3. agent-a 远程 status 可取"
curl -fsS -H "$AUTH" -X POST "$HUB_URL/api/agents/agent-a/status" \
  | jq -e '.report.adapters | length >= 1' >/dev/null
echo "    OK: agent-a status"

echo "==> 4. agent-b 远程 status 可取"
curl -fsS -H "$AUTH" -X POST "$HUB_URL/api/agents/agent-b/status" \
  | jq -e '.report.adapters | length >= 1' >/dev/null
echo "    OK: agent-b status"

echo "==> 5. 制造漂移 → agent-a 远程 push"
$COMPOSE exec -T agent-a sh -c \
  'printf "\n// hub-e2e\n" >> /root/toolhome/.pi/agent/settings.json'
curl -fsS -H "$AUTH" -X POST \
  "$HUB_URL/api/agents/agent-a/push?confirm=true" \
  | jq -e '.ok == true' >/dev/null
echo "    OK: agent-a pushed"

echo "==> 6. agent-b 远程 pull → 配置真实流转"
curl -fsS -H "$AUTH" -X POST \
  "$HUB_URL/api/agents/agent-b/pull?confirm=true" \
  | jq -e '.ok == true' >/dev/null
echo "    OK: agent-b pulled"

echo "==> 7. 终态断言：agent-b 工具目录出现同一行"
$COMPOSE exec -T agent-b sh -c \
  'grep -q "hub-e2e" /root/toolhome/.pi/agent/settings.json'
echo "    OK: 配置跨容器流转成功"

echo "==> 8. 鉴权负例：无 token / 错 token → 401（不拆拓扑）"
expect_status "GET /api/health 仍无需 token" 200 "$HUB_URL/api/health"
expect_status "GET /api/agents 无 token" 401 "$HUB_URL/api/agents"
assert_jq "无 token 的错误码是 unauthorized" '.error.code == "unauthorized"'
expect_status "GET /api/agents 错误 token" 401 \
  -H "Authorization: Bearer wrong-token" \
  "$HUB_URL/api/agents"
assert_jq "错误 token 的错误码是 unauthorized" '.error.code == "unauthorized"'

echo "==> 9. 密钥环：agent-a 加密，收取后下发，agent-b 解开"
if ! $COMPOSE exec -T agent-a sh -c '
  set -eu
  export HOME=/root/toolhome
  homer key create --id hubsmoke --name HubSmoke --password "$1" --home /root/.homer --json
  printf "%s\n" "$2" > /root/toolhome/hub-smoke-secret.txt
  homer key encrypt --id hubsmoke --file smoke \
    --path /root/toolhome/hub-smoke-secret.txt \
    --password "$1" --home /root/.homer --json
' sh "$KEY_PASSWORD" "$KEY_MARKER"; then
  fail "agent-a 创建或加密密钥失败"
fi
echo "    OK: agent-a 已加密（未绑定适配器，扇出 pi 不会被密钥守卫拦住）"
if $COMPOSE exec -T agent-b sh -c "test -f '$KEY_PLAINTEXT'"; then
  fail "agent-b 在解开前已经有明文"
fi
echo "    OK: agent-b 解开前没有明文"
expect_status "收取 agent-a 的 keyring" 200 \
  -H "$AUTH" -H "Content-Type: application/json" \
  -X POST "$HUB_URL/api/sync?direction=collect&agent=agent-a&confirm=true" \
  --data '{"adapters":["keyring"]}'
assert_jq "keyring 已写入中心" '.ok == true and .status == "synced"'
expect_status "下发 keyring 并在 agent-b 解开" 200 \
  -H "$AUTH" -H "Content-Type: application/json" \
  -X POST "$HUB_URL/api/agents/agent-b/pull?confirm=true" \
  --data "{\"adapters\":[\"keyring\"],\"unlocks\":[{\"id\":\"hubsmoke\",\"password\":\"$KEY_PASSWORD\"}]}"
assert_jq "agent-b pull ok" '.ok == true'
if ! $COMPOSE exec -T agent-b sh -c "grep -qx '$KEY_MARKER' '$KEY_PLAINTEXT'"; then
  fail "agent-b 没有解开出预期明文"
fi
echo "    OK: agent-b 明文与 agent-a 一致"
assert_absent hub /root/.homer/generations
assert_absent agent-a "$KEYRING_TREE"
assert_absent agent-b "$KEYRING_TREE"

echo "==> 10. 按机器停用 pi：显式 pull 拒绝，扇出跳过 agent-b"
expect_status "停用 agent-b 的 pi" 200 \
  -H "$AUTH" -H "Content-Type: application/json" \
  -X POST "$HUB_URL/api/agents/agent-b/adapters/pi" \
  --data '{"disabled":true}'
assert_jq "停用返回 ok" '.ok == true'
expect_status "读回 agent-b 适配器覆盖" 200 \
  -H "$AUTH" \
  "$HUB_URL/api/agents/agent-b/adapters"
assert_jq "pi 已标记停用" \
  '[.adapters[] | select(.id == "pi" and .disabled == true)] | length == 1'
expect_status "显式下发已停用的 pi" 400 \
  -H "$AUTH" -H "Content-Type: application/json" \
  -X POST "$HUB_URL/api/agents/agent-b/pull?confirm=true" \
  --data '{"adapters":["pi"]}'
assert_jq "错误码是 adapter-disabled-for-agent" \
  '.error.code == "adapter-disabled-for-agent"'
expect_status "扇出 pi" 200 \
  -H "$AUTH" -H "Content-Type: application/json" \
  -X POST "$HUB_URL/api/sync?direction=dispatch&confirm=true" \
  --data '{"adapters":["pi"]}'
assert_jq "agent-b 被跳过且 agent-a 仍同步" \
  '.ok == true and .direction == "dispatch"
   and ([.agents[] | select(.agentId == "agent-b" and .skipped == true and .reason == "adapters disabled for this agent")] | length == 1)
   and ([.agents[] | select(.agentId == "agent-a" and .ok == true and .skipped == false)] | length == 1)'

echo ""
echo "✅ 容器化 E2E 全部通过"
