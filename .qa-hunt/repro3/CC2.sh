#!/bin/sh
# C-C2 独立判据：不比 toast 文案，比「结构化真值」
#   J1 = HTTP 状态码 + 顶层 ok/status
#   J2 = 每台机器 skipped 计数 == 总机器数 且 status=="synced"
#   J3 = 磁盘真值：center 独有文件 c.md 是否落到 box-a（下发是否真的发生）
#   J4 = 前端 toast 的真实字符串（用真实响应跑 index.html:3776 的表达式）
set -u
H='Authorization: Bearer qa-hub-token'
B=http://127.0.0.1:17760
DC="timeout 25 docker compose -f qa/env/docker-compose.yml exec -T"

IDS=$(timeout 20 curl -s -H "$H" "$B/api/agents" | python3 -c "import json,sys;print(' '.join(a['agentId'] for a in json.load(sys.stdin)['agents']))")
echo "IDS=$IDS"

echo "=== control: 复位 box-a（删掉 center-only 文件 c.md），确认无停用 ==="
$DC box-a sh -c 'rm -f /root/.pi/agent/agents/c.md; echo "c.md after rm:"; ls /root/.pi/agent/agents/ 2>&1'
for id in $IDS; do echo -n "$id adapters: "; timeout 20 curl -s -H "$H" "$B/api/agents/$id/adapters" | python3 -c "import json,sys;d=json.load(sys.stdin);print([(a['id'],a['disabled']) for a in d['adapters']])"; done

echo
echo "=== control dispatch（未停用，应真的写盘）==="
timeout 60 curl -s -w "\nHTTP=%{http_code}\n" -X POST -H "$H" -H 'Content-Type: application/json' \
  --data '{"adapters":["pi"],"confirm":true}' "$B/api/sync?direction=dispatch&confirm=true" | tee /tmp/cc2-ctl.json
echo -n "box-a c.md 是否落盘: "; $DC box-a sh -c 'test -f /root/.pi/agent/agents/c.md && echo YES || echo NO'

echo
echo "=== 复位：删 c.md；对全部机器停用 pi ==="
$DC box-a sh -c 'rm -f /root/.pi/agent/agents/c.md'
for id in $IDS; do echo -n "disable pi on $id -> "; timeout 20 curl -s -o /dev/null -w '%{http_code}\n' -X POST -H "$H" -H 'Content-Type: application/json' --data '{"disabled":true}' "$B/api/agents/$id/adapters/pi"; done
for id in $IDS; do echo -n "$id pi disabled="; timeout 20 curl -s -H "$H" "$B/api/agents/$id/adapters" | python3 -c "import json,sys;d=json.load(sys.stdin);print([a['disabled'] for a in d['adapters'] if a['id']=='pi'])"; done

echo
echo "=== 实验 dispatch（全停用）==="
timeout 60 curl -s -w "\nHTTP=%{http_code}\n" -X POST -H "$H" -H 'Content-Type: application/json' \
  --data '{"adapters":["pi"],"confirm":true}' "$B/api/sync?direction=dispatch&confirm=true" | tee /tmp/cc2-exp.json
echo -n "box-a c.md 是否落盘: "; $DC box-a sh -c 'test -f /root/.pi/agent/agents/c.md && echo YES || echo NO'

echo
echo "=== J2 断言 ==="
python3 - <<'PY'
import json
raw=open('/tmp/cc2-exp.json').read().rsplit('\nHTTP=',1)[0]
d=json.loads(raw)
n=len(d['agents']); s=sum(1 for a in d['agents'] if a.get('skipped'))
print(f"machines={n} skipped={s} ok={d['ok']} status={d['status']} errors={d['errors']}")
print("J2 all-skipped-but-synced:", s==n and d['ok'] is True and d['status']=='synced' and d['errors'] is None)
PY

echo
echo "=== J4：用真实响应跑 index.html:3776 的 toast 表达式 ==="
python3 - <<'PY'
import json
raw=open('/tmp/cc2-exp.json').read().rsplit('\nHTTP=',1)[0]
result=json.loads(raw)
skipped=[a for a in (result.get('agents') or []) if a.get('skipped') and a.get('reason')]
if skipped: print('TOAST(全停用) = 已下发「pi」；%d 台机器因已停用该适配器被跳过'%len(skipped))
else: print('TOAST = 已下发「pi」到全部在线机器')
# 离线变体：sync.go:462-465 的 Stale 分支不写 Reason，用真实观测到的 stale 条目代入
offline={"ok":True,"status":"synced","agents":[{"agentId":"X","hostname":"x","ok":True,"skipped":True}]}
sk2=[a for a in offline['agents'] if a.get('skipped') and a.get('reason')]
print('TOAST(全离线, reason 缺失) =', ('已下发「pi」；%d 台机器因已停用该适配器被跳过'%len(sk2)) if sk2 else '已下发「pi」到全部在线机器')
PY

echo
echo "=== 恢复：全部机器重新启用 pi ==="
for id in $IDS; do timeout 20 curl -s -o /dev/null -w "$id %{http_code}\n" -X POST -H "$H" -H 'Content-Type: application/json' --data '{"disabled":false}' "$B/api/agents/$id/adapters/pi"; done
