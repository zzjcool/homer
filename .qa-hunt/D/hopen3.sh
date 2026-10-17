#!/bin/sh
# Isolated half-open test: SIGSTOP pid of /root/.hhuntD agent on box-c.
LOG=/root/code/homer-cli/.qa-hunt/D/hopen3.log
DC="docker compose -f /root/code/homer-cli/qa/env/docker-compose.yml"
: > $LOG
PID=$(timeout 20 $DC exec -T box-c sh -c 'for p in /proc/[0-9]*; do c=$(tr "\0" " " < $p/cmdline 2>/dev/null); case "$c" in *hhuntD\ --*) ;; *\ --home\ /root/.hhuntD*) basename $p;; esac; done | head -1')
echo "$(date -u +%FT%TZ) PID=$PID" | tee -a $LOG
snap() {
  timeout 10 curl -s -H 'Authorization: Bearer qa-hub-token' http://127.0.0.1:17760/api/agents \
   | python3 -c "
import json,sys,datetime
d=json.load(sys.stdin); now=datetime.datetime.now(datetime.timezone.utc)
for a in d.get('agents',[]):
    if a['agentId']=='54d0c3373428-4323':
        ls=datetime.datetime.fromisoformat(a['lastSeen'].replace('Z','+00:00'))
        print('stale=%-5s age=%.1fs' % (a['stale'],(now-ls).total_seconds()))
"
}
echo "$(date -u +%FT%TZ) PRE     $(snap)" | tee -a $LOG
timeout 20 $DC exec -T box-c sh -c "kill -STOP $PID"
echo "$(date -u +%FT%TZ) STOP" | tee -a $LOG
i=0
while [ $i -lt 26 ]; do
  sleep 5
  echo "$(date -u +%FT%TZ) T+$(( (i+1)*5 ))s $(snap)" | tee -a $LOG
  i=$((i+1))
done
timeout 20 $DC exec -T box-c sh -c "kill -CONT $PID"
echo "$(date -u +%FT%TZ) CONT" | tee -a $LOG
sleep 12
echo "$(date -u +%FT%TZ) POST    $(snap)" | tee -a $LOG
