set -x
H=http://127.0.0.1:17760
# mint code with hub token
CODE=$(curl -s -H "Authorization: Bearer qa-hub-token" $H/api/auth/join | python3 -c 'import sys,json;print(json.load(sys.stdin)["code"])')
echo "CODE=$CODE" | tee /tmp/code1.txt
# onboard box-a using that code (burns it via stream hello)
docker exec homer-qa-box-a sh -c "curl -fsSL http://hub:7760/install.sh | sh -s -- --token $CODE" 2>&1 | tail -20
sleep 8
docker exec homer-qa-box-a sh -c 'cat /root/.homer/agent.json 2>/dev/null' ; echo
curl -s -H "Authorization: Bearer qa-hub-token" $H/api/agents | python3 -c 'import sys,json;d=json.load(sys.stdin);print([a["agentId"] for a in d["agents"]])'
