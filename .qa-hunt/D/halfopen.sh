#!/bin/sh
# Half-open simulation: SIGSTOP the agent process on box-c (TCP stays ESTABLISHED, no pongs)
LOG=/root/code/homer/.qa-hunt/D/halfopen.log
DC="docker compose -f /root/code/homer/qa/env/docker-compose.yml"
API="http://127.0.0.1:17760/api/agents"
: > $LOG
pid=$(timeout 20 $DC exec -T box-c sh -c 'for p in /proc/[0-9]*; do c=$(tr "\0" " " < $p/cmdline 2>/dev/null); case "$c" in *"/root/.local/bin/homer agent"*) basename $p;; esac; done | head -1')
echo "$(date -u +%FT%TZ) AGENT_PID=$pid" | tee -a $LOG
snap() {
  timeout 10 curl -s -H 'Authorization: Bearer qa-hub-token' $API \
   | python3 -c "
import json,sys,datetime
d=json.load(sys.stdin)
now=datetime.datetime.now(datetime.timezone.utc)
for a in d.get('agents',[]):
    if a['agentId'].startswith('54d0c3373428'):
        ls=datetime.datetime.fromisoformat(a['lastSeen'].replace('Z','+00:00'))
        print('stale=%s lastSeenAge=%.1fs' % (a['stale'],(now-ls).total_seconds()))
"
}
echo "$(date -u +%FT%TZ) PRE  $(snap)" | tee -a $LOG
timeout 20 $DC exec -T box-c sh -c "kill -STOP $pid"
echo "$(date -u +%FT%TZ) STOPPED pid=$pid" | tee -a $LOG
i=0
while [ $i -lt 30 ]; do
  sleep 5
  echo "$(date -u +%FT%TZ) T+$(( (i+1)*5 ))s $(snap)" | tee -a $LOG
  i=$((i+1))
done
timeout 20 $DC exec -T box-c sh -c "kill -CONT $pid"
echo "$(date -u +%FT%TZ) CONT pid=$pid" | tee -a $LOG
sleep 8
echo "$(date -u +%FT%TZ) POST $(snap)" | tee -a $LOG
