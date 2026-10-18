#!/bin/sh
# Hold writeMutex via frozen agent, then measure UI responsiveness + refresh polling through CDP
DC="docker compose -f /root/code/homer/qa/env/docker-compose.yml"
PID=$(timeout 20 $DC exec -T box-c sh -c 'for p in /proc/[0-9]*; do c=$(tr "\0" " " < $p/cmdline 2>/dev/null); case "$c" in *\ --home\ /root/.hhuntD*) basename $p;; esac; done | head -1')
timeout 20 $DC exec -T box-c sh -c "kill -STOP $PID"
echo "$(date -u +%FT%TZ) STOPPED $PID"
# hold the mutex for ~70s
( timeout 100 curl -s -o .qa-hunt/D/hold.txt -w 'hold_http=%{http_code} hold_t=%{time_total}\n' -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:17760/api/agents/54d0c3373428-4323/pull?confirm=true" >> .qa-hunt/D/mutex3.log 2>&1 ) &
sleep 3
timeout 180 node /root/code/homer/.qa-hunt/D/uifreeze.js 2>&1 | grep -v -i warning | tail -40
wait
echo "$(date -u +%FT%TZ) hold: $(cat .qa-hunt/D/mutex3.log)"
timeout 20 $DC exec -T box-c sh -c "kill -CONT $PID"
echo "$(date -u +%FT%TZ) CONT $PID"
