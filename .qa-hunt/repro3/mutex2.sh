#!/bin/sh
# 第二轮：不提前 CONT，让假死机 pull 自然等到 ping timeout 才释放锁。
# 判据（非计时为主）：
#  [O1] 顺序：锁外的 /api/agents 立刻返回；同锁的 /api/keys 必须等长 pull 返回之后才返回
#  [O2] 无-confirm 的 409 早退路径是否也取锁 -> 对健康机发不带 confirm 的 pull，看是否同样被挡
#  [O3] 连接态：阻塞期间 curl 已建立 TCP（ss 若可得）
#  [O4] 因果：所有同锁请求的返回时刻都 >= 长 pull 返回时刻
set -u
H='Authorization: Bearer qa-hub-token'
B=http://127.0.0.1:17760
DC="timeout 40 docker compose -f qa/env/docker-compose.yml exec -T"
DEAD=54d0c3373428-d82d
LIVE=e363efdcf085-1563
PID=$($DC box-c sh -c 'ps -eo pid,args | awk "/[h]omer agent/ && \$1>1000 {print \$1}"' | head -1)
echo "PID=$PID"
echo "=== ss 可用性 ==="
timeout 20 docker exec homer-qa-hub sh -c 'command -v ss; command -v netstat; command -v lsof; command -v /proc; ls /proc/1/net/tcp >/dev/null && echo PROC_NET_TCP_OK'
rm -f /tmp/p2.log /tmp/dead2.t
echo "=== STOP ==="; $DC box-c sh -c "kill -STOP $PID"; $DC box-c sh -c "awk '{print \$3}' /proc/$PID/stat"
( timeout 150 curl -s -o /tmp/dead2.json -w "DEAD-PULL http=%{http_code} t=%{time_total}\n" \
   -X POST -H "$H" -H 'Content-Type: application/json' --data '{}' "$B/api/agents/$DEAD/pull?confirm=true" >> /tmp/dead2.t 2>&1 ) &
DEADPULL_PID=$!
sleep 3
echo "=== T+3 起并发探测 ==="
( timeout 130 curl -s -o /dev/null -w "keys-GET       http=%{http_code} t=%{time_total}\n" -H "$H" "$B/api/keys" >> /tmp/p2.log 2>&1 ) &
( timeout 130 curl -s -o /dev/null -w "keys-OP-POST   http=%{http_code} t=%{time_total}\n" -X POST -H "$H" -H 'Content-Type: application/json' --data '{"action":"list"}' "$B/api/keys/op" >> /tmp/p2.log 2>&1 ) &
( timeout 130 curl -s -o /dev/null -w "live-pull-NOCONFIRM http=%{http_code} t=%{time_total}\n" -X POST -H "$H" -H 'Content-Type: application/json' --data '{}' "$B/api/agents/$LIVE/pull" >> /tmp/p2.log 2>&1 ) &
( timeout 130 curl -s -o /dev/null -w "live-pull-CONFIRM   http=%{http_code} t=%{time_total}\n" -X POST -H "$H" -H 'Content-Type: application/json' --data '{}' "$B/api/agents/$LIVE/pull?confirm=true" >> /tmp/p2.log 2>&1 ) &
( timeout 130 curl -s -o /dev/null -w "OUT-agents     http=%{http_code} t=%{time_total}\n" -H "$H" "$B/api/agents" >> /tmp/p2.log 2>&1 ) &
( timeout 130 curl -s -o /dev/null -w "OUT-console    http=%{http_code} t=%{time_total}\n" -H "$H" "$B/api/console" >> /tmp/p2.log 2>&1 ) &
sleep 5
echo "=== T+8 锁外复查（应毫秒级）==="
timeout 8 curl -s -o /dev/null -w 'OUT-agents@T8 http=%{http_code} t=%{time_total}\n' -H "$H" "$B/api/agents"
echo "=== T+8 锁内复查（应 0 bytes 超时）==="
timeout 8 curl -s -o /dev/null -m 6 -w 'keys@T8       http=%{http_code} t=%{time_total}\n' -H "$H" "$B/api/keys" || echo "keys@T8 -> curl exit=$? (0 bytes, 挂起)"
echo "=== O3 /proc/net/tcp 中与 hub 端口相关的 ESTABLISHED 计数 ==="
timeout 20 docker exec homer-qa-hub sh -c 'awk "NR>1 && \$4==\"01\" {c++} END {print \"ESTABLISHED sockets:\", c+0}" /proc/net/tcp'
wait $DEADPULL_PID 2>/dev/null
wait
echo "=== CONT ==="; $DC box-c sh -c "kill -CONT $PID"; $DC box-c sh -c "awk '{print \$3}' /proc/$PID/stat"
echo "=== O1/O4 完成顺序 ==="; cat /tmp/p2.log
echo "=== DEAD pull ==="; cat /tmp/dead2.t; head -c 300 /tmp/dead2.json; echo
echo "=== 恢复后 ==="; timeout 10 curl -s -o /dev/null -w 'keys-after http=%{http_code} t=%{time_total}\n' -H "$H" "$B/api/keys"
