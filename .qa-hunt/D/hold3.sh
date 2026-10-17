#!/bin/sh
DC="docker compose -f /root/code/homer-cli/qa/env/docker-compose.yml"
PID=$(timeout 20 $DC exec -T box-c sh -c 'for p in /proc/[0-9]*; do c=$(tr "\0" " " < $p/cmdline 2>/dev/null); case "$c" in *\ --home\ /root/.hhuntD*) basename $p;; esac; done | head -1')
echo "$(date -u +%FT%TZ) PID=$PID" >&2
timeout 20 $DC exec -T box-c sh -c "kill -STOP $PID"
sleep 2
timeout 110 curl -s -o .qa-hunt/D/hold3.txt -w 'hold_http=%{http_code} hold_t=%{time_total}\n' -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:17760/api/agents/54d0c3373428-4323/pull?confirm=true"
timeout 20 $DC exec -T box-c sh -c "kill -CONT $PID"
echo "$(date -u +%FT%TZ) CONT" >&2
