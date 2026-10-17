#!/bin/sh
# C-C1 独立判据：不用「没有应用这次同步。」这个字符串，而是用
#   (a) 磁盘上 center-only 文件 c.md 是否真实落盘（inode+mtime+md5）
#   (b) 单机 pull 的 payload.status 与 conflicts[] 条目数
#   (c) fanout 的 payload.status —— 两者比较「结构化真值」，不比文案
set -u
H='Authorization: Bearer qa-hub-token'
B=http://127.0.0.1:17760
DC="timeout 25 docker compose -f qa/env/docker-compose.yml exec -T"
AID=e363efdcf085-1563   # box-a 的在线 agentId
CENTER_C="BOX-C-FILE"

snap() {  # $1 = label
  echo "  [disk $1]"
  $DC box-a sh -c 'if [ -e /root/.pi/agent/agents/c.md ]; then md5sum /root/.pi/agent/agents/c.md; stat -c "c.md mtime=%Y size=%s" /root/.pi/agent/agents/c.md; else echo "c.md ABSENT"; fi; echo -n "SYSTEM.md="; cat /root/.pi/agent/SYSTEM.md'
}

reset() { # 把 box-a 复位到「有冲突 + 缺 center-only 文件」的确定性状态
  $DC box-a sh -c 'rm -f /root/.pi/agent/agents/c.md; printf "SYS-REPRO3-LOCAL-%s\n" "$(date +%s)" > /root/.pi/agent/SYSTEM.md; printf "KEEP\n" > /root/.pi/agent/agents/a.md'
}

echo "=== 前置：确认 center 有 c.md 且内容与本地不同 ==="
timeout 20 curl -s -H "$H" "$B/api/storage/file?adapter=pi&path=agents/c.md"
echo
reset; snap "after-reset"

echo
echo "=== 实验 1：单机 pull (confirm=true, adapters=[pi]) ==="
timeout 45 curl -s -w "\nHTTP=%{http_code}\n" -X POST -H "$H" -H 'Content-Type: application/json' \
  --data '{"adapters":["pi"],"confirm":true}' "$B/api/agents/$AID/pull?confirm=true"
snap "after-single-pull"

echo
echo "=== 复位 ==="
reset; snap "after-reset-2"

echo
echo "=== 实验 2：fanout dispatch ==="
timeout 60 curl -s -w "\nHTTP=%{http_code}\n" -X POST -H "$H" -H 'Content-Type: application/json' \
  --data '{"adapters":["pi"],"confirm":true}' "$B/api/sync?direction=dispatch&confirm=true"
snap "after-fanout"

echo
echo "=== 复位（收尾） ==="
reset
