#!/bin/sh
LOG=/root/code/homer/.qa-hunt/D/hopen2.log
DC="docker compose -f /root/code/homer/qa/env/docker-compose.yml"
API="http://127.0.0.1:17760/api/agents"
: > $LOG
PID=40
snap() {
  timeout 10 curl -s -H 'Authorization: Bearer qa-hub-token' $API \
   | python3 -c "
import json,sys,datetime
d=json.load(sys.stdin)
now=datetime.datetime.now(datetime.timezone.utc)
for a in d.get('agents',[]):
    if a['agentId'].startswith('54d0c3373428'):
        ls=datetime.datetime.fromisoformat(a['lastSeen'].replace('Z','+00:00'))
        print('%-22s stale=%-5s age=%.1fs' % (a['agentId'],a['stale'],(now-ls).total_seconds()))
"
}
echo "$(date -u +%FT%TZ) PRE      $(snap)" | tee -a $LOG
timeout 20 $DC exec -T box-c sh -c "kill -STOP $PID && echo stopped"
echo "$(date -u +%FT%TZ) STOP pid=$PID" | tee -a $LOG
i=0
while [ $i -lt 26 ]; do
  sleep 5
  echo "$(date -u +%FT%TZ) T+$(( (i+1)*5 ))s $(snap)" | tee -a $LOG
  i=$((i+1))
done
timeout 20 $DC exec -T box-c sh -c "kill -CONT $PID && echo continued"
echo "$(date -u +%FT%TZ) CONT pid=$PID" | tee -a $LOG
sleep 10
echo "$(date -u +%FT%TZ) POST     $(snap)" | tee -a $LOG
