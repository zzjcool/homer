#!/bin/sh
# C-C3 / D-C1 独立判据（不依赖 curl time_total 作为唯一证据）：
#   P1 静态：grep writeMutex 全部持锁点 -> /api/keys 与 /api/agents/<id>/pull 同一把锁
#   P2 队列相位（不计时）：并发发 N 个同锁请求，按【完成顺序】推断串行化
#       若共享一把 FIFO 公平的 Go Mutex，先到先服务；被长请求挡住的会在长请求之后完成
#   P3 因果对照（不计时）：阻塞时长 ≈ 假死机 pull 的 DEAD_T（锁释放点=半开 session done）
#   P4 连接态（不计时）：阻塞期间用 ss 看 TCP 连接 ESTABLISHED 且对端零字节 -> 是"等锁"不是"网络"
#   P5 锁外对照：/api/agents /api/console /api/status 全程 <0.05s
set -u
H='Authorization: Bearer qa-hub-token'
B=http://127.0.0.1:17760
DC="timeout 40 docker compose -f qa/env/docker-compose.yml exec -T"
DEAD=54d0c3373428-d82d      # 我的独占探针机（box-c，私有 home /root/.r3）
LIVE=e363efdcf085-1563      # 健康机（box-a）

# 拿到探针机 pid
PID=$($DC box-c sh -c 'ps -eo pid,args | awk "/[h]omer agent/ && \$1>1000 {print \$1}"' | head -1)
echo "PID=$PID"

echo "=== P5 锁外基线（/api/agents 不应受影响）==="
timeout 10 curl -s -o /dev/null -w 'agents http=%{http_code} t=%{time_total}\n' -H "$H" "$B/api/agents"
timeout 10 curl -s -o /dev/null -w 'keys   http=%{http_code} t=%{time_total}\n' -H "$H" "$B/api/keys"

echo
echo "=== 冻结探针机（SIGSTOP）==="
$DC box-c sh -c "kill -STOP $PID"
echo -n "stat: "; $DC box-c sh -c "awk '{print \$3}' /proc/$PID/stat"

echo
echo "=== 发长请求到假死机（后台），2s 后发 P2 相位序列 ==="
( timeout 150 curl -s -o /tmp/dead.json -w "DEAD http=%{http_code} t=%{time_total}\n" \
    -X POST -H "$H" -H 'Content-Type: application/json' --data '{}' "$B/api/agents/$DEAD/pull?confirm=true" > /tmp/dead.t 2>&1 ) &
sleep 2
echo "--- P2: 同时发 3 个同锁请求 + 1 个锁外请求，记录完成顺序 ---"
for i in 1 2 3; do
  ( timeout 130 curl -s -o /dev/null -w "SAME-LOCK[$i] http=%{http_code} t=%{time_total}\n" -H "$H" "$B/api/keys" >> /tmp/phase.log 2>&1 ) &
done
( timeout 130 curl -s -o /dev/null -w "OUT-LOCK    http=%{http_code} t=%{time_total}\n" -H "$H" "$B/api/agents" >> /tmp/phase.log 2>&1 ) &

sleep 6
echo "--- P4: 阻塞期间的 TCP 连接态（hub 容器内 ss）---"
timeout 30 docker exec homer-qa-hub sh -c 'ss -tnp 2>/dev/null | grep -E ":7760|:17760" | head -20 || netstat -tn 2>/dev/null | head -20 || echo NO_SS'
echo "--- 阻塞中锁外端点复查 ---"
timeout 10 curl -s -o /dev/null -w 'agents(blocked) http=%{http_code} t=%{time_total}\n' -H "$H" "$B/api/agents"
timeout 8 curl -s -o /dev/null -w 'keys(blocked)   http=%{http_code} t=%{time_total}\n' -m 6 -H "$H" "$B/api/keys" || echo "keys(blocked) TIMEOUT/0bytes"

echo
echo "=== 解冻 ==="
$DC box-c sh -c "kill -CONT $PID"
echo -n "stat after CONT: "; $DC box-c sh -c "awk '{print \$3}' /proc/$PID/stat"

wait
echo
echo "=== P2 完成顺序（写入顺序 = 实际返回顺序）==="
cat /tmp/phase.log
echo "=== P3 假死机 pull 结果 ==="
cat /tmp/dead.t; head -c 400 /tmp/dead.json; echo
echo "=== 解冻后锁内端点恢复 ==="
timeout 10 curl -s -o /dev/null -w 'keys(after CONT) http=%{http_code} t=%{time_total}\n' -H "$H" "$B/api/keys"
