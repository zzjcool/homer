#!/bin/sh
# C-C1 复核：用 inode 变化证明 fanout 真的新建/覆写了文件（排除 mtime 疑点）
set -u
H='Authorization: Bearer qa-hub-token'; B=http://127.0.0.1:17760
DC="timeout 25 docker compose -f qa/env/docker-compose.yml exec -T"
AID=e363efdcf085-1563
echo "=== 中心内容 ==="; timeout 20 curl -s -H "$H" "$B/api/storage/file?adapter=pi&path=agents/c.md"; echo
echo "=== 复位 box-a：删 c.md + 制造 SYSTEM.md 冲突 ==="
$DC box-a sh -c 'rm -f /root/.pi/agent/agents/c.md; printf "SYS-REPRO3-LOCAL-%s\n" "$(date +%s)" > /root/.pi/agent/SYSTEM.md'
echo -n "BEFORE: "; $DC box-a sh -c 'if [ -e /root/.pi/agent/agents/c.md ]; then stat -c "inode=%i mtime=%Y %n" /root/.pi/agent/agents/c.md; else echo "c.md ABSENT"; fi'
echo "=== fanout ==="
timeout 60 curl -s -w "\nHTTP=%{http_code}\n" -X POST -H "$H" -H 'Content-Type: application/json' \
  --data '{"adapters":["pi"],"confirm":true}' "$B/api/sync?direction=dispatch&confirm=true" | tee /tmp/cc1b.json
echo -n "AFTER : "; $DC box-a sh -c 'if [ -e /root/.pi/agent/agents/c.md ]; then stat -c "inode=%i mtime=%Y %n" /root/.pi/agent/agents/c.md; cat /root/.pi/agent/agents/c.md; else echo "c.md ABSENT"; fi'
echo -n "SYSTEM.md 是否仍为本地内容: "; $DC box-a sh -c 'cat /root/.pi/agent/SYSTEM.md'
echo
echo "=== 同一时刻单机 pull 的结构化输出（对照）==="
$DC box-a sh -c 'rm -f /root/.pi/agent/agents/c.md; printf "SYS-REPRO3-LOCAL-%s\n" "$(date +%s)" > /root/.pi/agent/SYSTEM.md'
timeout 45 curl -s -w "\nHTTP=%{http_code}\n" -X POST -H "$H" -H 'Content-Type: application/json' \
  --data '{"adapters":["pi"],"confirm":true}' "$B/api/agents/$AID/pull?confirm=true" | tee /tmp/cc1b-single.json
echo -n "AFTER : "; $DC box-a sh -c 'if [ -e /root/.pi/agent/agents/c.md ]; then stat -c "inode=%i mtime=%Y" /root/.pi/agent/agents/c.md; else echo "c.md ABSENT"; fi'
echo "=== 字段级对照 ==="
python3 - <<'PY'
import json
f=json.loads(open('/tmp/cc1b.json').read().rsplit('\nHTTP=',1)[0])
s=json.loads(open('/tmp/cc1b-single.json').read().rsplit('\nHTTP=',1)[0])
fe=[a for a in f['agents'] if a['agentId']=='e363efdcf085-1563'][0]
print("fanout 条目 keys :", sorted(fe.keys()))
print("fanout 条目      :", json.dumps(fe,ensure_ascii=False))
print("fanout 顶层 errors:", f['errors'])
print("single keys      :", sorted(s.keys()))
print("single applied   :", json.dumps(s.get('applied'),ensure_ascii=False))
print("single conflicts :", json.dumps(s.get('conflicts'),ensure_ascii=False))
print("single errors    :", json.dumps(s.get('errors'),ensure_ascii=False))
PY
