#!/bin/sh
# D-C3 独立判据：不用 stale 布尔，而用「下发路径的行为相变」
#   相变判据 = POST /api/agents/<半开机>/pull 的行为：
#      翻转前：requireOnline 通过 -> 请求挂起等 agent（curl -m 2 超时 0 bytes）
#      翻转后：requireOnline 拒绝 -> 毫秒级 503 agent-offline
#   同时记录 lastSeen，算出翻转时刻的 age，与 75s(PingTimeout)/90s(AgentStaleAfter) 对照
set -u
H='Authorization: Bearer qa-hub-token'
B=http://127.0.0.1:17760
DC="timeout 40 docker compose -f qa/env/docker-compose.yml exec -T"
DEAD=54d0c3373428-d82d
PID=$($DC box-c sh -c 'ps -eo pid,args | awk "/[h]omer agent/ && \$1>1000 {print \$1}"' | head -1)
echo "PID=$PID"
echo "=== STOP 前：确认在线且下发不挂 ==="
timeout 10 curl -s -o /dev/null -m 8 -w 'pre-pull http=%{http_code} t=%{time_total}\n' -X POST -H "$H" -H 'Content-Type: application/json' --data '{}' "$B/api/agents/$DEAD/pull?confirm=true"
echo -n "STOP at: "; date -u +%H:%M:%S
$DC box-c sh -c "kill -STOP $PID"; $DC box-c sh -c "awk '{print \$3}' /proc/$PID/stat"
T0=$(date +%s)
: > /tmp/stale.log
for i in $(seq 1 40); do
  NOW=$(date +%s); EL=$((NOW-T0))
  # 行为判据：短超时 pull。-m 2 -> 挂起=超时(28)，翻转=503 快速返回
  OUT=$(timeout 4 curl -s -o /tmp/st.out -m 2 -w '%{http_code}' -X POST -H "$H" -H 'Content-Type: application/json' --data '{}' "$B/api/agents/$DEAD/pull?confirm=true" 2>/dev/null)
  RC=$?
  ST=$(timeout 8 curl -s -H "$H" "$B/api/agents" | python3 -c "
import json,sys,datetime
for a in json.load(sys.stdin)['agents']:
    if a['agentId']=='$DEAD':
        now=datetime.datetime.now(datetime.timezone.utc)
        ls=datetime.datetime.fromisoformat(a['lastSeen'].replace('Z','+00:00'))
        print('%s age=%.1f'%(a['stale'],(now-ls).total_seconds()))
")
  if [ "$RC" = "28" ]; then BEHAV="HANG(等agent)"; else BEHAV="RET=$OUT"; fi
  echo "T+${EL}s  beh=${BEHAV}  stale/age=${ST}"
  printf 'T+%ss\t%s\t%s\n' "$EL" "$BEHAV" "$ST" >> /tmp/stale.log
  case "$BEHAV" in RET=503) echo ">>> 相变发生在 T+${EL}s"; break;; esac
  sleep 3
done
echo "=== CONT ==="; $DC box-c sh -c "kill -CONT $PID"; $DC box-c sh -c "awk '{print \$3}' /proc/$PID/stat"
