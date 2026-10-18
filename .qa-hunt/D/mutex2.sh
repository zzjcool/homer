#!/bin/sh
# Repeat mutex test with confirm=true so both pull requests reach the real dispatch path.
LOG=/root/code/homer/.qa-hunt/D/mutex2.log
DC="docker compose -f /root/code/homer/qa/env/docker-compose.yml"
: > $LOG
DEAD=54d0c3373428-4323
LIVE=b6675d68ddf7-7920
PID=$(timeout 20 $DC exec -T box-c sh -c 'for p in /proc/[0-9]*; do c=$(tr "\0" " " < $p/cmdline 2>/dev/null); case "$c" in *\ --home\ /root/.hhuntD*) basename $p;; esac; done | head -1')
echo "$(date -u +%FT%TZ) DEAD_PID=$PID" | tee -a $LOG
timeout 20 $DC exec -T box-c sh -c "kill -STOP $PID"
echo "$(date -u +%FT%TZ) STOPPED" | tee -a $LOG
sleep 2
echo "$(date -u +%FT%TZ) BASELINE live: $(timeout 100 curl -s -o /dev/null -w 'http=%{http_code} t=%{time_total}' -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:17760/api/agents/$LIVE/pull?confirm=true")" | tee -a $LOG
echo "$(date -u +%FT%TZ) ISSUE frozen pull (bg)" | tee -a $LOG
( timeout 100 curl -s -w '\nDEAD_HTTP=%{http_code} DEAD_T=%{time_total}\n' -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:17760/api/agents/$DEAD/pull?confirm=true" > .qa-hunt/D/dead_pull2.txt 2>&1 ) &
sleep 2
echo "$(date -u +%FT%TZ) BLOCKED live: $(timeout 100 curl -s -o /dev/null -w 'http=%{http_code} t=%{time_total}' -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:17760/api/agents/$LIVE/pull?confirm=true")" | tee -a $LOG
echo "$(date -u +%FT%TZ) BLOCKED keys: $(timeout 100 curl -s -o /dev/null -w 'http=%{http_code} t=%{time_total}' -H 'Authorization: Bearer qa-hub-token' http://127.0.0.1:17760/api/keys)" | tee -a $LOG
echo "$(date -u +%FT%TZ) BLOCKED status(live, no mutex? ): $(timeout 100 curl -s -o /dev/null -w 'http=%{http_code} t=%{time_total}' -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:17760/api/agents/$LIVE/status")" | tee -a $LOG
wait
echo "$(date -u +%FT%TZ) DEAD: $(tr '\n' ' ' < .qa-hunt/D/dead_pull2.txt | head -c 500)" | tee -a $LOG
timeout 20 $DC exec -T box-c sh -c "kill -CONT $PID"
echo "$(date -u +%FT%TZ) CONT" | tee -a $LOG
