#!/bin/sh
# Does a half-open (frozen) agent's pull hold the global writeMutex and block other machines?
LOG=/root/code/homer-cli/.qa-hunt/D/mutex.log
DC="docker compose -f /root/code/homer-cli/qa/env/docker-compose.yml"
: > $LOG
DEAD=54d0c3373428-4323    # my isolated agent on box-c
LIVE=b6675d68ddf7-7920    # healthy machine (box-b)
PID=$(timeout 20 $DC exec -T box-c sh -c 'for p in /proc/[0-9]*; do c=$(tr "\0" " " < $p/cmdline 2>/dev/null); case "$c" in *\ --home\ /root/.hhuntD*) basename $p;; esac; done | head -1')
echo "$(date -u +%FT%TZ) DEAD_PID=$PID" | tee -a $LOG
timeout 20 $DC exec -T box-c sh -c "kill -STOP $PID"
echo "$(date -u +%FT%TZ) STOPPED $PID" | tee -a $LOG
sleep 3
# baseline: healthy machine pull latency before any lock is held
echo "$(date -u +%FT%TZ) BASELINE live pull: $(timeout 100 curl -s -o /dev/null -w 'http=%{http_code} t=%{time_total}' -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:17760/api/agents/$LIVE/pull)" | tee -a $LOG
echo "$(date -u +%FT%TZ) ISSUE pull to FROZEN $DEAD (background)" | tee -a $LOG
( timeout 100 curl -s -w '\nDEAD_HTTP=%{http_code} DEAD_T=%{time_total}\n' -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:17760/api/agents/$DEAD/pull" > .qa-hunt/D/dead_pull.txt 2>&1 ) &
sleep 2
echo "$(date -u +%FT%TZ) NOW live pull (should block if mutex held): $(timeout 100 curl -s -o /dev/null -w 'http=%{http_code} t=%{time_total}' -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:17760/api/agents/$LIVE/pull)" | tee -a $LOG
echo "$(date -u +%FT%TZ) NOW keys (also takes writeMutex): $(timeout 100 curl -s -o /dev/null -w 'http=%{http_code} t=%{time_total}' -H 'Authorization: Bearer qa-hub-token' http://127.0.0.1:17760/api/keys)" | tee -a $LOG
wait
echo "$(date -u +%FT%TZ) DEAD RESULT: $(tr '\n' ' ' < .qa-hunt/D/dead_pull.txt | head -c 400)" | tee -a $LOG
timeout 20 $DC exec -T box-c sh -c "kill -CONT $PID"
echo "$(date -u +%FT%TZ) CONT $PID" | tee -a $LOG
