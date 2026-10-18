#!/bin/sh
# Hold the global writeMutex for ~70s by pulling a frozen agent, while CDP observes the UI.
DC="docker compose -f /root/code/homer/qa/env/docker-compose.yml"
PID=$(timeout 20 $DC exec -T box-c sh -c 'for p in /proc/[0-9]*; do c=$(tr "\0" " " < $p/cmdline 2>/dev/null); case "$c" in *\ --home\ /root/.hhuntD*) basename $p;; esac; done | head -1')
echo "$(date -u +%FT%TZ) PID=$PID" >&2
timeout 20 $DC exec -T box-c sh -c "kill -STOP $PID"
echo "$(date -u +%FT%TZ) STOPPED" >&2
sleep 2
timeout 100 curl -s -o .qa-hunt/D/hold2.txt -w 'hold_http=%{http_code} hold_t=%{time_total}\n' -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:17760/api/agents/54d0c3373428-4323/pull?confirm=true"
echo "$(date -u +%FT%TZ) released" >&2
timeout 20 $DC exec -T box-c sh -c "kill -CONT $PID"
echo "$(date -u +%FT%TZ) CONT" >&2
