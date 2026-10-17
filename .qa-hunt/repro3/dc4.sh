#!/bin/sh
# D-C4 独立判据：不看 hunter 的「行还在 + stale 翻了」，而做
#   「已吊销机器」与「只是掉线的机器」的【渲染卡片逐字段 diff】
#   若两卡片 dotClass / statusText / badge / disabled 按钮集 完全一致 -> 不可区分成立
set -u
H='Authorization: Bearer qa-hub-token'
B=http://127.0.0.1:17760
MY=54d0c3373428-d82d
OFFLINE=e363efdcf085-7b6d     # 本就掉线的机器（非我造成）
echo "=== revoke 我的探针机 $MY ==="
timeout 20 curl -s -w "\nHTTP=%{http_code}\n" -X POST -H "$H" -H 'Content-Type: application/json' --data "{\"agentId\":\"$MY\"}" "$B/api/agents/revoke"
sleep 6
echo "=== 第二次 revoke（应 409 not-enrolled）==="
timeout 20 curl -s -w "\nHTTP=%{http_code}\n" -X POST -H "$H" -H 'Content-Type: application/json' --data "{\"agentId\":\"$MY\"}" "$B/api/agents/revoke"
echo
echo "=== API 侧两行状态 ==="
timeout 20 curl -s -H "$H" "$B/api/agents" | python3 -c "
import json,sys
for a in json.load(sys.stdin)['agents']:
    if a['agentId'] in ('$MY','$OFFLINE'):
        print(a['agentId'],'stale=',a['stale'],'drift=',(a.get('drift') or {}))"
echo
echo "=== hub 日志（确认真被踢）==="
timeout 30 docker logs --since 2m homer-qa-hub 2>&1 | grep -i "$MY" | tail -5
